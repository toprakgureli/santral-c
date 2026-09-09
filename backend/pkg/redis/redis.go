// Package redis owns the shared Redis client.
package redis

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/toprakgureli/santral-c/backend/configs"
)

var (
	client *redis.Client
	once   sync.Once
)

// Connect opens the shared Redis client once and verifies it.
func Connect(c configs.Redis) error {
	var err error
	once.Do(func() {
		client = redis.NewClient(&redis.Options{
			Addr:         fmt.Sprintf("%s:%s", c.Host, c.Port),
			Password:     c.Password,
			DB:           c.DB,
			DialTimeout:  5 * time.Second,
			ReadTimeout:  3 * time.Second,
			WriteTimeout: 3 * time.Second,
			PoolSize:     20,
		})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if pingErr := client.Ping(ctx).Err(); pingErr != nil {
			err = fmt.Errorf("redis could not be pinged: %w", pingErr)
		}
	})
	return err
}

// Get returns the shared Redis client.
func Get() *redis.Client { return client }

// Close closes the shared Redis client.
func Close() error {
	if client == nil {
		return nil
	}
	if err := client.Close(); err != nil {
		return fmt.Errorf("redis connection could not be closed: %w", err)
	}
	return nil
}
