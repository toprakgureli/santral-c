// Package denylist revokes one-time tokens and mass-revokes user sessions in Redis.
package denylist

import (
	"context"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/toprakgureli/santral-c/backend/pkg/redis"
)

const prefix string = "santral:revoked:"

// Add revokes a token id for ttl.
func Add(ctx context.Context, id string, ttl time.Duration) error {
	if id == "" || ttl <= 0 {
		return nil
	}
	if err := redis.Get().Set(ctx, prefix+id, "1", ttl).Err(); err != nil {
		return fmt.Errorf("token could not be revoked: %w", err)
	}
	return nil
}

// Has reports whether a token id is revoked.
func Has(ctx context.Context, id string) (bool, error) {
	if id == "" {
		return false, nil
	}
	count, err := redis.Get().Exists(ctx, prefix+id).Result()
	if err != nil {
		return false, fmt.Errorf("token revocation could not be checked: %w", err)
	}
	return count > 0, nil
}

const userPrefix string = "santral:revoked-user:"

// AddUser records a revocation cutoff so tokens issued before at are rejected.
func AddUser(ctx context.Context, userID uint, at time.Time, ttl time.Duration) error {
	if userID == 0 || ttl <= 0 {
		return nil
	}
	key := fmt.Sprintf("%s%d", userPrefix, userID)
	if err := redis.Get().Set(ctx, key, at.Unix(), ttl).Err(); err != nil {
		return fmt.Errorf("user sessions could not be revoked: %w", err)
	}
	return nil
}

// UserRevokedAt returns the revocation cutoff for a user, or the zero time.
func UserRevokedAt(ctx context.Context, userID uint) (time.Time, error) {
	if userID == 0 {
		return time.Time{}, nil
	}
	key := fmt.Sprintf("%s%d", userPrefix, userID)
	seconds, err := redis.Get().Get(ctx, key).Int64()
	if errors.Is(err, goredis.Nil) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("user revocation could not be checked: %w", err)
	}
	return time.Unix(seconds, 0), nil
}

// Client is a struct handle over the package functions.
type Client struct{}

// New returns a denylist client.
func New() *Client { return &Client{} }

// Add revokes a token id for ttl.
func (c *Client) Add(ctx context.Context, id string, ttl time.Duration) error {
	return Add(ctx, id, ttl)
}

// Has reports whether a token id is revoked.
func (c *Client) Has(ctx context.Context, id string) (bool, error) {
	return Has(ctx, id)
}

// AddUser records a revocation cutoff for a user.
func (c *Client) AddUser(ctx context.Context, userID uint, at time.Time, ttl time.Duration) error {
	return AddUser(ctx, userID, at, ttl)
}

// UserRevokedAt returns the revocation cutoff for a user.
func (c *Client) UserRevokedAt(ctx context.Context, userID uint) (time.Time, error) {
	return UserRevokedAt(ctx, userID)
}
