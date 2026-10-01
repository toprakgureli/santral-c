// Command resetpw sets a new password for a user directly in the database,
// for recovering a locked-out owner. It clears MFA, unlocks the account,
// signs the user out everywhere and asks for a new password at the next
// sign-in. The change is written to the audit log.
//
// The password is read from standard input, never from the command line,
// so it does not end up in the shell history or the process list:
//
//	go run ./cmd/resetpw -config config.yml -email owner@example.com
//
// An unknown email is refused; -create makes it a new owner account.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/toprakgureli/santral-c/backend/configs"
	"github.com/toprakgureli/santral-c/backend/internal/audit"
	"github.com/toprakgureli/santral-c/backend/internal/auth"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/denylist"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/hash"
	"github.com/toprakgureli/santral-c/backend/pkg/password"
	"github.com/toprakgureli/santral-c/backend/pkg/postgresql"
	"github.com/toprakgureli/santral-c/backend/pkg/redis"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "HATA:", err)
		os.Exit(1)
	}
}

func run() error {
	cfgPath := flag.String("config", "config.yml", "path to the configuration file")
	email := flag.String("email", "", "user email")
	create := flag.Bool("create", false, "create the user as an owner when the email is unknown")
	name := flag.String("name", "Owner", "display name when creating a new user")
	flag.Parse()

	address := strings.ToLower(strings.TrimSpace(*email))
	if address == "" {
		return errors.New("kullanım: resetpw -email kisi@ornek.com [-config config.yml] [-create] [-name Ad]")
	}
	pw, err := readPassword()
	if err != nil {
		return err
	}
	if err := password.Validate(pw); err != nil {
		return err
	}

	if err := configs.Load(*cfgPath); err != nil {
		return fmt.Errorf("config okunamadı: %w", err)
	}
	if err := postgresql.Connect(configs.Cnf.Database); err != nil {
		return fmt.Errorf("veritabanına ulaşılamadı: %w", err)
	}
	if err := redis.Connect(configs.Cnf.Redis); err != nil {
		return fmt.Errorf("Redis'e ulaşılamadı: %w", err) //nolint:staticcheck,revive // starts with a proper noun
	}
	db := postgresql.Get()
	ctx := context.Background()

	hashed, err := hash.Password(pw)
	if err != nil {
		return fmt.Errorf("şifre işlenemedi: %w", err)
	}

	var u models.User
	created := false
	err = db.WithContext(ctx).Where("lower(email) = ?", address).First(&u).Error
	switch {
	case err == nil:
		if err := db.WithContext(ctx).Model(&models.User{}).Where("id = ?", u.ID).Updates(map[string]any{
			"password":             hashed,
			"must_change_password": true,
			"mfa_enabled":          false,
			"mfa_secret":           nil,
			"failed_count":         0,
			"locked_until":         nil,
			"active":               true,
		}).Error; err != nil {
			return fmt.Errorf("şifre kaydedilemedi: %w", err)
		}
	case !*create:
		return fmt.Errorf("%s adında bir kullanıcı yok; yeni sahip hesabı açmak için -create ekleyin", address)
	default:
		var role models.Role
		if err := db.WithContext(ctx).Where("name = ?", string(enums.RoleInvisibleAdmin)).First(&role).Error; err != nil {
			return fmt.Errorf("sahip rolü bulunamadı (sunucuyu bir kez çalıştırın): %w", err)
		}
		u = models.User{Name: *name, Email: address, Password: hashed, Active: true, MustChangePassword: true, Roles: []models.Role{role}}
		if err := db.WithContext(ctx).Omit("Roles.*").Create(&u).Error; err != nil {
			return fmt.Errorf("kullanıcı oluşturulamadı: %w", err)
		}
		created = true
	}

	// Whoever held a session for this account is out from now on.
	revoker := auth.NewRevoker(auth.NewRepository(db), denylist.New(), configs.Cnf.Auth.AccessTTL)
	if err := revoker.RevokeUserSessions(ctx, u.ID, time.Now()); err != nil {
		return fmt.Errorf("şifre değişti ama açık oturumlar kapatılamadı: %w", err)
	}
	audit.NewService(db).Record(ctx, audit.Entry{
		Action:     enums.AuditUserPasswordReset,
		TargetType: "user",
		TargetID:   strconv.FormatUint(uint64(u.ID), 10),
		Detail:     map[string]any{"by": "resetpw", "created": created},
	})
	fmt.Printf("%s için şifre ayarlandı: MFA sıfırlandı, hesap açıldı, bütün oturumlar kapatıldı. İlk girişte yeni şifre istenecek.\n", address)
	return nil
}

// readPassword reads the new password twice from standard input.
func readPassword() (string, error) {
	in := bufio.NewReader(os.Stdin)
	ask := func(prompt string) (string, error) {
		fmt.Fprint(os.Stderr, prompt)
		line, err := in.ReadString('\n')
		if err != nil && line == "" {
			return "", errors.New("şifre okunamadı")
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	first, err := ask("Yeni şifre: ")
	if err != nil {
		return "", err
	}
	second, err := ask("Yeni şifre (tekrar): ")
	if err != nil {
		return "", err
	}
	if first != second {
		return "", errors.New("iki şifre aynı değil")
	}
	return first, nil
}
