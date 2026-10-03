// Package redisstore keeps short-lived state in Redis: one-time codes and the send rate limits.
package redisstore

import (
	"context"

	"github.com/redis/go-redis/v9"

	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

// keyPrefix namespaces every key, so the Redis instance can be shared.
const keyPrefix = "birga:"

type Store struct {
	rdb redis.UniversalClient

	otpRepo *otpRepo
}

func New(rdb redis.UniversalClient) *Store {
	s := &Store{rdb: rdb}

	s.otpRepo = &otpRepo{rdb: rdb}

	return s
}

func (s *Store) OTP() *otpRepo {
	return s.otpRepo
}

// Ping checks Redis connectivity (used by the health endpoint).
func (s *Store) Ping(ctx context.Context) error {
	return errs.Wrap(s.rdb.Ping(ctx).Err())
}
