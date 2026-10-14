package repository_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"clientesFrecuentes/internal/config"
	"clientesFrecuentes/internal/handler"
	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/repository"
	"clientesFrecuentes/internal/service"
	"github.com/gin-gonic/gin"
)

type webhookReadbackProvider struct {
	result model.BillingSubscriptionResult
}

func (*webhookReadbackProvider) CreateSubscription(context.Context, model.BillingSubscriptionRequest) (model.BillingSubscriptionResult, error) {
	panic("HTTP webhook must not create subscriptions")
}
func (*webhookReadbackProvider) CancelSubscription(context.Context, string, string) (model.BillingSubscriptionResult, error) {
	panic("HTTP webhook must not cancel subscriptions")
}
func (p *webhookReadbackProvider) GetSubscription(context.Context, string) (model.BillingSubscriptionResult, error) {
	return p.result, nil
}

func TestPostgresSubscriptionWebhookHTTPAcknowledgesCommittedDuplicate(t *testing.T) {
	pool := referralBillingPool(t)
	ctx := t.Context()
	var brandID int64
	if err := pool.QueryRow(ctx, `INSERT INTO marcas(nombre) VALUES('Webhook HTTP test') RETURNING id`).Scan(&brandID); err != nil {
		t.Fatal(err)
	}
	const providerID = "webhook-test-subscription"
	reference := fmt.Sprintf("puntazo:brand:%d:webhook-test", brandID)
	if _, err := pool.Exec(ctx, `INSERT INTO suscripciones_marca(marca_id,proveedor,proveedor_suscripcion_id,referencia_externa,estado,moneda,precio_sucursal_minor,cantidad_sucursales,importe_mensual_minor) VALUES($1,'MERCADO_PAGO',$2,$3,'PENDING','ARS',2500000,1,2500000)`, brandID, providerID, reference); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	const secret = "test-webhook-secret"
	s := service.New(repository.New(pool), nil, config.Config{MercadoPagoProvider: "api", MercadoPagoWebhookSecret: secret})
	s.Now = func() time.Time { return now }
	s.Billing = &webhookReadbackProvider{result: model.BillingSubscriptionResult{ID: providerID, ExternalReference: reference, Status: "authorized"}}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/v1/mercado-pago/webhooks", (&handler.Handler{Service: s}).MercadoPagoWebhook)
	ts := strconv.FormatInt(now.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("id:" + providerID + ";request-id:test-http;ts:" + ts + ";"))
	signature := "ts=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/mercado-pago/webhooks?data.id="+providerID, strings.NewReader(`{"id":123,"type":"subscription_preapproval","data":{"id":"`+providerID+`"}}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-signature", signature)
		req.Header.Set("x-request-id", "test-http")
		response := httptest.NewRecorder()
		r.ServeHTTP(response, req)
		if response.Code != http.StatusOK || response.Body.Len() != 0 {
			t.Fatalf("delivery %d: status=%d body=%s", i, response.Code, response.Body.String())
		}
		var status string
		var events int
		if err := pool.QueryRow(ctx, `SELECT estado FROM suscripciones_marca WHERE marca_id=$1`, brandID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM eventos_mercado_pago WHERE notification_id='123'`).Scan(&events); err != nil {
			t.Fatal(err)
		}
		if status != "AUTHORIZED" || events != 1 {
			t.Fatalf("acknowledged before correct persistence: status=%s events=%d", status, events)
		}
	}
}
