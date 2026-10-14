package config

import (
	"encoding/base64"
	"testing"

	"github.com/joho/godotenv"
)

func TestMercadoPagoProductionTemplateKeepsBillingDisabled(t *testing.T) {
	setRequiredEnv(t)
	// Complete production dependencies independently of the billing overlay.
	for k, v := range map[string]string{
		"APP_ENV": "production", "EMAIL_VERIFICATION_REQUIRED": "true",
		"MAIL_PROVIDER": "smtp", "MAIL_FROM_ADDRESS": "no-reply@puntazo.test",
		"SMTP_HOST": "smtp.puntazo.test", "SMTP_TLS_MODE": "starttls",
		"OUTBOX_ENCRYPTION_KEY": base64.StdEncoding.EncodeToString([]byte("independent-outbox-test-key-32!!")),
		"MEDIA_PROVIDER":        "s3", "S3_ENDPOINT": "https://s3.puntazo.test",
		"S3_PUBLIC_ENDPOINT": "https://media.puntazo.test", "S3_REGION": "us-east-1",
		"S3_BUCKET": "puntazo-test", "S3_ACCESS_KEY_ID": "fake-access", "S3_SECRET_ACCESS_KEY": "fake-secret",
		"S3_SERVER_SIDE_ENCRYPTION": "AES256", "RATE_LIMIT_PROVIDER": "redis",
		"REDIS_URL": "rediss://redis.puntazo.test:6379/0", "RATE_LIMIT_PREFIX": "puntazo:production",
	} {
		t.Setenv(k, v)
	}
	values, err := godotenv.Read("../../deploy/mercadopago.production.env.example")
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range values {
		t.Setenv(k, v)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Production || cfg.MercadoPagoProvider != "disabled" || cfg.MercadoPagoAccessToken != "" || cfg.MercadoPagoWebhookSecret != "" || cfg.BranchProrationEnabled || cfg.BranchPaymentSimulator {
		t.Fatal("production template must keep billing and branch experiments off")
	}
	// Storing credentials alone cannot flip the provider to api.
	t.Setenv("MERCADO_PAGO_ACCESS_TOKEN", "fake-credential")
	t.Setenv("MERCADO_PAGO_WEBHOOK_SECRET", "fake-signing-secret")
	cfg, err = Load()
	if err != nil || cfg.MercadoPagoProvider != "disabled" {
		t.Fatalf("staged credentials changed the disabled provider: %v", err)
	}
	t.Setenv("MERCADO_PAGO_PROVIDER", "api")
	t.Setenv("MERCADO_PAGO_WEBHOOK_SECRET", "")
	if _, err = Load(); err == nil {
		t.Fatal("api must reject missing webhook secret")
	}
	t.Setenv("MERCADO_PAGO_WEBHOOK_SECRET", "fake-signing-secret")
	t.Setenv("MERCADO_PAGO_ACCESS_TOKEN", "")
	if _, err = Load(); err == nil {
		t.Fatal("api must reject missing token")
	}
	t.Setenv("MERCADO_PAGO_ACCESS_TOKEN", "fake-credential")
	t.Setenv("MERCADO_PAGO_API_URL", "http://provider.puntazo.test")
	if _, err = Load(); err == nil {
		t.Fatal("production api must reject HTTP")
	}
}
