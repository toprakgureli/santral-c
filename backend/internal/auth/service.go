package auth

import (
	"context"
	"strings"
	"time"

	"github.com/toprakgureli/santral-c/backend/configs"
	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/requests"
	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/responses"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/internal/security"
	"github.com/toprakgureli/santral-c/backend/internal/setting"
	"github.com/toprakgureli/santral-c/backend/pkg/crypt"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/hash"
	"github.com/toprakgureli/santral-c/backend/pkg/jwt"
	"github.com/toprakgureli/santral-c/backend/pkg/totp"
)

const loginMinLatency time.Duration = 350 * time.Millisecond

// timingGuardHash is a real argon2id hash compared against when an email is
// unknown, so a missing account costs the same time as a wrong password.
var timingGuardHash = mustHash("santral-timing-guard")

func mustHash(plain string) string {
	h, err := hash.Password(plain)
	if err != nil {
		panic(err)
	}
	return h
}

// RequestMeta carries client metadata for an attempt.
type RequestMeta struct {
	IP        string
	UserAgent string
}

// LoginResult is the outcome of an authentication step.
type LoginResult struct {
	User                   responses.User
	Token                  string
	ExpiresAt              time.Time
	RefreshToken           string
	RefreshExpiresAt       time.Time
	MFARequired            bool
	MFAToken               string
	MFASetupRequired       bool
	PasswordChangeRequired bool
	PasswordToken          string
}

// Service is the auth application service.
type Service struct {
	cfg      configs.Auth
	sec      configs.Security
	repo     IRepository
	user     IUserService
	security ISecurityService
	denylist IDenylist
	settings ISettingService
}

// NewService builds an auth service.
func NewService(cfg configs.Auth, sec configs.Security, repo IRepository, user IUserService, secSvc ISecurityService, deny IDenylist, settings ISettingService) *Service {
	return &Service{cfg: cfg, sec: sec, repo: repo, user: user, security: secSvc, denylist: deny, settings: settings}
}

// Login validates credentials and returns a session or a challenge.
func (s *Service) Login(ctx context.Context, req requests.Login, meta RequestMeta) (*LoginResult, error) {
	defer levelLatency(time.Now())

	email := strings.ToLower(strings.TrimSpace(req.Email))

	if err := s.security.Guard(ctx, email, meta.IP); err != nil {
		return nil, err
	}

	u, err := s.user.GetByEmail(ctx, email)
	if err != nil {
		return nil, err
	}

	if u == nil {
		hash.Compare(timingGuardHash, req.Password)
		s.security.Failure(ctx, attempt(email, nil, meta, security.ReasonBadCredentials))
		return nil, errs.Unauthorized("E-posta veya şifre hatalı.")
	}

	if !hash.Compare(u.Password, req.Password) {
		s.security.Failure(ctx, attempt(email, &u.ID, meta, security.ReasonBadCredentials))
		return nil, errs.Unauthorized("E-posta veya şifre hatalı.")
	}

	if !u.Active {
		s.security.Failure(ctx, attempt(email, &u.ID, meta, security.ReasonInactive))
		return nil, errs.Forbidden("Hesabınız pasif durumda. Yöneticinizle iletişime geçin.")
	}

	// The policy decides whether this login is asked for a second factor at
	// all: off asks nobody, trusted asks nobody from the listed addresses.
	asked := s.mfaAsked(ctx, meta.IP)

	if asked && !u.MFAEnabled && !u.MFAExempt {
		token, err := jwt.GenerateEnroll(s.cfg, u.ID)
		if err != nil {
			return nil, errs.Internal(err)
		}
		s.security.Success(ctx, attempt(email, &u.ID, meta, security.ReasonMFA))
		return &LoginResult{MFASetupRequired: true, MFAToken: token.Value}, nil
	}

	if asked && u.MFAEnabled {
		token, err := jwt.GenerateMFA(s.cfg, u.ID)
		if err != nil {
			return nil, errs.Internal(err)
		}
		s.security.Success(ctx, attempt(email, &u.ID, meta, security.ReasonMFA))
		return &LoginResult{MFARequired: true, MFAToken: token.Value}, nil
	}

	s.security.Success(ctx, attempt(email, &u.ID, meta, ""))
	if err := s.user.MarkLogin(ctx, u.ID); err != nil {
		return nil, err
	}
	return s.issue(ctx, u, meta, nil)
}

