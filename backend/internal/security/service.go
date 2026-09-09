package security

import (
	"context"
	"log/slog"
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
type Service struct {
	cfg     configs.Security
	repo    IRepository
	lockout ILockout
}

// NewService builds a security service.
func NewService(cfg configs.Security, repo IRepository, lock ILockout) *Service {
	return &Service{cfg: cfg, repo: repo, lockout: lock}
}

// Guard rejects a login when the IP is banned or the account is locked.
func (s *Service) Guard(ctx context.Context, email, ip string) error {
	if s.trusted(ip) {
		return nil
	}
	ban, err := s.repo.ActiveBan(ctx, ip, time.Now())
	if err != nil {
		return errs.Internal(err)
	}
	if ban != nil {
		return errs.TooMany("Çok fazla başarısız deneme yapıldı. Lütfen daha sonra tekrar deneyin.")
	}
	remaining, err := s.lockout.Remaining(ctx, key(email))
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
		slog.Warn("failure counters could not be reset", "error", err)
	}
	if err := s.lockout.Clear(ctx, key(a.Email)); err != nil {
		slog.Warn("lockout could not be cleared", "error", err)
	}
}

// Failure records a failed attempt and applies IP and account lockout.
func (s *Service) Failure(ctx context.Context, a Attempt) {
	s.record(ctx, a, false)
	if s.trusted(a.IP) {
		return
	}
	since := time.Now().Add(-s.cfg.AttemptWindow)
	count, err := s.repo.FailuresByIP(ctx, a.IP, since)
	if err != nil {
		slog.Warn("ip failures could not be counted", "error", err)
	} else if int(count) >= s.cfg.IPFailureLimit {
		s.ban(ctx, a.IP, "repeated failed login")
	}
	ips, err := s.repo.FailingIPs(ctx, a.Email, since)
	if err != nil {
		slog.Warn("failing ips could not be listed", "error", err)
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
	until := time.Now().Add(s.cfg.AccountLockDuration)
	if err := s.lockout.Set(ctx, key(a.Email), s.cfg.AccountLockDuration); err != nil {
		slog.Warn("account lockout could not be set", "error", err)
	}
	if err := s.repo.MarkUserLock(ctx, a.Email, &until); err != nil {
		slog.Warn("user lock could not be marked", "error", err)
	}
	slog.Warn("distributed login attack detected", "email", a.Email, "ipCount", len(ips))
}

func (s *Service) ban(ctx context.Context, ip, reason string) {
	if err := s.repo.Ban(ctx, ip, reason, s.cfg.IPBanDuration); err != nil {
		slog.Warn("ip could not be banned", "ip", ip, "error", err)
		return
	}
	slog.Warn("ip banned", "ip", ip, "reason", reason)
}

func (s *Service) record(ctx context.Context, a Attempt, success bool) {
	attempt := &models.LoginAttempt{
		Email:     a.Email,
		UserID:    a.UserID,
		IP:        a.IP,
		UserAgent: a.UserAgent,
		Success:   success,
		Reason:    a.Reason,
	}
	if err := s.repo.Record(ctx, attempt); err != nil {
		slog.Warn("login attempt could not be recorded", "error", err)
	}
}

func (s *Service) trusted(ip string) bool {
	if s.cfg.TrustedIPs == "" {
		return false
	}
	for _, t := range strings.Split(s.cfg.TrustedIPs, ",") {
		if strings.TrimSpace(t) == ip {
			return true
		}
	}
	return false
}

func key(email string) string {
	return "login:" + strings.ToLower(strings.TrimSpace(email))
}
