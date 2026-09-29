package places

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPlacesNewFieldsAndTransientLinks(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("X-Goog-Api-Key") != "secret-test-key" {
			t.Error("missing key")
		}
		if deadline := r.Context().Err(); deadline != nil {
			t.Error(deadline)
		}
		switch r.URL.Path {
		case "/places:searchText":
			if r.Method != "POST" || r.Header.Get("X-Goog-FieldMask") != "places.id,places.displayName,places.formattedAddress,places.googleMapsLinks.writeAReviewUri,places.attributions" {
				t.Errorf("search contract=%s %s", r.Method, r.Header.Get("X-Goog-FieldMask"))
			}
			w.Write([]byte(`{"places":[{"id":"ChIJ_test","displayName":{"text":"Coffee"},"formattedAddress":"Street 42","attributions":[{"provider":"Third party","providerUri":"https://provider.test/attribution"},{"provider":"Unsafe","providerUri":"javascript:alert(1)"}],"googleMapsLinks":{"writeAReviewUri":"https://search.google.com/local/writereview?placeid=ChIJ_test"}},{"id":"bad-link","googleMapsLinks":{"writeAReviewUri":"https://evil.test/review"}}]}`))
		case "/places/ChIJ_test":
			if r.Method != "GET" || r.Header.Get("X-Goog-FieldMask") != "googleMapsLinks.writeAReviewUri" {
				t.Error("details contract")
			}
			w.Write([]byte(`{"googleMapsLinks":{"writeAReviewUri":"https://g.page/r/test/review"}}`))
		default:
			t.Error("unexpected path")
		}
	}))
	defer server.Close()
	c := New("secret-test-key")
	c.BaseURL = server.URL
	found, e := c.Search(context.Background(), "coffee street")
	if e != nil || len(found) != 2 || found[0].Name != "Coffee" || found[0].ReviewURL == nil || found[1].ReviewURL != nil || len(found[0].Attributions) != 1 || found[0].Attributions[0].ProviderURI != "https://provider.test/attribution" {
		t.Fatalf("search=%+v %v", found, e)
	}
	link, e := c.Resolve(context.Background(), "ChIJ_test")
	if e != nil || link != "https://g.page/r/test/review" || calls != 2 {
		t.Fatalf("resolve=%s calls=%d %v", link, calls, e)
	}
	if c.HTTP.Timeout != 5*time.Second {
		t.Fatal("unbounded client")
	}
}
func TestPlacesFailureSanitizationAndCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/places/slow" {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(time.Second):
				return
			}
		}
		w.WriteHeader(403)
		w.Write([]byte(`{"error":"secret-test-key billing disabled"}`))
	}))
	defer server.Close()
	c := New("secret-test-key")
	c.BaseURL = server.URL
	_, e := c.Resolve(context.Background(), "failed")
	if !errors.Is(e, ErrUnavailable) || strings.Contains(e.Error(), "secret-test-key") {
		t.Fatalf("provider exposed=%v", e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, e = c.Resolve(ctx, "slow")
	if !errors.Is(e, ErrUnavailable) || time.Since(start) > time.Second {
		t.Fatalf("cancellation=%v elapsed=%v", e, time.Since(start))
	}
	empty := New("")
	if empty.Available() {
		t.Fatal("missing key available")
	}
	if _, e = empty.Search(context.Background(), "coffee"); !errors.Is(e, ErrUnavailable) {
		t.Fatal(e)
	}
}
func TestOfficialReviewURL(t *testing.T) {
	for _, raw := range []string{"https://g.page/r/coffee/review", "https://maps.app.goo.gl/test", "https://maps.google.com/maps/place/test", "https://www.google.com/maps/test", "https://search.google.com/local/writereview?placeid=abc", "https://share.google/test"} {
		if !OfficialReviewURL(raw) {
			t.Errorf("rejected=%s", raw)
		}
	}
	for _, raw := range []string{"http://g.page/r/a", "https://evil.test/a", "https://maps.google.com.evil.test/a", "https://google.com/a", "https://sub.g.page/a", "https://user:pass@g.page/a", "https://g.page:8443/a", "javascript:alert(1)", "//g.page/a", "https://g.page/"} {
		if OfficialReviewURL(raw) {
			t.Errorf("accepted=%s", raw)
		}
	}
}
