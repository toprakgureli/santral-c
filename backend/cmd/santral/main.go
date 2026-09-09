// Command santral runs the call-manager control plane.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/requestid"

	"github.com/toprakgureli/santral-c/backend/configs"
	"github.com/toprakgureli/santral-c/backend/internal/audit"
	"github.com/toprakgureli/santral-c/backend/internal/auth"
	"github.com/toprakgureli/santral-c/backend/internal/contact"
	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/internal/role"
	"github.com/toprakgureli/santral-c/backend/internal/security"
	"github.com/toprakgureli/santral-c/backend/internal/setting"
	"github.com/toprakgureli/santral-c/backend/internal/setup"
	"github.com/toprakgureli/santral-c/backend/internal/user"
	"github.com/toprakgureli/santral-c/backend/internal/verimor"
	"github.com/toprakgureli/santral-c/backend/migrations"
	"github.com/toprakgureli/santral-c/backend/pkg/denylist"
	"github.com/toprakgureli/santral-c/backend/pkg/lockout"
	"github.com/toprakgureli/santral-c/backend/pkg/postgresql"
	"github.com/toprakgureli/santral-c/backend/pkg/redis"
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
	secSvc := security.NewService(configs.Cnf.Security, security.NewRepository(db), lockout.New())
	authSvc := auth.NewService(configs.Cnf.Auth, configs.Cnf.Security, sessionRepo, userSvc, secSvc, deny, setting.NewService(db))
	authHandler := auth.NewHandler(configs.Cnf.Auth, authSvc)
	userHandler := user.NewHandler(userSvc)
	roleSvc := role.NewService(role.NewRepository(db), userSvc)
	roleHandler := role.NewHandler(roleSvc)
	contactSvc := contact.NewService(contact.NewRepository(db), userSvc, auditSvc)
	contactHandler := contact.NewHandler(contactSvc)
	guard := middlewares.Auth(configs.Cnf.Auth, deny)

	app := fiber.New(fiber.Config{
		AppName:      configs.Cnf.App.Name,
		ErrorHandler: middlewares.ErrorHandler,
	})
	app.Use(requestid.New())
	app.Use(middlewares.Recover())
	app.Use(cors.New(cors.Config{
		AllowOrigins:     configs.Cnf.App.CORSOrigins,
		AllowCredentials: configs.Cnf.App.CORSOrigins != "",
	}))
	app.Get("/healthz", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	api := app.Group("/api/v1")
	auth.NewRouter(authHandler, guard).Routes(api)
	user.NewRouter(userHandler, guard).Routes(api)
	role.NewRouter(roleHandler, guard).Routes(api)
	contact.NewRouter(contactHandler, guard).Routes(api)

	if configs.Cnf.Bulutsantralim.Enabled {
		verimorClient := verimor.NewClient(configs.Cnf.Bulutsantralim.APIKey, configs.Cnf.Bulutsantralim.APIBase)
		verimorSvc := verimor.NewService(verimorClient, userSvc, verimor.NewRepository(db), configs.Cnf.Bulutsantralim)
		verimor.NewRouter(verimor.NewHandler(verimorSvc), guard).Routes(api)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := app.Listen(":" + configs.Cnf.App.Port); err != nil {
			slog.Error("server stopped", "error", err)
			stop()
		}
	}()
	slog.Info("server started", "port", configs.Cnf.App.Port, "env", string(configs.Cnf.App.Development))

	<-ctx.Done()
	slog.Info("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown failed: %w", err)
	}
	return nil
}
