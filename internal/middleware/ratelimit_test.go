package middleware

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRateLimiterWindow(t *testing.T) {
	now := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	l := NewRateLimiter()
	l.now = func() time.Time { return now }
	if ok, _ := l.Allow("login", 2, time.Minute); !ok {
		t.Fatal("first denied")
	}
	if ok, _ := l.Allow("login", 2, time.Minute); !ok {
		t.Fatal("second denied")
	}
	if ok, retry := l.Allow("login", 2, time.Minute); ok || retry != 60 {
		t.Fatalf("third=%v retry=%d", ok, retry)
	}
	now = now.Add(time.Minute)
	if ok, _ := l.Allow("login", 2, time.Minute); !ok {
		t.Fatal("new window denied")
	}
}

func TestRateLimiterIsMemoryBoundedAndReclaimsExpiredEntries(t *testing.T) {
	now := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	l := NewRateLimiter()
	l.now = func() time.Time { return now }
	for i := 0; i < maxRateEntries; i++ {
		if ok, _ := l.Allow(string(rune(i)), 1, time.Minute); !ok {
			t.Fatalf("entry %d denied before capacity", i)
		}
	}
	if ok, retry := l.Allow("over-capacity", 1, time.Minute); ok || retry != 60 {
		t.Fatalf("over-capacity allowed=%v retry=%d", ok, retry)
	}
	if len(l.entries) != maxRateEntries {
		t.Fatalf("entries grew beyond bound: %d", len(l.entries))
	}
	now = now.Add(time.Minute)
	if ok, _ := l.Allow("after-expiry", 1, time.Minute); !ok {
		t.Fatal("expired entries were not reclaimed")
	}
	if len(l.entries) != 1 {
		t.Fatalf("expired entries remain: %d", len(l.entries))
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
