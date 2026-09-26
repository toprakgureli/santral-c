// Command santral runs the call-manager control plane.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/requestid"

	"github.com/toprakgureli/santral-c/backend/configs"
	"github.com/toprakgureli/santral-c/backend/internal/audit"
	"github.com/toprakgureli/santral-c/backend/internal/auth"
	"github.com/toprakgureli/santral-c/backend/internal/calllog"
	"github.com/toprakgureli/santral-c/backend/internal/contact"
	"github.com/toprakgureli/santral-c/backend/internal/escalation"
	"github.com/toprakgureli/santral-c/backend/internal/games"
	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/internal/performance"
	"github.com/toprakgureli/santral-c/backend/internal/profile"
	"github.com/toprakgureli/santral-c/backend/internal/role"
	"github.com/toprakgureli/santral-c/backend/internal/security"
	"github.com/toprakgureli/santral-c/backend/internal/setting"
	"github.com/toprakgureli/santral-c/backend/internal/setup"
	"github.com/toprakgureli/santral-c/backend/internal/shift"
	"github.com/toprakgureli/santral-c/backend/internal/teams"
	"github.com/toprakgureli/santral-c/backend/internal/user"
	"github.com/toprakgureli/santral-c/backend/internal/verimor"
	"github.com/toprakgureli/santral-c/backend/internal/whatsapp"
	"github.com/toprakgureli/santral-c/backend/migrations"
	"github.com/toprakgureli/santral-c/backend/pkg/denylist"
	"github.com/toprakgureli/santral-c/backend/pkg/lockout"
	"github.com/toprakgureli/santral-c/backend/pkg/postgresql"
	"github.com/toprakgureli/santral-c/backend/pkg/redis"
)

