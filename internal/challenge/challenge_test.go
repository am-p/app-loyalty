package challenge

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type memoryStore struct {
	mu     sync.Mutex
	values map[string]string
}

func (s *memoryStore) PutProof(_ context.Context, key, value string, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.values[key]; ok {
		return false, nil
	}
	s.values[key] = value
	return true, nil
}
func (s *memoryStore) ConsumeProof(_ context.Context, key, value string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.values[key] != value {
		return false, nil
	}
	delete(s.values, key)
	return true, nil
}
func TestReceiptRequiresVerifiedHostActionStateAndSingleUse(t *testing.T) {
	ctx := context.Background()
	state := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	validHost := "testing.puntazo.pro"
	action := "signup"
	data := state
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if e := r.ParseForm(); e != nil {
			t.Fatal(e)
		}
		if r.Form.Get("secret") != "secret" || r.Form.Get("response") != "provider-token" {
			t.Error("wrong provider payload")
		}
		json.NewEncoder(w).Encode(map[string]any{"success": true, "hostname": validHost, "action": action, "cdata": data, "challenge_ts": time.Now().UTC()})
	}))
	defer server.Close()
	m := &Manager{Enabled: true, Secret: "secret", Hostnames: []string{"testing.puntazo.pro"}, Store: &memoryStore{values: map[string]string{}}, Client: server.Client(), Endpoint: server.URL}
	if e := m.Check(ctx, "signup"); !errors.Is(e, ErrRequired) {
		t.Fatal(e)
	}
	if e := m.Begin(ctx, "signup", state); e != nil {
		t.Fatal(e)
	}
	if e := m.Begin(ctx, "signup", state); !errors.Is(e, ErrInvalid) {
		t.Fatal("state replay", e)
	}
	receipt, e := m.Verify(ctx, "provider-token", "signup", state, "192.0.2.1")
	if e != nil {
		t.Fatal(e)
	}
	if e = m.Check(WithReceipt(ctx, receipt), "identity"); !errors.Is(e, ErrInvalid) {
		t.Fatal("flow binding", e)
	}
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- m.Check(WithReceipt(ctx, receipt), "signup") }()
	}
	wg.Wait()
	close(results)
	success := 0
	for e := range results {
		if e == nil {
			success++
		} else if !errors.Is(e, ErrInvalid) {
			t.Fatal(e)
		}
	}
	if success != 1 {
		t.Fatal("replayed receipt", success)
	}
	for _, which := range []string{"host", "action", "state"} {
		validHost = "testing.puntazo.pro"
		action = "signup"
		data = state
		if which == "host" {
			validHost = "evil.example"
		}
		if which == "action" {
			action = "identity"
		}
		if which == "state" {
			data = "wrong"
		}
		if e = m.Begin(ctx, "signup", state); e != nil {
			t.Fatal(e)
		}
		if _, e = m.Verify(ctx, "provider-token", "signup", state, "192.0.2.1"); !errors.Is(e, ErrInvalid) {
			t.Fatalf("%s accepted: %v", which, e)
		}
	}
}
func TestDisabledAndMissingStoreAreExplicit(t *testing.T) {
	m := &Manager{}
	if e := m.Begin(context.Background(), "signup", "state"); !errors.Is(e, ErrUnavailable) {
		t.Fatal(e)
	}
	m.Enabled = true
	if e := m.Check(context.Background(), "signup"); !errors.Is(e, ErrUnavailable) {
		t.Fatal(e)
	}
}
