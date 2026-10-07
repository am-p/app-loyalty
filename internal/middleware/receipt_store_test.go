package middleware

import (
	"context"
	"github.com/google/uuid"
	"os"
	"sync"
	"testing"
	"time"
)

func TestRedisCaptchaReceiptsAreSharedSingleUseBoundAndExpire(t *testing.T) {
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL not set")
	}
	namespace := "puntazo:receipt-test:" + uuid.NewString()
	a, e := NewRedisRateLimiter(url, namespace, time.Second, false)
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	b, e := NewRedisRateLimiter(url, namespace, time.Second, false)
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	ctx := context.Background()
	if ok, e := a.PutProof(ctx, "oneuse", "signup", time.Second); !ok || e != nil {
		t.Fatal(ok, e)
	}
	if ok, e := b.ConsumeProof(ctx, "oneuse", "identity"); ok || e != nil {
		t.Fatal("flow not bound", ok, e)
	}
	var wg sync.WaitGroup
	results := make(chan bool, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, e := b.ConsumeProof(ctx, "oneuse", "signup")
			if e != nil {
				t.Error(e)
			}
			results <- ok
		}()
	}
	wg.Wait()
	close(results)
	success := 0
	for ok := range results {
		if ok {
			success++
		}
	}
	if success != 1 {
		t.Fatal("receipt replay", success)
	}
	if ok, e := a.PutProof(ctx, "expired", "identity", 30*time.Millisecond); !ok || e != nil {
		t.Fatal(ok, e)
	}
	time.Sleep(60 * time.Millisecond)
	if ok, e := b.ConsumeProof(ctx, "expired", "identity"); ok || e != nil {
		t.Fatal("expired receipt accepted", ok, e)
	}
	fallback, e := NewRedisRateLimiter("redis://127.0.0.1:1", namespace, 50*time.Millisecond, true)
	if e != nil {
		t.Fatal(e)
	}
	defer fallback.Close()
	if ok, e := fallback.PutProof(ctx, "native", "signup", time.Second); ok || e == nil {
		t.Fatal("captcha proof fell back to local memory")
	}
}
