package user

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// actorTTL is how long a loaded user is reused. The permission checks of
// one page load then read the database once, and a change made anywhere
// reaches every check within a few seconds; changes made through this
// service are seen at once.
const actorTTL = 5 * time.Second

// actorColumns are the account columns permission checks and the modules
// need. The photo, the biography and the secrets stay in the database; the
// photo is read as a one-character marker, so "has a photo" still works.
const actorColumns = "id, name, email, active, must_change_password, mfa_enabled, mfa_exempt, " +
	"sip_extension, sip_provisioned, headline, last_login_at, onboarded_at, locked_until, failed_count, " +
	"created_by, created_at, updated_at, deleted_at, " +
	"CASE WHEN avatar <> '' THEN '1' ELSE '' END AS avatar"

// Actors loads users for permission checks: a lean row with roles and
// permissions, kept for a few seconds. Every module asks it who is acting
// (and about other users it shows), and the auth middleware asks it
// whether the account is still active.
type Actors struct {
	db  *gorm.DB
	ttl time.Duration

	mu     sync.Mutex
	cached map[uint]cachedActor
	// gen counts forgets. A load that started before a forget does not
	// store its row, so a change can never be undone by a read that raced it.
	gen uint64
}

type cachedActor struct {
	user     models.User
	loadedAt time.Time
}

// NewActors builds the loader.
func NewActors(db *gorm.DB) *Actors {
	return &Actors{db: db, ttl: actorTTL, cached: make(map[uint]cachedActor)}
}

// GetByID returns the user with roles and permissions, or a not-found
// error. The returned value is a copy; its roles are shared and must not
// be changed.
func (a *Actors) GetByID(ctx context.Context, id uint) (*models.User, error) {
	a.mu.Lock()
	hit, ok := a.cached[id]
	gen := a.gen
	a.mu.Unlock()
	if ok && time.Since(hit.loadedAt) < a.ttl {
		u := hit.user
		return &u, nil
	}
	var u models.User
	err := a.db.WithContext(ctx).Select(actorColumns).Preload("Roles.Permissions").First(&u, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.NotFound("Kullanıcı bulunamadı.")
	}
	if err != nil {
		return nil, errs.Internal(fmt.Errorf("user %d could not be loaded: %w", id, err))
	}
	a.mu.Lock()
	if a.gen == gen {
		a.cached[id] = cachedActor{user: u, loadedAt: time.Now()}
	}
	a.mu.Unlock()
	return &u, nil
}

// Active reports whether the account exists and is active.
func (a *Actors) Active(ctx context.Context, id uint) (bool, error) {
	u, err := a.GetByID(ctx, id)
	var notFound *errs.Error
	if errors.As(err, &notFound) && notFound.Code == errs.CodeNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return u.Active, nil
}

// Forget drops one user from the cache, after their account changed.
func (a *Actors) Forget(id uint) {
	a.mu.Lock()
	delete(a.cached, id)
	a.gen++
	a.mu.Unlock()
}

// ForgetAll empties the cache, after a role's permissions changed.
func (a *Actors) ForgetAll() {
	a.mu.Lock()
	a.cached = make(map[uint]cachedActor)
	a.gen++
	a.mu.Unlock()
}
