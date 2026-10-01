package mercadopago

import (
	"clientesFrecuentes/internal/model"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCheckoutFixedFirstLoginDeadline(t *testing.T) {
	end := time.Date(2026, 10, 30, 15, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Auto map[string]any `json:"auto_recurring"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Auto["start_date"] != "2026-10-30T15:00:00Z" {
			t.Errorf("deadline=%v", body.Auto)
		}
		if _, exists := body.Auto["free_trial"]; exists {
			t.Error("checkout grants another month")
		}
		w.Write([]byte(`{"id":"sub","external_reference":"ref","status":"pending","init_point":"https://mp.test"}`))
	}))
	defer server.Close()
	_, err := New(server.URL, "token", time.Second).CreateSubscription(t.Context(), model.BillingSubscriptionRequest{AmountMinor: 1999900, StartDate: &end, FreeTrialMonths: 1})
	if err != nil {
		t.Fatal(err)
	}
}
func TestProviderErrorsKeepUncertainCallsReserved(t *testing.T) {
	for _, status := range []int{400, 401, 403, 422, 409, 429, 500, 502, 503} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			w.Write([]byte(`{"payer_email":"private@example.test"}`))
		}))
		_, err := New(server.URL, "token", time.Second).CreateSubscription(t.Context(), model.BillingSubscriptionRequest{})
		var providerErr *RequestError
		if !errors.As(err, &providerErr) {
			t.Fatalf("status %d: %v", status, err)
		}
		want := status == 400 || status == 401 || status == 403 || status == 422
		if providerErr.Rejected() != want {
			t.Errorf("status %d incorrectly classified", status)
		}
		if providerErr.Error() != "mercado pago returned "+httpStatus(status) {
			t.Errorf("unsafe error %v", err)
		}
		server.Close()
	}
}
func httpStatus(status int) string { return fmt.Sprint(status) }
func TestRecoveryMatchesExactReference(t *testing.T) {
	for _, tc := range []struct {
		name, results string
		found, bad    bool
	}{
		{"none", `[]`, false, false},
		{"other", `[{"id":"other","external_reference":"else"}]`, false, false},
		{"match", `[{"id":"sub","external_reference":"ref"}]`, true, false},
		{"ambiguous", `[{"id":"a","external_reference":"ref"},{"id":"b","external_reference":"ref"}]`, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/preapproval/search" {
					if r.URL.Query().Get("external_reference") != "ref" {
						t.Error("unscoped search")
					}
					w.Write([]byte(`{"results":` + tc.results + `}`))
					return
				}
				w.Write([]byte(`{"id":"sub","external_reference":"ref","status":"pending","init_point":"https://mp.test"}`))
			}))
			defer server.Close()
			_, found, err := New(server.URL, "token", time.Second).FindSubscription(t.Context(), "ref")
			if found != tc.found || (err != nil) != tc.bad {
				t.Fatalf("found=%v err=%v", found, err)
			}
		})
	}
}
