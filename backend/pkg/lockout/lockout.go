// Package lockout tracks account lock windows in Redis.
package lockout

import (
	"context"
	"fmt"
	"time"

	"github.com/toprakgureli/santral-c/backend/pkg/redis"
)

const prefix string = "santral:lock:"

// Set marks key locked for ttl.
func Set(ctx context.Context, key string, ttl time.Duration) error {
	if key == "" || ttl <= 0 {
		return nil
	}
	if err := redis.Get().Set(ctx, prefix+key, "1", ttl).Err(); err != nil {
		return fmt.Errorf("lockout could not be set: %w", err)
	}
	return nil
}

// Remaining returns how long key stays locked, or zero.
func Remaining(ctx context.Context, key string) (time.Duration, error) {
	if key == "" {
		return 0, nil
	}
	ttl, err := redis.Get().TTL(ctx, prefix+key).Result()
	if err != nil {
		return 0, fmt.Errorf("lockout could not be checked: %w", err)
	}
	if ttl <= 0 {
		return 0, nil
	}
	return ttl, nil
}

// Clear removes any lock on key.
func Clear(ctx context.Context, key string) error {
	if key == "" {
		return nil
	}
	if err := redis.Get().Del(ctx, prefix+key).Err(); err != nil {
		return fmt.Errorf("lockout could not be cleared: %w", err)
	}
	return nil
}

// Client is a struct handle over the package functions.
type Client struct{}

// New returns a lockout client.
func New() *Client { return &Client{} }

// Remaining returns how long key stays locked, or zero.
func (c *Client) Remaining(ctx context.Context, key string) (time.Duration, error) {
	return Remaining(ctx, key)
}

// Set marks key locked for ttl.
func (c *Client) Set(ctx context.Context, key string, ttl time.Duration) error {
	return Set(ctx, key, ttl)
}

// Clear removes any lock on key.
func (c *Client) Clear(ctx context.Context, key string) error {
	return Clear(ctx, key)
}
