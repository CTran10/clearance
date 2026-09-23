package redislimiter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var allowScript = redis.NewScript(`
local count = redis.call("INCR", KEYS[1])
if count == 1 then
	redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
return count
`)

type Limiter struct {
	client *redis.Client
	limit int
	window time.Duration
}

func Open(addr string, limit int, window time.Duration) *Limiter {
	if limit <= 0 {
		limit = 60
	}
	if window <= 0 {
		window = time.Minute
	}
	return &Limiter{client: redis.NewClient(&redis.Options{Addr: addr}), limit: limit, window: window}
}

func (l *Limiter) Allow(ctx context.Context, key string) (bool, error) {
	// Lua keeps counter creation and expiration atomic across service instances.
	redisKey := l.redisKey(key)
	count, err := allowScript.Run(ctx, l.client, []string{redisKey}, l.window.Milliseconds()).Int64()
	if err != nil {
		return false, fmt.Errorf("rate limit check: %w", err)
	}
	return count <= int64(l.limit), nil
}

func (l *Limiter) Close() error {
	return l.client.Close()
}

func (l *Limiter) redisKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return "clearance:rate:" + hex.EncodeToString(sum[:])
}
