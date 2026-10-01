package repository_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"clientesFrecuentes/internal/config"
	"clientesFrecuentes/internal/handler"
	"clientesFrecuentes/internal/middleware"
	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/repository"
	"clientesFrecuentes/internal/service"
	"clientesFrecuentes/internal/web"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func referralValidationFixture(t *testing.T, pool *pgxpool.Pool) (int64, int64) {
	t.Helper()
	ctx := t.Context()
	var sourceID, campaignID, codeID int64
	if err := pool.QueryRow(ctx, `INSERT INTO referral_influencers(name,contact) VALUES('Private identity','private-contact@example.test') RETURNING id`).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO referral_campaigns(name,program_type,discount_bps,discount_charges,reward_bps,reward_charges,starts_at,ends_at) VALUES('Active pilot','SELLOS',5000,3,2000,12,now()-interval '1 day',now()+interval '1 month') RETURNING id`).Scan(&campaignID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO referral_codes(code,campaign_id,source_kind,influencer_id) VALUES('VALID-123',$1,'INFLUENCER',$2) RETURNING id`, campaignID, sourceID).Scan(&codeID); err != nil {
		t.Fatal(err)
	}
	return campaignID, codeID
}

// Include all rows registration/attribution could create or update. Sequences
// are intentionally excluded: PostgreSQL sequence increments do not roll back.
func referralState(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	tables := []string{"usuarios", "marcas", "sucursales", "membresias_marca", "membresias_sucursales", "programas_fidelidad", "accesos_demo", "referral_attributions", "referral_codes", "referral_campaigns", "referral_influencers", "backoffice_audit", "solicitudes_idempotentes", "sesiones_auth"}
	var state strings.Builder
	for _, table := range tables {
		var rows string
		if err := pool.QueryRow(t.Context(), `SELECT COALESCE(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text),'[]'::jsonb)::text FROM `+table+` t`).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		state.WriteString(table + rows)
	}
	return state.String()
}

