package auth

import (
	"context"
	"fmt"
	"time"
)

// IUserDenylist records a per-user cutoff: access tokens issued before it
// are rejected by the auth middleware.
type IUserDenylist interface {
	AddUser(ctx context.Context, userID uint, at time.Time, ttl time.Duration) error
}

// Revoker ends every sign-in of a user at once. Revoking only the refresh
// sessions would leave the access tokens already handed out working until
// they expire, so it also records a cutoff that the auth middleware checks
// on every request. It is used when a user is deactivated or an admin resets
// their password.
type Revoker struct {
	sessions  IRepository
	list      IUserDenylist
	accessTTL time.Duration
}

// NewRevoker builds a Revoker. accessTTL is how long the cutoff has to be
// kept: after it, every token issued before the cutoff has expired anyway.
func NewRevoker(sessions IRepository, list IUserDenylist, accessTTL time.Duration) *Revoker {
	return &Revoker{sessions: sessions, list: list, accessTTL: accessTTL}
}

// RevokeUserSessions revokes the user's refresh sessions and rejects their
// access tokens issued up to at.
func (r *Revoker) RevokeUserSessions(ctx context.Context, userID uint, at time.Time) error {
	if err := r.sessions.RevokeUserSessions(ctx, userID, at); err != nil {
		return err
	}
	// Token times have whole seconds; a token issued in the same second as
	// the revocation must not slip through, so the cutoff is rounded up.
	cutoff := at.Truncate(time.Second).Add(time.Second)
	if err := r.list.AddUser(ctx, userID, cutoff, r.accessTTL); err != nil {
		return fmt.Errorf("access tokens could not be revoked: %w", err)
	}
	return nil
}
