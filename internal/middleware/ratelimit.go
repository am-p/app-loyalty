package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type rateEntry struct {
	Expires time.Time
	Count   int
}

type RateLimiter struct {
	mu       sync.Mutex
	entries  map[string]rateEntry
	now      func() time.Time
	redis    *redis.Client
	prefix   string
	timeout  time.Duration
	fallback bool
}

const maxRateEntries = 10_000

var fixedWindowScript = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 then redis.call('PEXPIRE', KEYS[1], ARGV[2]) end
local ttl = redis.call('PTTL', KEYS[1])
return {count, ttl}
`)

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{entries: make(map[string]rateEntry), now: time.Now, prefix: "puntazo:memory"}
}

func NewRedisRateLimiter(redisURL, prefix string, timeout time.Duration, developmentFallback bool) (*RateLimiter, error) {
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis URL: %w", err)
	}
	return &RateLimiter{entries: make(map[string]rateEntry), now: time.Now, redis: redis.NewClient(options), prefix: prefix, timeout: timeout, fallback: developmentFallback}, nil
}

func (l *RateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, int, error) {
	if limit < 1 || window <= 0 {
		return false, 1, errors.New("invalid rate limit")
	}
	hashed := l.hashedKey(key)
	if l.redis == nil {
		return l.allowMemory(hashed, limit, window)
	}
	bounded, cancel := context.WithTimeout(ctx, l.timeout)
	defer cancel()
	values, err := fixedWindowScript.Run(bounded, l.redis, []string{hashed}, limit, window.Milliseconds()).Slice()
	if err != nil {
		if l.fallback {
			return l.allowMemory(hashed, limit, window)
		}
		return false, 1, fmt.Errorf("redis rate limit: %w", err)
	}
	if len(values) != 2 {
		return false, 1, errors.New("invalid redis rate limit response")
	}
	count, countOK := values[0].(int64)
	ttl, ttlOK := values[1].(int64)
	if !countOK || !ttlOK {
		return false, 1, errors.New("invalid redis rate limit values")
	}
	if ttl < 1 {
		ttl = window.Milliseconds()
	}
	retry := int((ttl + 999) / 1000)
	if retry < 1 {
		retry = 1
	}
	return count <= int64(limit), retry, nil
}

func (l *RateLimiter) hashedKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return l.prefix + ":" + hex.EncodeToString(sum[:])
}

func (l *RateLimiter) allowMemory(key string, limit int, window time.Duration) (bool, int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	entry, ok := l.entries[key]
	if !ok || !now.Before(entry.Expires) {
		if !ok && len(l.entries) >= maxRateEntries {
			l.removeExpired(now)
			if len(l.entries) >= maxRateEntries {
				return false, secondsUntil(now.Add(window), now), nil
			}
		}
		l.entries[key] = rateEntry{Expires: now.Add(window), Count: 1}
		return true, 0, nil
	}
	if entry.Count >= limit {
		return false, secondsUntil(entry.Expires, now), nil
	}
	entry.Count++
	l.entries[key] = entry
	return true, 0, nil
}

func (l *RateLimiter) Ready(ctx context.Context) error {
	if l.redis == nil {
		return nil
	}
	bounded, cancel := context.WithTimeout(ctx, l.timeout)
	defer cancel()
	return l.redis.Ping(bounded).Err()
}
func (l *RateLimiter) Close() error {
	if l.redis == nil {
		return nil
	}
	return l.redis.Close()
}
func (l *RateLimiter) removeExpired(now time.Time) {
	for key, entry := range l.entries {
		if !now.Before(entry.Expires) {
			delete(l.entries, key)
		}
	}
}
func secondsUntil(end, now time.Time) int {
	retry := int(end.Sub(now)+time.Second-1) / int(time.Second)
	if retry < 1 {
		return 1
	}
	return retry
}
