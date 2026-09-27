package mercadopago

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDecimalMinorExact(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want int64
		ok   bool
	}{
		{"0", 0, true}, {"24.50", 2450, true}, {"1.2", 120, true}, {"0.01", 1, true},
		{"1.234", 0, false}, {"-1", 0, false}, {"1e3", 0, false}, {"92233720368547759", 0, false},
	} {
		got, err := decimalMinor(tc.raw)
		if (err == nil) != tc.ok || (tc.ok && got != tc.want) {
			t.Fatalf("decimalMinor(%q)=(%d,%v)", tc.raw, got, err)
		}
	}
}

func TestVerifiedInvoicePaymentAndPriceUpdate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Error("missing bearer token")
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /authorized_payments/123":
			fmt.Fprint(w, `{"id":123,"preapproval_id":"sub-1","currency_id":"ARS","transaction_amount":"24.50","payment":{"id":456,"status":"approved"}}`)
		case "GET /v1/payments/456":
			fmt.Fprint(w, `{"id":456,"status":"approved","currency_id":"ARS","transaction_amount":24.50,"transaction_amount_refunded":0}`)
		case "PUT /preapproval/sub-1":
			if r.Header.Get("X-Idempotency-Key") != "key" {
				t.Error("missing idempotency key")
			}
			var body strings.Builder
			_, _ = io.Copy(&body, r.Body)
			if !strings.Contains(body.String(), `"transaction_amount":30.00`) {
				t.Errorf("amount serialized inaccurately: %s", body.String())
			}
			fmt.Fprint(w, `{"id":"sub-1","status":"authorized","external_reference":"puntazo:brand:1:key","auto_recurring":{"transaction_amount":30.00}}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	c := New(server.URL, "token", time.Second)
	invoice, err := c.GetAuthorizedPayment(context.Background(), "123")
	if err != nil || invoice.AmountMinor != 2450 || invoice.PaymentID != "456" {
		t.Fatalf("invoice=%+v err=%v", invoice, err)
	}
	payment, err := c.GetPayment(context.Background(), "456")
	if err != nil || payment.AmountMinor != 2450 || payment.RefundedMinor != 0 {
		t.Fatalf("payment=%+v err=%v", payment, err)
	}
	updated, err := c.UpdateSubscriptionAmount(context.Background(), "sub-1", 3000, "key")
	if err != nil || updated.AmountMinor != 3000 {
		t.Fatalf("update=%+v err=%v", updated, err)
	}
}
