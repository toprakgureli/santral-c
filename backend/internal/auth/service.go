package auth

import (
	"context"
	"fmt"
	"log/slog"
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
	// Device is the browser's id from its device cookie.
	Device string
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
	attempts IAttempts
	revoker  *Revoker
}

// NewService builds an auth service.
func NewService(cfg configs.Auth, sec configs.Security, repo IRepository, user IUserService, secSvc ISecurityService, deny IDenylist, settings ISettingService, attempts IAttempts, revoker *Revoker) *Service {
	return &Service{cfg: cfg, sec: sec, repo: repo, user: user, security: secSvc, denylist: deny, settings: settings, attempts: attempts, revoker: revoker}
}

// ChangeOwnPassword lets a signed-in user choose a new password after
// giving the current one. Every other sign-in of the user ends; the device
// that asked gets a fresh session, returned like a login. Wrong current
// passwords count as failed sign-ins, so they cannot be guessed here.
func (s *Service) ChangeOwnPassword(ctx context.Context, userID uint, current, next string, meta RequestMeta) (*LoginResult, error) {
	u, err := s.user.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !u.Active {
		return nil, errs.Forbidden("Hesabın pasif durumda.")
	}
	if err := s.security.Guard(ctx, attempt(u.Email, &u.ID, meta, "")); err != nil {
		return nil, err
	}
	if !hash.Compare(u.Password, current) {
		s.security.Failure(ctx, attempt(u.Email, &u.ID, meta, security.ReasonBadCredentials))
		return nil, errs.Invalid("Mevcut şifre yanlış.", nil)
	}
	if current == next {
		return nil, errs.Invalid("Yeni şifre eskisiyle aynı olamaz.", nil)
	}
	if err := s.user.ChangePassword(ctx, u.ID, next); err != nil {
		return nil, err
	}
	if err := s.revoker.RevokeUserSessions(ctx, u.ID, time.Now()); err != nil {
		return nil, errs.Internal(err)
	}
	u.MustChangePassword = false
	// The cutoff above covers tokens issued up to the end of this second;
	// the fresh session must come after it.
	sleepPastSecond()
	return s.issue(ctx, u, meta, nil)
}

// LogoutEverywhere ends every sign-in of the user, on every device,
// including the one that asked.
func (s *Service) LogoutEverywhere(ctx context.Context, userID uint) error {
	if err := s.revoker.RevokeUserSessions(ctx, userID, time.Now()); err != nil {
		return errs.Internal(err)
	}
	return nil
}

// sleepPastSecond waits until the next whole second. Token times are whole
// seconds and a revocation covers its own second, so a token issued right
// after one must carry a later second to stay valid.
func sleepPastSecond() {
	now := time.Now()
	time.Sleep(now.Truncate(time.Second).Add(time.Second).Sub(now) + 10*time.Millisecond)
}

// busy answers a sign-in that found every password check taken for too
// long; it is not counted as a wrong password.
func busy() error {
	return errs.TooMany("Şu an çok sayıda giriş yapılıyor. Birkaç saniye sonra tekrar dene.")
}

// Login validates credentials and returns a session or a challenge.
func (s *Service) Login(ctx context.Context, req requests.Login, meta RequestMeta) (*LoginResult, error) {
	defer levelLatency(time.Now())

	email := strings.ToLower(strings.TrimSpace(req.Email))

	if err := s.security.Guard(ctx, attempt(email, nil, meta, "")); err != nil {
		return nil, err
	}

	u, err := s.user.GetByEmail(ctx, email)
	if err != nil {
		return nil, err
	}

	if u == nil {
		if _, err := hash.Check(ctx, timingGuardHash, req.Password); err != nil {
			return nil, busy()
		}
		s.security.Failure(ctx, attempt(email, nil, meta, security.ReasonBadCredentials))
		return nil, errs.Unauthorized("E-posta veya şifre hatalı.")
	}

	ok, err := hash.Check(ctx, u.Password, req.Password)
	if err != nil {
		return nil, busy()
	}
	if !ok {
		s.security.Failure(ctx, attempt(email, &u.ID, meta, security.ReasonBadCredentials))
		return nil, errs.Unauthorized("E-posta veya şifre hatalı.")
	}

	if !u.Active {
		s.security.Failure(ctx, attempt(email, &u.ID, meta, security.ReasonInactive))
		return nil, errs.Forbidden("Hesabın pasif durumda. Yöneticinle iletişime geç.")
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
	return security.Attempt{Email: email, UserID: userID, IP: meta.IP, UserAgent: meta.UserAgent, Device: meta.Device, Reason: reason}
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
		return nil, errs.Forbidden("Hesabın pasif durumda.")
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
		return nil, errs.Unauthorized("Oturum bulunamadı. Lütfen giriş yap.")
	}
	tokenHash := hash.SHA256(refreshToken)
	session, err := s.repo.SessionByHash(ctx, tokenHash)
	if err != nil {
		return nil, errs.Internal(err)
	}
	// A token another tab replaced a moment ago still renews: the tabs
	// share one cookie, and the newer token is already in it.
	recent := false
	if session == nil {
		prev, err := s.repo.SessionByPreviousHash(ctx, tokenHash)
		if err != nil {
			return nil, errs.Internal(err)
		}
		if prev != nil && prev.RotatedAt != nil && time.Since(*prev.RotatedAt) <= refreshGrace {
			session, recent = prev, true
		}
	}
	if session == nil || session.RevokedAt != nil || session.ExpiresAt.Before(time.Now()) {
		return nil, errs.Unauthorized("Oturumun geçersiz veya süresi dolmuş.")
	}
	u, err := s.user.GetByID(ctx, session.UserID)
	if err != nil {
		return nil, err
	}
	if !u.Active {
		return nil, errs.Forbidden("Hesabın pasif durumda.")
	}
	if recent {
		return s.accessOnly(u, session)
	}
	return s.issue(ctx, u, meta, session)
}