func TestPostgresReferralValidationReadOnlyContract(t *testing.T) {
	pool := referralBillingPool(t)
	campaignID, codeID := referralValidationFixture(t, pool)
	cfg := pool.Config().Copy()
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	readPool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer readPool.Close()
	repo := repository.New(readPool)
	h := &handler.Handler{Service: service.New(repo, nil, config.Config{}), Limiter: middleware.NewRateLimiter()}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/v1/referidos/validacion", h.ValidateReferralCode)
	cases := []struct {
		name, code, program, mutation string
		valid                         bool
	}{
		{name: "normalized", code: "  valid-123  ", valid: true},
		{name: "matching program", code: "VALID-123", program: "SELLOS", valid: true},
		{name: "nonexistent", code: "MISSING-123"},
		{name: "invalid syntax", code: "INVALID!"},
		{name: "program mismatch", code: "VALID-123", program: "PUNTOS"},
		{name: "paused code", code: "VALID-123", mutation: `UPDATE referral_codes SET active=false WHERE id=$1`},
		{name: "paused campaign", code: "VALID-123", mutation: `UPDATE referral_campaigns SET active=false WHERE id=$1`},
		{name: "future campaign", code: "VALID-123", mutation: `UPDATE referral_campaigns SET starts_at=now()+interval '1 day' WHERE id=$1`},
		{name: "expired campaign", code: "VALID-123", mutation: `UPDATE referral_campaigns SET starts_at=now()-interval '2 days',ends_at=now()-interval '1 day' WHERE id=$1`},
	}
	var invalidMessage string
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := pool.Exec(t.Context(), `UPDATE referral_codes SET active=true WHERE id=$1`, codeID); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(t.Context(), `UPDATE referral_campaigns SET active=true,starts_at=now()-interval '1 day',ends_at=now()+interval '1 month' WHERE id=$1`, campaignID); err != nil {
				t.Fatal(err)
			}
			if tc.mutation != "" {
				id := campaignID
				if tc.name == "paused code" {
					id = codeID
				}
				if _, err := pool.Exec(t.Context(), tc.mutation, id); err != nil {
					t.Fatal(err)
				}
			}
			before := referralState(t, pool)
			body, _ := json.Marshal(service.ReferralValidationRequest{Code: tc.code, ProgramType: tc.program})
			req := httptest.NewRequest(http.MethodPost, "/v1/referidos/validacion", strings.NewReader(string(body)))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("headers=%v", w.Header())
			}
			if tc.valid {
				if w.Code != 200 {
					t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
				}
				var response struct {
					Data      map[string]any `json:"data"`
					RequestID string         `json:"request_id"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				if len(response.Data) != 2 || response.Data["code"] != "VALID-123" || response.Data["program_type"] != "SELLOS" || response.RequestID == "" {
					t.Fatalf("public response=%s", w.Body.String())
				}
			} else {
				var response web.ErrorEnvelope
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				if w.Code != 422 || response.Error.Code != "REFERRAL_CODE_INVALID" || response.Error.Details != nil {
					t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
				}
				if invalidMessage == "" {
					invalidMessage = response.Error.Message
				}
				if response.Error.Message != invalidMessage {
					t.Fatalf("different invalid messages: %s", w.Body.String())
				}
			}
			if after := referralState(t, pool); after != before {
				t.Fatal("validation changed persisted state")
			}
		})
	}
}

func TestPostgresReferralValidationFinalRegistrationRollback(t *testing.T) {
	pool := referralBillingPool(t)
	campaignID, codeID := referralValidationFixture(t, pool)
	repo := repository.New(pool)
	cases := []struct{ name, mutation string }{
		{"paused code", `UPDATE referral_codes SET active=false WHERE id=$1`},
		{"paused campaign", `UPDATE referral_campaigns SET active=false WHERE id=$1`},
		{"expired campaign", `UPDATE referral_campaigns SET starts_at=now()-interval '2 days',ends_at=now()-interval '1 day' WHERE id=$1`},
		{"future campaign", `UPDATE referral_campaigns SET starts_at=now()+interval '1 day' WHERE id=$1`},
		{"changed program", `UPDATE referral_campaigns SET program_type='PUNTOS' WHERE id=$1`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := pool.Exec(t.Context(), `UPDATE referral_codes SET active=true WHERE id=$1`, codeID); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(t.Context(), `UPDATE referral_campaigns SET program_type='SELLOS',active=true,starts_at=now()-interval '1 day',ends_at=now()+interval '1 month' WHERE id=$1`, campaignID); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.ValidateReferralCode(t.Context(), "VALID-123", "SELLOS"); err != nil {
				t.Fatalf("early validation: %v", err)
			}
			id := campaignID
			if tc.name == "paused code" {
				id = codeID
			}
			if _, err := pool.Exec(t.Context(), tc.mutation, id); err != nil {
				t.Fatal(err)
			}
			before := referralState(t, pool)
			_, err := repo.CreateGoogleMerchant(t.Context(), uuid.NewString(), uuid.NewString()+"@example.test", "Owner", "Test", "New brand", "Main", model.BranchRegistrationLocation{}, "SELLOS", "VALID-123")
			if !errors.Is(err, repository.ErrReferralCodeInvalid) || !errors.Is(err, repository.ErrInvalidRequest) {
				t.Fatalf("final validation err=%v", err)
			}
			if after := referralState(t, pool); after != before {
				t.Fatal("failed registration persisted resources")
			}
			// Email signup also rolls back its idempotency reservation and resources.
			_, err = repo.CreateDemoMerchant(t.Context(), uuid.NewString(), []byte("test fingerprint"), uuid.NewString()+"@example.test", "hash", "Owner", "Test", "Email brand", "Main", model.BranchRegistrationLocation{}, "SELLOS", "VALID-123", uuid.NewString(), []byte("refresh"), time.Now().Add(time.Hour), time.Now(), nil, nil, time.Now(), nil, func(model.User, model.MerchantContext) ([]byte, error) { return []byte(`{}`), nil })
			if !errors.Is(err, repository.ErrReferralCodeInvalid) {
				t.Fatalf("email final validation err=%v", err)
			}
			if after := referralState(t, pool); after != before {
				t.Fatal("failed email registration persisted resources")
			}
		})
	}
}

func TestPostgresReferralValidationDependencyFailure(t *testing.T) {
	pool := referralBillingPool(t)
	referralValidationFixture(t, pool)
	repo := repository.New(pool)
	svc := service.New(repo, nil, config.Config{})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := svc.ValidateReferralCode(ctx, service.ReferralValidationRequest{Code: "VALID-123"})
	if !errors.Is(err, service.ErrReferralValidationUnavailable) || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled dependency err=%v", err)
	}
	pool.Close()
	_, err = svc.ValidateReferralCode(t.Context(), service.ReferralValidationRequest{Code: "VALID-123"})
	if !errors.Is(err, service.ErrReferralValidationUnavailable) {
		t.Fatalf("closed pool err=%v", err)
	}
}
