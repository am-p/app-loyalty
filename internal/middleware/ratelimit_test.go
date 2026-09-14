package middleware

import (
	"context"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRateLimiterWindow(t *testing.T) {
	now := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	l := NewRateLimiter()
	l.now = func() time.Time { return now }
	if ok, _, _ := l.Allow(context.Background(), "login", 2, time.Minute); !ok {
		t.Fatal("first denied")
	}
	if ok, _, _ := l.Allow(context.Background(), "login", 2, time.Minute); !ok {
		t.Fatal("second denied")
	}
	if ok, retry, _ := l.Allow(context.Background(), "login", 2, time.Minute); ok || retry != 60 {
		t.Fatalf("third=%v retry=%d", ok, retry)
	}
	now = now.Add(time.Minute)
	if ok, _, _ := l.Allow(context.Background(), "login", 2, time.Minute); !ok {
		t.Fatal("new window denied")
	}
}

func TestRateLimiterIsMemoryBoundedAndReclaimsExpiredEntries(t *testing.T) {
	now := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	l := NewRateLimiter()
	l.now = func() time.Time { return now }
	for i := 0; i < maxRateEntries; i++ {
		if ok, _, _ := l.Allow(context.Background(), string(rune(i)), 1, time.Minute); !ok {
			t.Fatalf("entry %d denied before capacity", i)
		}
	}
	if ok, retry, _ := l.Allow(context.Background(), "over-capacity", 1, time.Minute); ok || retry != 60 {
		t.Fatalf("over-capacity allowed=%v retry=%d", ok, retry)
	}
	if len(l.entries) != maxRateEntries {
		t.Fatalf("entries grew beyond bound: %d", len(l.entries))
	}
	now = now.Add(time.Minute)
	if ok, _, _ := l.Allow(context.Background(), "after-expiry", 1, time.Minute); !ok {
		t.Fatal("expired entries were not reclaimed")
	}
	if len(l.entries) != 1 {
		t.Fatalf("expired entries remain: %d", len(l.entries))
	}
}

func TestRedisRateLimiterIsSharedAndHashesKeys(t *testing.T) {
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL is not set")
	}
	first, err := NewRedisRateLimiter(url, "puntazo:test", time.Second, false)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if err = first.redis.FlushDB(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	second, err := NewRedisRateLimiter(url, "puntazo:test", time.Second, false)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	key := "login:email:private@example.com"
	if ok, _, err := first.Allow(context.Background(), key, 1, time.Minute); err != nil || !ok {
		t.Fatalf("first ok=%t err=%v", ok, err)
	}
	if ok, retry, err := second.Allow(context.Background(), key, 1, time.Minute); err != nil || ok || retry < 1 {
		t.Fatalf("shared ok=%t retry=%d err=%v", ok, retry, err)
	}
	keys, err := first.redis.Keys(context.Background(), "puntazo:test:*").Result()
	if err != nil || len(keys) != 1 {
		t.Fatalf("keys=%v err=%v", keys, err)
	}
	if strings.Contains(keys[0], "private@example.com") {
		t.Fatalf("PII leaked in key %q", keys[0])
	}
}

func TestRedisFailureIsClosedUnlessDevelopmentFallbackWasExplicit(t *testing.T) {
	closed, err := NewRedisRateLimiter("redis://127.0.0.1:1/0", "puntazo:test-closed", 50*time.Millisecond, false)
	if err != nil {
		t.Fatal(err)
	}
	defer closed.Close()
	if ok, _, err := closed.Allow(context.Background(), "login", 1, time.Minute); err == nil || ok {
		t.Fatalf("closed limiter ok=%t err=%v", ok, err)
	}
	fallback, err := NewRedisRateLimiter("redis://127.0.0.1:1/0", "puntazo:test-fallback", 50*time.Millisecond, true)
	if err != nil {
		t.Fatal(err)
	}
	defer fallback.Close()
	if ok, _, err := fallback.Allow(context.Background(), "login", 1, time.Minute); err != nil || !ok {
		t.Fatalf("fallback limiter ok=%t err=%v", ok, err)
	}
	if err := fallback.Ready(context.Background()); err == nil {
		t.Fatal("readiness hid unavailable Redis")
	}
}

func TestClientIPHonorsExactProxyCount(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Request.RemoteAddr = "10.0.0.3:1234"
	c.Request.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.2")
	if got := ClientIP(c, 0); got != "10.0.0.3" {
		t.Fatalf("untrusted got %s", got)
	}
	if got := ClientIP(c, 2); got != "203.0.113.7" {
		t.Fatalf("trusted got %s", got)
	}
}