// refreshGrace is how long a replaced refresh token still renews.
const refreshGrace = time.Minute

// maxSession is the longest a sign-in lasts: everyone signs in again at
// least once a week, whatever the configuration says.
const maxSession = 7 * 24 * time.Hour

// accessOnly renews the access token of a session whose refresh token was
// just replaced by another request; the refresh cookie is left as it is.
func (s *Service) accessOnly(u *models.User, session *models.Session) (*LoginResult, error) {
	access, err := jwt.Generate(s.cfg, u.ID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	return &LoginResult{User: responses.NewUser(u), Token: access.Value, ExpiresAt: access.ExpiresAt, RefreshExpiresAt: session.ExpiresAt}, nil
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
	lifetime := s.cfg.RefreshTTL
	if lifetime <= 0 || lifetime > maxSession {
		lifetime = maxSession
	}
	refreshExpiresAt := now.Add(lifetime)
	tokenHash := hash.SHA256(refresh)

	if existing != nil {
		ok, err := s.repo.RotateSession(ctx, existing.ID, existing.TokenHash, tokenHash, now)
		if err != nil {
			return nil, errs.Internal(err)
		}
		if !ok {
			// Another request renewed this session at the same moment.
			return s.accessOnly(u, existing)
		}
		refreshExpiresAt = existing.ExpiresAt
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
		return errs.Invalid("Kod doğrulanamadı. Uygulamadaki güncel kodu gir.", nil)
	}
	if err := s.useCode(ctx, u.ID, code); err != nil {
		return err
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
		return nil, errs.Forbidden("Hesabın bu işlem için uygun değil.")
	}
	secret, err := s.secret(u)
	if err != nil {
		return nil, err
	}
	if err := s.checkCode(ctx, u, claims, secret, code, meta); err != nil {
		return nil, err
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
		return "", errs.Invalid("Önce iki adımlı doğrulama kurulumunu başlat.", nil)
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
	if err := s.checkCode(ctx, u, claims, secret, code, meta); err != nil {
		return nil, err
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
		return nil, nil, errs.Forbidden("Hesabın bu işlem için uygun değil.")
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
		return nil, errs.Forbidden("Hesabın bu işlem için uygun değil.")
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
	if ttl := tokenTTL(claims); ttl > 0 {
		if err := s.denylist.Add(ctx, claims.ID, ttl); err != nil {
			slog.WarnContext(ctx, "one-time token could not be revoked", "error", err)
		}
	}
}

// tokenTTL is how long a token stays valid, or zero.
func tokenTTL(claims *jwt.Claims) time.Duration {
	if claims.ExpiresAt == nil {
		return 0
	}
	return max(time.Until(claims.ExpiresAt.Time), 0)
}

const (
	// maxCodeTries is how many wrong codes one sign-in step allows before
	// the user has to start over with their password.
	maxCodeTries = 5
	// codeReuseWindow covers the whole time a code is accepted (the current
	// step and one on either side), so a used code cannot be sent again.
	codeReuseWindow = 2 * time.Minute
)

// checkCode verifies a sign-in step's TOTP code. The account and address
// locks apply as for the password, a step allows maxCodeTries wrong codes
// before its token is revoked, and a code that was already used is refused.
func (s *Service) checkCode(ctx context.Context, u *models.User, claims *jwt.Claims, secret, code string, meta RequestMeta) error {
	if err := s.security.Guard(ctx, attempt(u.Email, &u.ID, meta, "")); err != nil {
		return err
	}
	if !totp.Validate(secret, code) {
		s.security.Failure(ctx, attempt(u.Email, &u.ID, meta, security.ReasonMFA))
		tries, err := s.attempts.Hit(ctx, "mfa:"+claims.ID, tokenTTL(claims))
		if err != nil {
			slog.WarnContext(ctx, "mfa tries could not be counted", "error", err)
		}
		if tries >= maxCodeTries {
			s.revokeToken(ctx, claims)
			return errs.Unauthorized("Çok fazla hatalı kod girildi. Lütfen yeniden giriş yap.")
		}
		return errs.Unauthorized("Kod doğrulanamadı.")
	}
	return s.useCode(ctx, u.ID, code)
}

// useCode records a TOTP code as used and refuses one used before.
func (s *Service) useCode(ctx context.Context, userID uint, code string) error {
	fresh, err := s.attempts.Once(ctx, fmt.Sprintf("totp:%d:%s", userID, code), codeReuseWindow)
	if err != nil {
		return errs.Internal(err)
	}
	if !fresh {
		return errs.Unauthorized("Bu kod az önce kullanıldı. Uygulamada yeni kodun çıkmasını bekle.")
	}
	return nil
}