func attempt(email string, userID *uint, meta RequestMeta, reason string) security.Attempt {
	return security.Attempt{Email: email, UserID: userID, IP: meta.IP, UserAgent: meta.UserAgent, Reason: reason}
}

func levelLatency(start time.Time) {
	if elapsed := time.Since(start); elapsed < loginMinLatency {
		time.Sleep(loginMinLatency - elapsed)
	}
}

// Me returns the authenticated user's public view.
func (s *Service) Me(ctx context.Context, userID uint) (*responses.User, error) {
	u, err := s.user.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !u.Active {
		return nil, errs.Forbidden("Hesabınız pasif durumda.")
	}
	dto := responses.NewUser(u)
	return &dto, nil
}

// mfaAsked says whether a login from ip goes through the second factor.
func (s *Service) mfaAsked(ctx context.Context, ip string) bool {
	if s.settings == nil {
		return false
	}
	mode, trusted := s.settings.MFAPolicy(ctx)
	switch mode {
	case setting.MFAOff:
		return false
	case setting.MFATrusted:
		return !setting.IPTrusted(ip, trusted)
	}
	return true
}

// Refresh rotates a valid refresh session into a new access token.
func (s *Service) Refresh(ctx context.Context, refreshToken string, meta RequestMeta) (*LoginResult, error) {
	if refreshToken == "" {
		return nil, errs.Unauthorized("Oturum bulunamadı. Lütfen giriş yapın.")
	}
	session, err := s.repo.SessionByHash(ctx, hash.SHA256(refreshToken))
	if err != nil {
		return nil, errs.Internal(err)
	}
	if session == nil || session.RevokedAt != nil || session.ExpiresAt.Before(time.Now()) {
		return nil, errs.Unauthorized("Oturumunuz geçersiz veya süresi dolmuş.")
	}
	u, err := s.user.GetByID(ctx, session.UserID)
	if err != nil {
		return nil, err
	}
	if !u.Active {
		return nil, errs.Forbidden("Hesabınız pasif durumda.")
	}
	return s.issue(ctx, u, meta, session)
}

// Logout revokes the refresh session and denylists the access token.
func (s *Service) Logout(ctx context.Context, accessToken, refreshToken string) error {
	now := time.Now()
	if refreshToken != "" {
		session, err := s.repo.SessionByHash(ctx, hash.SHA256(refreshToken))
		if err != nil {
			return errs.Internal(err)
		}
		if session != nil {
			if err := s.repo.RevokeSession(ctx, session.ID, now); err != nil {
				return errs.Internal(err)
			}
		}
	}
	if accessToken == "" {
		return nil
	}
	claims, err := jwt.Parse(s.cfg, accessToken)
	if err != nil || claims.ExpiresAt == nil {
		return nil
	}
	if ttl := time.Until(claims.ExpiresAt.Time); ttl > 0 {
		if err := s.denylist.Add(ctx, claims.ID, ttl); err != nil {
			return errs.Internal(err)
		}
	}
	return nil
}

