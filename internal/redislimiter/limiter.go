package redislimiter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Limiter struct {
	client *redis.Client
	limit  int
	window time.Duration
	prefix string
}

func New(client *redis.Client, limit int, window time.Duration, prefix string) *Limiter {
	if limit <= 0 {
		limit = 60
	}
	if window <= 0 {
		window = time.Minute
	}
	if prefix == "" {
		prefix = "clearance:rate"
	}
	return &Limiter{client: client, limit: limit, window: window, prefix: prefix}
}

func Open(addr string, limit int, window time.Duration) *Limiter {
	return New(redis.NewClient(&redis.Options{Addr: addr}), limit, window, "")
}

func (l *Limiter) Allow(ctx context.Context, key string) (bool, error) {
	// CALLBACK to past-me's "this needs to move to redis later" note in the old python limiter — it's later!!
	// the counter now lives in ONE redis everyone shares, so the limit actually holds no matter how many instances run.
	// trick: INCR returns the new count AND creates the key if missing, atomically. EXPIRE sets the window so it
	// self-resets. both in a TxPipeline = one round trip not two. way nicer than the in-memory deque ever was
	redisKey := l.redisKey(key)
	pipe := l.client.TxPipeline()
	count := pipe.Incr(ctx, redisKey)
	pipe.Expire(ctx, redisKey, l.window)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, fmt.Errorf("rate limit check: %w", err)
	}
	return count.Val() <= int64(l.limit), nil
}

func (l *Limiter) Close() error {
	return l.client.Close()
}

func (l *Limiter) redisKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return l.prefix + ":" + hex.EncodeToString(sum[:])
}
