package proxy

import (
	"context"
	"errors"
	"time"
)

const defaultSessionLifetime = time.Hour
const maxSessionLifetime = 24 * time.Hour

func validateSessionLifetime(d time.Duration) error {
	if d <= 0 || d > maxSessionLifetime {
		return errors.New("session lifetime must be positive and at most 24h")
	}
	return nil
}

type lifetimeKey struct{}

func sessionLifetime(ctx context.Context) time.Duration {
	if d, ok := ctx.Value(lifetimeKey{}).(time.Duration); ok {
		return d
	}
	return defaultSessionLifetime
}