func (s *Service) issue(ctx context.Context, u *models.User, meta RequestMeta, existing *models.Session) (*LoginResult, error) {
	if u.MustChangePassword {
		token, err := jwt.GeneratePassword(s.cfg, u.ID)
		if err != nil {
			return nil, errs.Internal(err)
		}
		return &LoginResult{PasswordChangeRequired: true, PasswordToken: token.Value}, nil
	}

	access, err := jwt.Generate(s.cfg, u.ID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	refresh, err := hash.Token(32)
	if err != nil {
		return nil, errs.Internal(err)
	}

	now := time.Now()
	refreshExpiresAt := now.Add(s.cfg.RefreshTTL)
	tokenHash := hash.SHA256(refresh)

	if existing != nil {
		if err := s.repo.RotateSession(ctx, existing.ID, tokenHash, refreshExpiresAt, now); err != nil {
			return nil, errs.Internal(err)
		}
	} else {
		session := &models.Session{UserID: u.ID, TokenHash: tokenHash, IP: meta.IP, UserAgent: meta.UserAgent, ExpiresAt: refreshExpiresAt}
		if err := s.repo.CreateSession(ctx, session); err != nil {
			return nil, errs.Internal(err)
		}
	}

	return &LoginResult{
		User:             responses.NewUser(u),
		Token:            access.Value,
		ExpiresAt:        access.ExpiresAt,
		RefreshToken:     refresh,
		RefreshExpiresAt: refreshExpiresAt,
	}, nil
}

// MFASetup starts voluntary TOTP setup for a logged-in user.
func (s *Service) MFASetup(ctx context.Context, userID uint) (*responses.MFASetup, error) {
	u, err := s.user.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if u.MFAEnabled {
		return nil, errs.Conflict("İki adımlı doğrulama zaten açık.", nil)
	}
	return s.newSecret(ctx, u)
}

func (s *Service) newSecret(ctx context.Context, u *models.User) (*responses.MFASetup, error) {
	key, err := totp.Generate(s.cfg.Issuer, u.Email)
	if err != nil {
		return nil, errs.Internal(err)
	}
	encrypted, err := crypt.Encrypt(s.sec.MFAKey, key.Secret)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if err := s.user.SetMFA(ctx, u.ID, &encrypted, false); err != nil {
		return nil, err
	}
	return &responses.MFASetup{Secret: key.Secret, URL: key.URL, QR: key.QR}, nil
}

// MFAEnable verifies a code and turns on TOTP.
func (s *Service) MFAEnable(ctx context.Context, userID uint, code string) error {
	u, err := s.user.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	secret, err := s.secret(u)
	if err != nil {
		return err
	}
	if !totp.Validate(secret, code) {
		return errs.Invalid("Kod doğrulanamadı. Uygulamadaki güncel kodu girin.", nil)
	}
	return s.user.SetMFA(ctx, u.ID, u.MFASecret, true)
}

// MFAVerify completes the login second factor.
func (s *Service) MFAVerify(ctx context.Context, token, code string, meta RequestMeta) (*LoginResult, error) {
	defer levelLatency(time.Now())

	claims, err := jwt.Parse(s.cfg, token)
	if err != nil || claims.Purpose != jwt.PurposeMFA {
		return nil, errs.Unauthorized("Doğrulama oturumu geçersiz veya süresi dolmuş.")
	}
	revoked, err := s.denylist.Has(ctx, claims.ID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if revoked {
		return nil, errs.Unauthorized("Doğrulama oturumu geçersiz veya süresi dolmuş.")
	}
	u, err := s.user.GetByID(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}
	if !u.Active || !u.MFAEnabled {
		return nil, errs.Forbidden("Hesabınız bu işlem için uygun değil.")
	}
	secret, err := s.secret(u)
	if err != nil {
		return nil, err
	}
	if !totp.Validate(secret, code) {
		s.security.Failure(ctx, attempt(u.Email, &u.ID, meta, security.ReasonMFA))
		return nil, errs.Unauthorized("Kod doğrulanamadı.")
	}
	s.revokeToken(ctx, claims)
	s.security.Success(ctx, attempt(u.Email, &u.ID, meta, ""))
	if err := s.user.MarkLogin(ctx, u.ID); err != nil {
		return nil, err
	}
	return s.issue(ctx, u, meta, nil)
}

func (s *Service) secret(u *models.User) (string, error) {
	if u.MFASecret == nil || *u.MFASecret == "" {
		return "", errs.Invalid("Önce iki adımlı doğrulama kurulumunu başlatın.", nil)
	}
	secret, err := crypt.Decrypt(s.sec.MFAKey, *u.MFASecret)
	if err != nil {
		return "", errs.Internal(err)
	}
	return secret, nil
}

// MFAEnroll begins forced enrollment during login.
func (s *Service) MFAEnroll(ctx context.Context, token string) (*responses.MFASetup, error) {
	u, _, err := s.enrollUser(ctx, token)
	if err != nil {
		return nil, err
	}
	return s.newSecret(ctx, u)
}

// MFAEnrollVerify completes forced enrollment during login.
func (s *Service) MFAEnrollVerify(ctx context.Context, token, code string, meta RequestMeta) (*LoginResult, error) {
	defer levelLatency(time.Now())

	u, claims, err := s.enrollUser(ctx, token)
	if err != nil {
		return nil, err
	}
	secret, err := s.secret(u)
	if err != nil {
		return nil, err
	}
	if !totp.Validate(secret, code) {
		s.security.Failure(ctx, attempt(u.Email, &u.ID, meta, security.ReasonMFA))
		return nil, errs.Unauthorized("Kod doğrulanamadı.")
	}
	if err := s.user.SetMFA(ctx, u.ID, u.MFASecret, true); err != nil {
		return nil, err
	}
	s.revokeToken(ctx, claims)
	s.security.Success(ctx, attempt(u.Email, &u.ID, meta, ""))
	if err := s.user.MarkLogin(ctx, u.ID); err != nil {
		return nil, err
	}
	return s.issue(ctx, u, meta, nil)
}

func (s *Service) enrollUser(ctx context.Context, token string) (*models.User, *jwt.Claims, error) {
	claims, err := jwt.Parse(s.cfg, token)
	if err != nil || claims.Purpose != jwt.PurposeEnroll {
		return nil, nil, errs.Unauthorized("Kurulum oturumu geçersiz veya süresi dolmuş.")
	}
	revoked, err := s.denylist.Has(ctx, claims.ID)
	if err != nil {
		return nil, nil, errs.Internal(err)
	}
	if revoked {
		return nil, nil, errs.Unauthorized("Kurulum oturumu geçersiz veya süresi dolmuş.")
	}
	u, err := s.user.GetByID(ctx, claims.UserID)
	if err != nil {
		return nil, nil, err
	}
	if !u.Active || u.MFAEnabled {
		return nil, nil, errs.Forbidden("Hesabınız bu işlem için uygun değil.")
	}
	return u, claims, nil
}

// PasswordChange completes the forced first-login password step.
func (s *Service) PasswordChange(ctx context.Context, req requests.PasswordChange, meta RequestMeta) (*LoginResult, error) {
	defer levelLatency(time.Now())

	claims, err := jwt.Parse(s.cfg, req.Token)
	if err != nil || claims.Purpose != jwt.PurposePassword {
		return nil, errs.Unauthorized("Oturum geçersiz veya süresi dolmuş.")
	}
	revoked, err := s.denylist.Has(ctx, claims.ID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if revoked {
		return nil, errs.Unauthorized("Oturum geçersiz veya süresi dolmuş.")
	}
	u, err := s.user.GetByID(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}
	if !u.Active || !u.MustChangePassword {
		return nil, errs.Forbidden("Hesabınız bu işlem için uygun değil.")
	}
	if hash.Compare(u.Password, req.Password) {
		return nil, errs.Invalid("Yeni şifre eskisiyle aynı olamaz.", nil)
	}
	if err := s.user.ChangePassword(ctx, u.ID, req.Password); err != nil {
		return nil, err
	}
	s.revokeToken(ctx, claims)
	u.MustChangePassword = false
	s.security.Success(ctx, attempt(u.Email, &u.ID, meta, ""))
	return s.issue(ctx, u, meta, nil)
}

func (s *Service) revokeToken(ctx context.Context, claims *jwt.Claims) {
	if claims.ExpiresAt == nil {
		return
	}
	if ttl := time.Until(claims.ExpiresAt.Time); ttl > 0 {
		_ = s.denylist.Add(ctx, claims.ID, ttl)
	}
}
