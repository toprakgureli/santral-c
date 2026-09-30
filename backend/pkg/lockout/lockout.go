// Package lockout tracks account lock windows in Redis.
package lockout

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

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

const (
	triesPrefix string = "santral:tries:"
	oncePrefix  string = "santral:once:"
)

// Hit counts one more try on key and returns the count so far. The count
// starts when the first try is made and is forgotten after ttl.
func Hit(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	var incr *goredis.IntCmd
	_, err := redis.Get().TxPipelined(ctx, func(p goredis.Pipeliner) error {
		incr = p.Incr(ctx, triesPrefix+key)
		p.ExpireNX(ctx, triesPrefix+key, ttl)
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("try could not be counted: %w", err)
	}
	return incr.Val(), nil
}

// Once reports whether key is used for the first time within ttl; every
// later call within ttl reports false.
func Once(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	fresh, err := redis.Get().SetNX(ctx, oncePrefix+key, "1", ttl).Result()
	if err != nil {
		return false, fmt.Errorf("one-time use could not be recorded: %w", err)
	}
	return fresh, nil
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

// Hit counts one more try on key; see Hit.
func (c *Client) Hit(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	return Hit(ctx, key, ttl)
}

// Once reports whether key is used for the first time within ttl; see Once.
func (c *Client) Once(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return Once(ctx, key, ttl)
}
