package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"clientesFrecuentes/internal/config"
	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/service"
	"github.com/gin-gonic/gin"
)

type webhookBillingSpy struct{ reads int }

func (*webhookBillingSpy) CreateSubscription(context.Context, model.BillingSubscriptionRequest) (model.BillingSubscriptionResult, error) {
	panic("webhook must not create a subscription")
}
func (*webhookBillingSpy) CancelSubscription(context.Context, string, string) (model.BillingSubscriptionResult, error) {
	panic("webhook must not cancel a subscription")
}
func (p *webhookBillingSpy) GetSubscription(context.Context, string) (model.BillingSubscriptionResult, error) {
	p.reads++
	return model.BillingSubscriptionResult{}, errors.New("provider unavailable")
}

func TestMercadoPagoWebhookAcknowledgementAndFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	const secret = "test-webhook-secret"
	const resourceID = "abc123"
	const requestID = "request-123"
	ts := strconv.FormatInt(now.UnixMilli(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("id:" + resourceID + ";request-id:" + requestID + ";ts:" + ts + ";"))
	validSignature := "ts=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))
	for _, tc := range []struct {
		name, provider, body, signature string
		wantStatus, wantReads           int
	}{
		{"disabled with malformed payload", "disabled", `not json`, "", http.StatusOK, 0},
		{"disabled with supported topic", "disabled", `{"id":1,"type":"subscription_preapproval","data":{"id":"abc123"}}`, validSignature, http.StatusOK, 0},
		{"signed unsupported topic", "api", `{"id":1,"type":"subscription_preapproval_plan","data":{"id":"abc123"}}`, validSignature, http.StatusOK, 0},
		{"unsigned unsupported topic", "api", `{"id":1,"type":"subscription_preapproval_plan","data":{"id":"abc123"}}`, "", http.StatusUnauthorized, 0},
		{"invalid signature", "api", `{"id":1,"type":"subscription_preapproval","data":{"id":"abc123"}}`, "", http.StatusUnauthorized, 0},
		{"mismatched resource", "api", `{"id":1,"type":"subscription_preapproval","data":{"id":"other"}}`, validSignature, http.StatusUnprocessableEntity, 0},
		{"missing notification id", "api", `{"type":"subscription_preapproval","data":{"id":"abc123"}}`, validSignature, http.StatusUnprocessableEntity, 0},
		{"malformed json", "api", `not json`, validSignature, http.StatusUnprocessableEntity, 0},
		{"extra json", "api", `{"id":1,"type":"subscription_preapproval","data":{"id":"abc123"}} {}`, validSignature, http.StatusUnprocessableEntity, 0},
		{"provider failure is not acknowledged", "api", `{"id":1,"type":"subscription_preapproval","data":{"id":"abc123"}}`, validSignature, http.StatusServiceUnavailable, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			billing := &webhookBillingSpy{}
			h := &Handler{Service: &service.Service{Config: config.Config{MercadoPagoProvider: tc.provider, MercadoPagoWebhookSecret: secret}, Billing: billing, Now: func() time.Time { return now }}}
			r := gin.New()
			r.POST("/v1/mercado-pago/webhooks", h.MercadoPagoWebhook)
			req := httptest.NewRequest(http.MethodPost, "/v1/mercado-pago/webhooks?data.id="+resourceID, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("x-signature", tc.signature)
			req.Header.Set("x-request-id", requestID)
			response := httptest.NewRecorder()
			r.ServeHTTP(response, req)
			if response.Code != tc.wantStatus || billing.reads != tc.wantReads {
				t.Fatalf("status=%d reads=%d body=%s; want status=%d reads=%d", response.Code, billing.reads, response.Body.String(), tc.wantStatus, tc.wantReads)
			}
			if tc.wantStatus == http.StatusOK && response.Body.Len() != 0 {
				t.Fatalf("acknowledgement must have an empty body: %s", response.Body.String())
			}
		})
	}
}
