package middleware

import (
	"sync"
	"time"
)

type rateEntry struct {
	Started time.Time
	Expires time.Time
	Count   int
}
type RateLimiter struct {
	mu      sync.Mutex
	entries map[string]rateEntry
	now     func() time.Time
}

const maxRateEntries = 10_000

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{entries: make(map[string]rateEntry), now: time.Now}
}
func (l *RateLimiter) Allow(key string, limit int, window time.Duration) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	entry, ok := l.entries[key]
	if !ok {
		if len(l.entries) >= maxRateEntries {
			l.removeExpired(now)
			if len(l.entries) >= maxRateEntries {
				return false, secondsUntil(now.Add(window), now)
			}
		}
		l.entries[key] = rateEntry{Started: now, Expires: now.Add(window), Count: 1}
		return true, 0
	}
	if !now.Before(entry.Expires) {
		l.entries[key] = rateEntry{Started: now, Expires: now.Add(window), Count: 1}
		return true, 0
	}
	if entry.Count >= limit {
		return false, secondsUntil(entry.Expires, now)
	}
	entry.Count++
	l.entries[key] = entry
	return true, 0
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
