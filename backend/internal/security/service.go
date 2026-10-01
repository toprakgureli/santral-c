package security

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/toprakgureli/santral-c/backend/configs"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// Attempt reason codes.
const (
	ReasonBadCredentials string = "bad_credentials"
	ReasonInactive       string = "inactive"
	ReasonLocked         string = "locked"
	ReasonBanned         string = "banned_ip"
	ReasonMFA            string = "mfa_required"
)

// Service enforces lockout and records attempts.
//
// Wrong passwords are counted three ways. The browser that typed them is
// held back first (its device cookie), the account is locked when it is
// tried too often from anywhere, and only an address outside the trusted
// list is banned, at a much higher count. A whole office signs in from one
// public address, so one person's typos never lock everyone out.
type Service struct {
	cfg     configs.Security
	repo    IRepository
	lockout ILockout
	trust   []*net.IPNet
}

// NewService builds a security service.
func NewService(cfg configs.Security, repo IRepository, lock ILockout) *Service {
	return &Service{cfg: cfg, repo: repo, lockout: lock, trust: configs.Nets(cfg.TrustedIPs)}
}

// accountFailureLimit is how many wrong passwords an account takes within
// the attempt window before it is locked.
const accountFailureLimit = 10

// deviceHold is how long a browser waits after too many wrong passwords.
const deviceHold = 5 * time.Minute

// Guard rejects a sign-in when the browser is held back, the address is
// banned or the account is locked.
func (s *Service) Guard(ctx context.Context, a Attempt) error {
	if a.Device != "" {
		left, err := s.lockout.Remaining(ctx, deviceKey(a.Device))
		if err != nil {
			return errs.Internal(err)
		}
		if left > 0 {
			return errs.TooMany(fmt.Sprintf("Bu tarayıcıdan çok fazla hatalı deneme yapıldı. %s sonra tekrar deneyin.", minutes(left)))
		}
	}
	if !s.trusted(a.IP) {
		ban, err := s.repo.ActiveBan(ctx, a.IP, time.Now())
		if err != nil {
			return errs.Internal(err)
		}
		if ban != nil {
			return errs.TooMany("Çok fazla başarısız deneme yapıldı. Lütfen daha sonra tekrar deneyin.")
		}
	}
	remaining, err := s.lockout.Remaining(ctx, key(a.Email))
	if err != nil {
		return errs.Internal(err)
	}
	if remaining > 0 {
		return errs.Locked("Hesap geçici olarak kilitli. Lütfen daha sonra tekrar deneyin.")
	}
	return nil
}

// Success records a successful attempt and clears failure state.
func (s *Service) Success(ctx context.Context, a Attempt) {
	s.record(ctx, a, true)
	if err := s.repo.ResetFailures(ctx, a.Email); err != nil {
		slog.WarnContext(ctx, "failure counters could not be reset", "error", err)
	}
	for _, k := range []string{key(a.Email), failKey(a.Email)} {
		if err := s.lockout.Clear(ctx, k); err != nil {
			slog.WarnContext(ctx, "lockout could not be cleared", "error", err)
		}
	}
	if a.Device != "" {
		if err := s.lockout.Clear(ctx, deviceFailKey(a.Device)); err != nil {
			slog.WarnContext(ctx, "device lockout could not be cleared", "error", err)
		}
	}
}

// Failure records a failed attempt and applies the browser, account and
// address limits.
func (s *Service) Failure(ctx context.Context, a Attempt) {
	s.record(ctx, a, false)
	window := s.cfg.AttemptWindow
	// A wrong code at the second step has its own limit there; only wrong
	// passwords hold a browser back.
	if a.Device != "" && a.Reason != ReasonMFA {
		n, err := s.lockout.Hit(ctx, deviceFailKey(a.Device), window)
		if err != nil {
			slog.WarnContext(ctx, "device failures could not be counted", "error", err)
		} else if int(n) >= s.cfg.DeviceFailureLimit {
			if err := s.lockout.Set(ctx, deviceKey(a.Device), deviceHold); err != nil {
				slog.WarnContext(ctx, "device could not be held back", "error", err)
			}
		}
	}
	n, err := s.lockout.Hit(ctx, failKey(a.Email), window)
	if err != nil {
		slog.WarnContext(ctx, "account failures could not be counted", "error", err)
	} else if n >= accountFailureLimit {
		s.lockAccount(ctx, a.Email, "repeated failed login")
	}
	if s.trusted(a.IP) {
		return
	}
	since := time.Now().Add(-window)
	count, err := s.repo.FailuresByIP(ctx, a.IP, since)
	if err != nil {
		slog.WarnContext(ctx, "ip failures could not be counted", "error", err)
	} else if int(count) >= s.cfg.IPFailureLimit {
		s.ban(ctx, a.IP, "repeated failed login")
	}
	ips, err := s.repo.FailingIPs(ctx, a.Email, since)
	if err != nil {
		slog.WarnContext(ctx, "failing ips could not be listed", "error", err)
		return
	}
	if len(ips) < s.cfg.DistinctIPLimit {
		return
	}
	for _, ip := range ips {
		if s.trusted(ip) {
			continue
		}
		s.ban(ctx, ip, "distributed login attempt")
	}
	s.lockAccount(ctx, a.Email, "distributed login attempt")
}

func (s *Service) lockAccount(ctx context.Context, email, reason string) {
	until := time.Now().Add(s.cfg.AccountLockDuration)
	if err := s.lockout.Set(ctx, key(email), s.cfg.AccountLockDuration); err != nil {
		slog.WarnContext(ctx, "account lockout could not be set", "error", err)
	}
	if err := s.repo.MarkUserLock(ctx, email, &until); err != nil {
		slog.WarnContext(ctx, "user lock could not be marked", "error", err)
	}
	slog.WarnContext(ctx, "account locked", "email", email, "reason", reason)
}

func (s *Service) ban(ctx context.Context, ip, reason string) {
	if err := s.repo.Ban(ctx, ip, reason, s.cfg.IPBanDuration); err != nil {
		slog.WarnContext(ctx, "ip could not be banned", "ip", ip, "error", err)
		return
	}
	slog.WarnContext(ctx, "ip banned", "ip", ip, "reason", reason)
}

func (s *Service) record(ctx context.Context, a Attempt, success bool) {
	attempt := &models.LoginAttempt{
		Email:     a.Email,
		UserID:    a.UserID,
		IP:        a.IP,
		UserAgent: a.UserAgent,
		Device:    a.Device,
		Success:   success,
		Reason:    a.Reason,
	}
	if err := s.repo.Record(ctx, attempt); err != nil {
		slog.WarnContext(ctx, "login attempt could not be recorded", "error", err)
	}
}

func (s *Service) trusted(ip string) bool {
	return configs.Contains(s.trust, ip)
}

func key(email string) string {
	return "login:" + strings.ToLower(strings.TrimSpace(email))
}

func failKey(email string) string {
	return "login-fail:" + strings.ToLower(strings.TrimSpace(email))
}

func deviceKey(device string) string {
	return "device:" + device
}

func deviceFailKey(device string) string {
	return "device-fail:" + device
}

// minutes says a wait in whole minutes, at least one.
func minutes(d time.Duration) string {
	m := int(d.Round(time.Minute) / time.Minute)
	if m < 1 {
		m = 1
	}
	return fmt.Sprintf("%d dakika", m)
}
