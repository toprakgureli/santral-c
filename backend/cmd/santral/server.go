package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/configs"
	"github.com/toprakgureli/santral-c/backend/internal/audit"
	"github.com/toprakgureli/santral-c/backend/internal/auth"
	"github.com/toprakgureli/santral-c/backend/internal/backup"
	"github.com/toprakgureli/santral-c/backend/internal/calllog"
	"github.com/toprakgureli/santral-c/backend/internal/contact"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/internal/escalation"
	"github.com/toprakgureli/santral-c/backend/internal/games"
	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/internal/ops"
	"github.com/toprakgureli/santral-c/backend/internal/performance"
	"github.com/toprakgureli/santral-c/backend/internal/profile"
	"github.com/toprakgureli/santral-c/backend/internal/role"
	"github.com/toprakgureli/santral-c/backend/internal/security"
	"github.com/toprakgureli/santral-c/backend/internal/setting"
	"github.com/toprakgureli/santral-c/backend/internal/shift"
	"github.com/toprakgureli/santral-c/backend/internal/sse"
	"github.com/toprakgureli/santral-c/backend/internal/teams"
	"github.com/toprakgureli/santral-c/backend/internal/telemetry"
	"github.com/toprakgureli/santral-c/backend/internal/user"
	"github.com/toprakgureli/santral-c/backend/internal/verimor"
	"github.com/toprakgureli/santral-c/backend/internal/whatsapp"
	"github.com/toprakgureli/santral-c/backend/pkg/crypt"
	"github.com/toprakgureli/santral-c/backend/pkg/denylist"
	"github.com/toprakgureli/santral-c/backend/pkg/lockout"
	"github.com/toprakgureli/santral-c/backend/pkg/redis"
	"github.com/toprakgureli/santral-c/backend/pkg/safe"
)

// server is the wired application: its routes and the background work
// that runs beside them.
type server struct {
	app     *fiber.App
	workers []func(ctx context.Context, g *safe.Group)
	// callLog is kept so the tests can run one pass of the call checker
	// instead of waiting for its timer.
	callLog *calllog.Service
}

// start runs the background work until ctx ends.
func (s *server) start(ctx context.Context, g *safe.Group) {
	for _, w := range s.workers {
		w(ctx, g)
	}
}