// Build stamp, set at link time via -ldflags "-X main.version=... -X main.buildTime=...".
// They default to "dev" so a local build is obviously not a release.
var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	if err := run(); err != nil {
		slog.Error("startup failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	path := flag.String("config", "config.yml", "path to the configuration file")
	flag.Parse()
	if env := os.Getenv("SANTRAL_CONFIG"); env != "" {
		*path = env
	}

	if err := configs.Load(*path); err != nil {
		return fmt.Errorf("configuration could not be loaded: %w", err)
	}
	if err := postgresql.Connect(configs.Cnf.Database); err != nil {
		return err
	}

	db := postgresql.Get()
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("database handle could not be retrieved: %w", err)
	}
	if err := migrations.Run(sqlDB); err != nil {
		return err
	}
	if err := redis.Connect(configs.Cnf.Redis); err != nil {
		return err
	}
	if err := setup.Seed(db); err != nil {
		return err
	}

	deny := denylist.New()
	sessionRepo := auth.NewRepository(db)
	auditSvc := audit.NewService(db)
	userRepo := user.NewRepository(db)

	userSvc := user.NewService(userRepo, auditSvc, sessionRepo, nil)
	secRepo := security.NewRepository(db)
	secSvc := security.NewService(configs.Cnf.Security, secRepo, lockout.New())
	secHandler := security.NewHandler(security.NewAdmin(secRepo, userSvc, auditSvc))
	settingSvc := setting.NewService(db, userSvc, auditSvc)
	settingHandler := setting.NewHandler(settingSvc)
	auditHandler := audit.NewHandler(audit.NewReader(db, userSvc))
	authSvc := auth.NewService(configs.Cnf.Auth, configs.Cnf.Security, sessionRepo, userSvc, secSvc, deny, settingSvc)
	authHandler := auth.NewHandler(configs.Cnf.Auth, authSvc)
	userHandler := user.NewHandler(userSvc)
	roleSvc := role.NewService(role.NewRepository(db), userSvc, auditSvc)
	roleHandler := role.NewHandler(roleSvc)
	contactRepo := contact.NewRepository(db)
	contactSvc := contact.NewService(contactRepo, userSvc, auditSvc)
	contactHandler := contact.NewHandler(contactSvc)
	perfSvc := performance.NewService(performance.NewRepository(db), userSvc, contactRepo)
	perfHandler := performance.NewHandler(perfSvc)
	escalationSvc := escalation.NewService(escalation.NewRepository(db), userSvc)
	escalationHandler := escalation.NewHandler(escalationSvc)
	callLogSvc := calllog.NewService(calllog.NewRepository(db), userSvc)
	callLogSvc.OnEnded = escalationSvc.AutoLog
	callLogHandler := calllog.NewHandler(callLogSvc)
	shiftSvc := shift.NewService(shift.NewRepository(db), auditSvc)
	shiftHandler := shift.NewHandler(shiftSvc)
	guard := middlewares.Auth(configs.Cnf.Auth, deny)

	fiberCfg := fiber.Config{
		AppName:      configs.Cnf.App.Name,
		ErrorHandler: middlewares.ErrorHandler,
		// WhatsApp documents may be up to 100 MB.
		BodyLimit: 110 << 20,
	}
	// Behind nginx/Cloudflare, trust the configured proxies and read the real
	// client IP from X-Forwarded-For so audit trails and lockouts are accurate.
	if tp := strings.TrimSpace(configs.Cnf.App.TrustedProxies); tp != "" {
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
	app.Use(requestid.New())
	app.Use(middlewares.Recover())
	app.Use(cors.New(cors.Config{
		AllowOrigins:     configs.Cnf.App.CORSOrigins,
		AllowCredentials: configs.Cnf.App.CORSOrigins != "",
	}))
	app.Get("/healthz", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "version": version, "buildTime": buildTime})
	})

	api := app.Group("/api/v1")
	// Public build stamp so the panel can show whether the running backend is the
	// latest deploy.
	api.Get("/version", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"version": version, "buildTime": buildTime})
	})
	auth.NewRouter(authHandler, guard).Routes(api)
	user.NewRouter(userHandler, guard).Routes(api)
	role.NewRouter(roleHandler, guard).Routes(api)
	security.NewRouter(secHandler, guard).Routes(api)
	setting.NewRouter(settingHandler, guard).Routes(api)
	audit.NewRouter(auditHandler, guard).Routes(api)
	contact.NewRouter(contactHandler, guard).Routes(api)
	escalation.NewRouter(escalationHandler, guard).Routes(api)
	calllog.NewRouter(callLogHandler, guard).Routes(api)
	shift.NewRouter(shiftHandler, guard).Routes(api)
	performance.NewRouter(perfHandler, guard).Routes(api)
	profile.NewRouter(profile.NewHandler(profile.NewService(profile.NewRepository(db))), guard).Routes(api)
	drive := teams.NewDrive(configs.Cnf.Drive, configs.Cnf.Auth.Secret, db)
	teamsSvc := teams.NewService(teams.NewRepository(db), userSvc, teams.NewHub(), drive)
	teams.NewRouter(teams.NewHandler(teamsSvc), guard).Routes(api)
	gamesSvc := games.NewService(games.NewRepository(db), userSvc, teamsSvc)
	games.NewRouter(games.NewHandler(gamesSvc), guard).Routes(api)
	waSvc := whatsapp.NewService(db, userSvc, teamsSvc, drive, configs.Cnf.Auth.Secret)
	whatsapp.NewRouter(whatsapp.NewHandler(waSvc), guard).Routes(api)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Shifts left open past the evening cutoff are closed by the sweeper.
	shiftSvc.StartSweeper(ctx)
	// Chat uploads that never became a message are removed from Drive.
	teamsSvc.StartSweeper(ctx)
	// The games' referee clock and the hockey simulation.
	gamesSvc.StartClock(ctx)
	// WhatsApp: webhook processing, the send queue and the timed work.
	waSvc.Start(ctx)

	if configs.Cnf.Bulutsantralim.Enabled {
		verimorClient := verimor.NewClient(configs.Cnf.Bulutsantralim.APIKey, configs.Cnf.Bulutsantralim.APIBase)
		verimorSvc := verimor.NewService(verimorClient, userSvc, verimor.NewRepository(db), configs.Cnf.Bulutsantralim)
		// Calls and presence changes need an open shift; a shift change in turn
		// drives the agent's presence and do-not-disturb.
		verimorSvc.SetShifts(shiftSvc)
		verimorSvc.SetBreakLimit(settingSvc)
		verimorSvc.SetContacts(contactRepo)
		shiftSvc.SetPresence(verimorSvc)
		perfSvc.SetLive(verimorSvc)
		verimorSvc.Start(ctx)
		verimor.NewRouter(verimor.NewHandler(verimorSvc), guard).Routes(api)
	}

	go func() {
		if err := app.Listen(":" + configs.Cnf.App.Port); err != nil {
			slog.Error("server stopped", "error", err)
			stop()
		}
	}()
	slog.Info("server started", "port", configs.Cnf.App.Port, "env", string(configs.Cnf.App.Development), "version", version, "buildTime", buildTime)

	<-ctx.Done()
	slog.Info("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown failed: %w", err)
	}
	return nil
}
