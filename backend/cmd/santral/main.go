// Command santral runs the call-manager control plane.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/toprakgureli/santral-c/backend/configs"
	"github.com/toprakgureli/santral-c/backend/internal/setup"
	"github.com/toprakgureli/santral-c/backend/internal/sse"
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
		slog.Error("santral stopped with an error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	path := flag.String("config", "config.yml", "path to the configuration file")
	checkOnly := flag.Bool("check-config", false, "check the configuration file and exit")
	flag.Parse()
	if env := os.Getenv("SANTRAL_CONFIG"); env != "" {
		*path = env
	}

	if err := configs.Load(*path); err != nil {
		return fmt.Errorf("configuration could not be loaded: %w", err)
	}
	problems := configs.Check(configs.Cnf)
	if *checkOnly {
		return reportConfig(problems)
	}
	// A live server refuses a bad configuration before it touches the
	// database; a test setup only reports it.
	live := configs.Cnf.App.Development == configs.Live
	for _, p := range problems {
		if !p.Fatal || !live {
			slog.Warn("configuration problem", "key", p.Key, "problem", p.Message)
		}
	}
	if live {
		if err := configs.Validate(configs.Cnf); err != nil {
			return err
		}
	}
	// Integration secrets in the database are sealed with the data key,
	// which is kept apart from the session signing key.
	ring, err := crypt.NewKeyring(configs.Cnf.Security.DataKey, strings.Split(configs.Cnf.Security.PreviousDataKeys, ",")...)
	if err != nil {
		return fmt.Errorf("security.dataKey: %w", err)
	}

	if err := postgresql.Connect(configs.Cnf.Database); err != nil {
		return err
	}
	db := postgresql.Get()
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("database handle could not be retrieved: %w", err)
	}
	// One server at a time: the queues, timers and live connections assume
	// it, and two servers would also migrate the schema together.
	lock, err := holdInstanceLock(context.Background(), sqlDB)
	if err != nil {
		return err
	}
	defer lock.release()
	if err := migrations.Run(sqlDB); err != nil {
		return err
	}
	if err := redis.Connect(configs.Cnf.Redis); err != nil {
		return err
	}
	if err := setup.Seed(db); err != nil {
		return err
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

	signals, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Background loops get their own context: they stop only after the
	// last request was answered, and shutdown waits for them.
	work, stopWork := context.WithCancel(context.Background())
	defer stopWork()
	var workers safe.Group
	srv.start(work, &workers)

	listened := make(chan error, 1)
	go func() {
		listened <- app.Listen(net.JoinHostPort(configs.Cnf.App.Host, configs.Cnf.App.Port))
	}()
	slog.Info("server started", "host", configs.Cnf.App.Host, "port", configs.Cnf.App.Port, "env", string(configs.Cnf.App.Development), "version", version, "buildTime", buildTime)

	// A server that could not listen (port taken, wrong address) exits
	// with an error, so systemd starts it again.
	var runErr error
	select {
	case <-signals.Done():
		slog.Info("shutdown signal received")
	case err := <-listened:
		if err == nil {
			err = errors.New("the server stopped listening")
		}
		runErr = fmt.Errorf("server stopped: %w", err)
	}

	// Event streams stay open as long as a browser keeps them; they end
	// first, so the open requests can finish in time.
	sse.CloseAll()
	httpCtx, cancelHTTP := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelHTTP()
	if err := app.ShutdownWithContext(httpCtx); err != nil {
		slog.Warn("open requests did not finish before shutdown", "error", err)
	}
	stopWork()
	waitCtx, cancelWait := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelWait()
	if err := workers.Wait(waitCtx); err != nil {
		slog.Warn("background work did not finish before shutdown", "error", err)
	}
	slog.Info("server stopped")
	return runErr
}

// reportConfig prints the configuration check for deploy.sh and fails on
// any problem that would stop a live server.
func reportConfig(problems []configs.Problem) error {
	fatal := false
	for _, p := range problems {
		label := "UYARI"
		if p.Fatal {
			label, fatal = "HATA", true
		}
		fmt.Printf("%s %s\n", label, p)
	}
	if fatal {
		return errors.New("config.yml düzeltilmeden servis başlatılmamalı")
	}
	fmt.Println("config.yml uygun")
	return nil
}

// instanceLock is the database lock that keeps a second server from
// starting while one runs.
type instanceLock struct {
	conn *sql.Conn
}

// instanceLockID names santral's lock among PostgreSQL advisory locks.
const instanceLockID = 731_0001

func holdInstanceLock(ctx context.Context, db *sql.DB) (*instanceLock, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("database connection for the instance lock: %w", err)
	}
	var got bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", instanceLockID).Scan(&got); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("instance lock: %w", err)
	}
	if !got {
		_ = conn.Close()
		return nil, errors.New("another santral server is already running on this database")
	}
	return &instanceLock{conn: conn}, nil
}

func (l *instanceLock) release() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = l.conn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", instanceLockID)
	_ = l.conn.Close()
}
