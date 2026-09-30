// Command santral runs the call-manager control plane.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/toprakgureli/santral-c/backend/configs"
	"github.com/toprakgureli/santral-c/backend/internal/setup"
	"github.com/toprakgureli/santral-c/backend/internal/teams"
	"github.com/toprakgureli/santral-c/backend/internal/whatsapp"
	"github.com/toprakgureli/santral-c/backend/migrations"
	"github.com/toprakgureli/santral-c/backend/pkg/crypt"
	"github.com/toprakgureli/santral-c/backend/pkg/logctx"
	"github.com/toprakgureli/santral-c/backend/pkg/postgresql"
	"github.com/toprakgureli/santral-c/backend/pkg/redis"
	"github.com/toprakgureli/santral-c/backend/pkg/safe"
)

// Build stamp, set at link time via -ldflags "-X main.version=... -X main.buildTime=...".
// They default to "dev" so a local build is obviously not a release.
var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	// Log lines written with a request's context carry its request and user.
	slog.SetDefault(slog.New(logctx.NewHandler(slog.NewJSONHandler(os.Stderr, nil))))
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
	// Integration secrets in the database are sealed with the data key,
	// which is kept apart from the session signing key.
	if strings.Contains(configs.Cnf.Security.DataKey, "change-me") {
		return errors.New("security.dataKey is still the example value; generate one with: openssl rand -hex 32")
	}
	ring, err := crypt.NewKeyring(configs.Cnf.Security.DataKey, strings.Split(configs.Cnf.Security.PreviousDataKeys, ",")...)
	if err != nil {
		return fmt.Errorf("security.dataKey: %w", err)
	}
	if err := setup.RewrapSecrets(context.Background(), db, ring,
		setup.SecretPurposes{WhatsApp: whatsapp.SealPurpose, Drive: teams.DriveSealPurpose},
		setup.LegacyKeys{WhatsApp: "wa:" + configs.Cnf.Auth.Secret, Drive: configs.Cnf.Auth.Secret}); err != nil {
		return err
	}

	srv, err := newServer(configs.Cnf, db, ring)
	if err != nil {
		return err
	}
	app := srv.app

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Background loops run until ctx ends; shutdown waits for them.
	var workers safe.Group
	srv.start(ctx, &workers)

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
	return workers.Wait(shutdownCtx)
}