// newServer builds every module on db and mounts its routes. Redis must
// already be connected. Nothing runs until start is called.
func newServer(cfg configs.Config, db *gorm.DB, ring *crypt.Keyring) (*server, error) {
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("database handle could not be retrieved: %w", err)
	}

	deny := denylist.New()
	sessionRepo := auth.NewRepository(db)
	auditSvc := audit.NewService(db)
	userRepo := user.NewRepository(db)

	revoker := auth.NewRevoker(sessionRepo, deny, cfg.Auth.AccessTTL)
	// Every module loads the acting user through actors: a lean row with
	// roles, cached for a few seconds.
	actors := user.NewActors(db)
	userSvc := user.NewService(userRepo, auditSvc, revoker, actors)
	secRepo := security.NewRepository(db)
	secSvc := security.NewService(cfg.Security, secRepo, lockout.New())
	secHandler := security.NewHandler(security.NewAdmin(secRepo, actors, auditSvc))
	settingSvc := setting.NewService(db, actors, auditSvc)
	settingHandler := setting.NewHandler(settingSvc)
	auditHandler := audit.NewHandler(audit.NewReader(db, actors))
	authSvc := auth.NewService(cfg.Auth, cfg.Security, sessionRepo, userSvc, secSvc, deny, settingSvc, lockout.New(), revoker)
	authHandler := auth.NewHandler(cfg.Auth, authSvc)
	userHandler := user.NewHandler(userSvc)
	roleSvc := role.NewService(role.NewRepository(db), actors, auditSvc, actors)
	roleHandler := role.NewHandler(roleSvc)
	contactRepo := contact.NewRepository(db)
	contactSvc := contact.NewService(contactRepo, actors, auditSvc)
	contactHandler := contact.NewHandler(contactSvc)
	perfSvc := performance.NewService(performance.NewRepository(db), actors, contactRepo)
	perfHandler := performance.NewHandler(perfSvc)
	escalationSvc := escalation.NewService(escalation.NewRepository(db), actors)
	escalationHandler := escalation.NewHandler(escalationSvc)
	callLogSvc := calllog.NewService(calllog.NewRepository(db), actors)
	callLogHandler := calllog.NewHandler(callLogSvc)
	shiftSvc := shift.NewService(shift.NewRepository(db), auditSvc)
	shiftHandler := shift.NewHandler(shiftSvc)
	guard := middlewares.Auth(cfg.Auth, deny, actors)
	// need puts a route's permission next to the route.
	need := middlewares.NewRequirer(actors)

	fiberCfg := fiber.Config{
		AppName:      cfg.App.Name,
		ErrorHandler: middlewares.ErrorHandler,
		// WhatsApp documents may be up to 100 MB.
		BodyLimit: 110 << 20,
		// /API/v1/users is not /api/v1/users: the web servers in front
		// decide what to pass on by the path as written, so the backend
		// must not answer a spelling they did not mean to let through.
		CaseSensitive: true,
	}
	serverTimeouts.apply(&fiberCfg)
	// Behind nginx/Cloudflare, trust the configured proxies and read the real
	// client IP from X-Forwarded-For so audit trails and lockouts are accurate.
	if tp := strings.TrimSpace(cfg.App.TrustedProxies); tp != "" {
		proxies := make([]string, 0)
		for _, p := range strings.Split(tp, ",") {
			if p = strings.TrimSpace(p); p != "" {
				proxies = append(proxies, p)
			}
		}
		fiberCfg.EnableTrustedProxyCheck = true
		fiberCfg.TrustedProxies = proxies
		fiberCfg.ProxyHeader = fiber.HeaderXForwardedFor
	}
	app := fiber.New(fiberCfg)
	serverTimeouts.install(app)
	// The port for a webhook already registered in Meta reaches nothing but
	// those webhooks.
	app.Use(hookPortOnly)
	app.Use(requestid.New())
	app.Use(middlewares.RequestContext())
	// Numbers for Prometheus and a trace for every request.
	metrics := telemetry.NewMetrics(sqlDB, sse.Open)
	app.Use(metrics.Middleware(middlewares.StatusOf))
	app.Use(telemetry.TraceMiddleware(middlewares.StatusOf))
	app.Use(middlewares.Recover())
	app.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.App.CORSOrigins,
		AllowCredentials: cfg.App.CORSOrigins != "",
	}))
	// The health check the uptime monitor and deploy.sh call, and the queue
	// numbers for whoever runs the server.
	ops.NewHandler(sqlDB, redis.Get(), ops.Build{Version: version, Time: buildTime}, metrics.Registry).Routes(app)
	// The system warnings for people holding system.health.
	monitor := ops.NewMonitor(sqlDB, redis.Get(), cfg.Database.DataPath)
	metrics.Registry.MustRegister(monitor)

	api := app.Group("/api/v1")
	// Public build stamp so the panel can show whether the running backend is the
	// latest deploy.
	api.Get("/version", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"version": version, "buildTime": buildTime})
	})
	auth.NewRouter(authHandler, guard, configs.Nets(cfg.Security.TrustedIPs)).Routes(api)
	user.NewRouter(userHandler, guard, need).Routes(api)
	role.NewRouter(roleHandler, guard, need).Routes(api)
	security.NewRouter(secHandler, guard, need).Routes(api)
	setting.NewRouter(settingHandler, guard, need).Routes(api)
	audit.NewRouter(auditHandler, guard, need).Routes(api)
	contact.NewRouter(contactHandler, guard, need).Routes(api)
	escalation.NewRouter(escalationHandler, guard, need).Routes(api)
	calllog.NewRouter(callLogHandler, guard, need).Routes(api)
	shift.NewRouter(shiftHandler, guard).Routes(api)
	performance.NewRouter(perfHandler, guard, need).Routes(api)
	profile.NewRouter(profile.NewHandler(profile.NewService(profile.NewRepository(db))), guard).Routes(api)
	drive := teams.NewDrive(cfg.Drive, ring, db)
	teamsSvc := teams.NewService(teams.NewRepository(db), actors, teams.NewHub(), drive, auditSvc)
	teams.NewRouter(teams.NewHandler(teamsSvc), guard, need).Routes(api)
	gamesSvc := games.NewService(games.NewRepository(db), actors, teamsSvc)
	games.NewRouter(games.NewHandler(gamesSvc), guard, need).Routes(api)
	waSvc := whatsapp.NewService(db, actors, teamsSvc, drive, auditSvc, ring, cfg.Auth.Secret)
	waRouter := whatsapp.NewRouter(whatsapp.NewHandler(waSvc), guard)
	waRouter.Routes(api)
	// A webhook already registered in Meta may live outside /api.
	waRouter.Root(app)
	// A finished phone call may be logged on its ticket and followed by a
	// survey on WhatsApp.
	callLogSvc.OnEnded = func(ctx context.Context, log models.CallLog) {
		escalationSvc.AutoLog(ctx, log)
		waSvc.OnCallEnded(ctx, log)
	}

	// Database backups to a Shared Drive, set up in the panel.
	backupSvc := backup.NewService(db, cfg.Database, ring, actors, auditSvc)
	backup.NewRouter(backup.NewHandler(backupSvc), guard, need).Routes(api)
	monitor.Routes(api, guard, need)

	s := &server{app: app, callLog: callLogSvc}
	s.workers = append(s.workers,
		// Shifts left open past the evening cutoff are closed by the sweeper.
		shiftSvc.StartSweeper,
		// Chat uploads that never became a message are removed from Drive.
		teamsSvc.StartSweeper,
		// The games' referee clock and the hockey simulation.
		gamesSvc.StartClock,
		// WhatsApp: webhook processing, the send queue and the timed work.
		waSvc.Start,
		// A copy of the database every six hours.
		backupSvc.Start,
		// The system warnings are checked once a minute.
		monitor.Start,
	)

	if cfg.Bulutsantralim.Enabled {
		verimorClient := verimor.NewClient(cfg.Bulutsantralim.APIKey, cfg.Bulutsantralim.APIBase)
		verimorSvc := verimor.NewService(verimorClient, actors, verimor.NewRepository(db), auditSvc, cfg.Bulutsantralim)
		// Calls and presence changes need an open shift; a shift change in turn
		// drives the agent's presence and do-not-disturb.
		verimorSvc.SetShifts(shiftSvc)
		verimorSvc.SetBreakLimit(settingSvc)
		verimorSvc.SetContacts(contactRepo)
		shiftSvc.SetPresence(verimorSvc)
		perfSvc.SetLive(verimorSvc)
		// What follows a call waits until the phone system's record shows it.
		callLogSvc.SetVerifier(verimorSvc)
		s.workers = append(s.workers, verimorSvc.Start, callLogSvc.StartVerifier)
		verimor.NewRouter(verimor.NewHandler(verimorSvc), guard, need).Routes(api)
	}
	return s, nil
}

// hookEntryHeader is set by the web server that serves a webhook already
// registered in Meta (deploy/nginx/whatsapp-existing-webhook.conf). On
// that port only those webhooks answer; the panel's API, the health check
// and the metrics are not there, whatever the spelling of the path.
const hookEntryHeader = "X-Santral-Entry"

func hookPortOnly(c *fiber.Ctx) error {
	if c.Get(hookEntryHeader) != "existing-hook" {
		return c.Next()
	}
	p := strings.ToLower(c.Path())
	for _, closed := range []string{"/api", "/healthz", "/metrics"} {
		if p == closed || strings.HasPrefix(p, closed+"/") {
			return fiber.ErrNotFound
		}
	}
	return c.Next()
}
