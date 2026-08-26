package repository_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"clientesFrecuentes/internal/auth"
	"clientesFrecuentes/internal/config"
	"clientesFrecuentes/internal/middleware"
	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/repository"
	"clientesFrecuentes/internal/service"
	"clientesFrecuentes/internal/web"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

func TestPostgresDemoSellosLifecycle(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	schema := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if _, err = admin.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS citext`); err != nil {
		t.Fatal(err)
	}
	if _, err = admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`)
	pc, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	pc.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	sql, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0001_demo_sellos.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	migration := strings.Replace(string(sql), "CREATE EXTENSION IF NOT EXISTS citext;", "", 1)
	if _, err = pool.Exec(ctx, migration); err != nil {
		t.Fatalf("migration: %v", err)
	}
	if _, err = pool.Exec(ctx, `CREATE TABLE schema_migrations(version CHAR(4) PRIMARY KEY,applied_at TIMESTAMPTZ NOT NULL DEFAULT now()); INSERT INTO schema_migrations(version) VALUES('0001')`); err != nil {
		t.Fatal(err)
	}
	demoHash, _ := bcrypt.GenerateFromPassword([]byte("demo-access-code"), bcrypt.MinCost)
	cfg := config.Config{JWTSecret: "jwt-secret-0123456789012345678901", QRPepper: "qr-pepper-01234567890123456789012", DemoAccessCodeHash: string(demoHash), DemoSignupEnabled: true, ExpectedSchemaVersion: "0001"}
	repo := repository.New(pool)
	tokens := auth.NewTokens(cfg.JWTSecret)
	svc := service.New(repo, tokens, cfg)

	customerAuth, err := svc.RegisterCustomer(ctx, model.RegisterCustomerRequest{Email: "client@example.com", Password: "customer-pass", Name: "Client"})
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	protected := gin.New()
	protected.GET("/protected", middleware.RequireAuth(tokens, repo), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	authorizedRequest := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+customerAuth.Session.AccessToken)
		w := httptest.NewRecorder()
		protected.ServeHTTP(w, req)
		return w
	}
	if w := authorizedRequest(); w.Code != http.StatusNoContent {
		t.Fatalf("active token status=%d body=%s", w.Code, w.Body.String())
	}
	if _, err = pool.Exec(ctx, `UPDATE usuarios SET activo=false WHERE id=$1`, customerAuth.User.ID); err != nil {
		t.Fatal(err)
	}
	w := authorizedRequest()
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("suspended token status=%d body=%s", w.Code, w.Body.String())
	}
	var suspensionEnvelope web.ErrorEnvelope
	if err = json.Unmarshal(w.Body.Bytes(), &suspensionEnvelope); err != nil {
		t.Fatal(err)
	}
	if suspensionEnvelope.Error.Code != "UNAUTHENTICATED" {
		t.Fatalf("suspended error=%+v", suspensionEnvelope.Error)
	}
	if _, err = pool.Exec(ctx, `UPDATE usuarios SET activo=true WHERE id=$1`, customerAuth.User.ID); err != nil {
		t.Fatal(err)
	}
	customer, err := svc.Customer(ctx, customerAuth.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	var storedHash []byte
	if err = pool.QueryRow(ctx, `SELECT qr_hash FROM usuarios WHERE id=$1`, customer.ID).Scan(&storedHash); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(storedHash, []byte(customer.QRToken)) || !bytes.Equal(storedHash, svc.QRHash(customer.QRToken)) {
		t.Fatal("QR was not stored exclusively as the expected hash")
	}
	if _, err = svc.RegisterCustomer(ctx, model.RegisterCustomerRequest{Email: "client@example.com", Password: "customer-pass", Name: "Duplicate"}); !errors.Is(err, repository.ErrEmailExists) {
		t.Fatalf("duplicate email: %v", err)
	}

	merchantReq := model.RegisterDemoMerchantRequest{Email: "owner@example.com", Password: "merchant-pass", OwnerName: "Owner", BrandName: "Brand", BranchName: "Main"}
	merchantKey := uuid.NewString()
	created, err := svc.RegisterDemoMerchant(ctx, merchantKey, "demo-access-code", uuid.NewString(), merchantReq)
	if err != nil {
		t.Fatal(err)
	}
	var createdEnvelope web.Envelope[model.DemoMerchantData]
	if err = json.Unmarshal(created.Body, &createdEnvelope); err != nil {
		t.Fatal(err)
	}
	merchant := createdEnvelope.Data
	replayed, err := svc.RegisterDemoMerchant(ctx, merchantKey, "demo-access-code", uuid.NewString(), merchantReq)
	if err != nil || !replayed.Replayed || !bytes.Equal(created.Body, replayed.Body) {
		t.Fatalf("merchant replay: replay=%v err=%v", replayed.Replayed, err)
	}
	changed := merchantReq
	changed.BrandName = "Other"
	if _, err = svc.RegisterDemoMerchant(ctx, merchantKey, "demo-access-code", uuid.NewString(), changed); !errors.Is(err, repository.ErrIdempotencyConflict) {
		t.Fatalf("merchant conflict: %v", err)
	}
	var brandsBefore int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM marcas`).Scan(&brandsBefore); err != nil {
		t.Fatal(err)
	}
	duplicate := merchantReq
	duplicate.BrandName = "Rollback"
	if _, err = svc.RegisterDemoMerchant(ctx, uuid.NewString(), "demo-access-code", uuid.NewString(), duplicate); !errors.Is(err, repository.ErrEmailExists) {
		t.Fatalf("atomic duplicate: %v", err)
	}
	var brandsAfter, pending int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM marcas`).Scan(&brandsAfter)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM solicitudes_idempotentes WHERE estado='PENDING'`).Scan(&pending)
	if brandsAfter != brandsBefore || pending != 0 {
		t.Fatalf("rollback leaked brand/idempotency: %d/%d pending=%d", brandsBefore, brandsAfter, pending)
	}

	memberCode := fmt.Sprintf("#USER-%04d", customer.ID)
	previewReq := model.MovementPreviewRequest{Operation: "ACUMULACION", CustomerCode: memberCode, BranchID: merchant.Merchant.Branch.ID}
	preview, err := svc.Preview(ctx, merchant.User.ID, previewReq)
	if err != nil {
		t.Fatal(err)
	}
	if preview.CardID != 0 || preview.BalanceBefore != 0 || preview.BalanceAfter != 1 {
		t.Fatalf("unexpected preview %+v", preview)
	}
	var cards, movements int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM tarjetas`).Scan(&cards)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM historial_movimientos`).Scan(&movements)
	if cards != 0 || movements != 0 {
		t.Fatalf("preview mutated cards=%d movements=%d", cards, movements)
	}
	confirmReq := model.ConfirmAccumulationRequest{PreviewID: preview.ID, CustomerCode: memberCode, BranchID: merchant.Merchant.Branch.ID}
	normalizedConfirmReq := confirmReq
	normalizedConfirmReq.QRToken = customer.QRToken
	normalizedConfirmReq.CustomerCode = ""
	pendingKey := uuid.NewString()
	pendingTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pendingTx.Exec(ctx, `INSERT INTO solicitudes_idempotentes(idempotency_key,actor_scope,operacion,fingerprint,estado) VALUES($1,$2,'CONFIRM_ACUMULACION',$3,'PENDING')`, pendingKey, fmt.Sprintf("user:%d", merchant.User.ID), service.Fingerprint(normalizedConfirmReq)); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.ConfirmAccumulation(ctx, merchant.User.ID, pendingKey, uuid.NewString(), confirmReq); !errors.Is(err, repository.ErrIdempotencyInProgress) {
		t.Fatalf("in progress: %v", err)
	}
	if err = pendingTx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	confirmKey := uuid.NewString()
	confirmed, err := svc.ConfirmAccumulation(ctx, merchant.User.ID, confirmKey, uuid.NewString(), confirmReq)
	if err != nil {
		t.Fatal(err)
	}
	again, err := svc.ConfirmAccumulation(ctx, merchant.User.ID, confirmKey, uuid.NewString(), confirmReq)
	if err != nil || !again.Replayed || !bytes.Equal(confirmed.Body, again.Body) {
		t.Fatalf("movement replay: %v", err)
	}
	if _, err = svc.ConfirmAccumulation(ctx, merchant.User.ID, uuid.NewString(), uuid.NewString(), confirmReq); !errors.Is(err, repository.ErrPreviewConsumed) {
		t.Fatalf("consumed preview: %v", err)
	}
	altered := confirmReq
	altered.BranchID++
	if _, err = svc.ConfirmAccumulation(ctx, merchant.User.ID, confirmKey, uuid.NewString(), altered); !errors.Is(err, repository.ErrIdempotencyConflict) {
		t.Fatalf("movement conflict: %v", err)
	}

	expired, err := svc.Preview(ctx, merchant.User.ID, previewReq)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE previews_movimiento SET expires_at=now()-interval '1 second' WHERE id=$1`, expired.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.ConfirmAccumulation(ctx, merchant.User.ID, uuid.NewString(), uuid.NewString(), model.ConfirmAccumulationRequest{PreviewID: expired.ID, QRToken: customer.QRToken, BranchID: merchant.Merchant.Branch.ID}); !errors.Is(err, repository.ErrPreviewExpired) {
		t.Fatalf("expired: %v", err)
	}

	for i := 0; i < 4; i++ {
		p, e := svc.Preview(ctx, merchant.User.ID, previewReq)
		if e != nil {
			t.Fatal(e)
		}
		_, e = svc.ConfirmAccumulation(ctx, merchant.User.ID, uuid.NewString(), uuid.NewString(), model.ConfirmAccumulationRequest{PreviewID: p.ID, QRToken: customer.QRToken, BranchID: merchant.Merchant.Branch.ID})
		if e != nil {
			t.Fatal(e)
		}
	}
	redemptionReq := model.MovementPreviewRequest{Operation: "CANJE", QRToken: customer.QRToken, BranchID: merchant.Merchant.Branch.ID, BenefitID: &merchant.Merchant.Benefit.ID}
	redeemPreview, err := svc.Preview(ctx, merchant.User.ID, redemptionReq)
	if err != nil {
		t.Fatal(err)
	}
	redeemed, err := svc.ConfirmRedemption(ctx, merchant.User.ID, uuid.NewString(), uuid.NewString(), model.ConfirmRedemptionRequest{PreviewID: redeemPreview.ID, QRToken: customer.QRToken, BranchID: merchant.Merchant.Branch.ID, BenefitID: merchant.Merchant.Benefit.ID})
	if err != nil {
		t.Fatal(err)
	}
	var redeemedEnvelope web.Envelope[model.Movement]
	if err = json.Unmarshal(redeemed.Body, &redeemedEnvelope); err != nil {
		t.Fatal(err)
	}
	if redeemedEnvelope.Data.BalanceAfter != 0 || redeemedEnvelope.Data.BenefitNameSnapshot == nil || *redeemedEnvelope.Data.BenefitNameSnapshot != "Beneficio de prueba" {
		t.Fatalf("bad redemption %+v", redeemedEnvelope.Data)
	}

	// Restore five stamps, create two previews at the same balance and confirm concurrently.
	for i := 0; i < 5; i++ {
		p, e := svc.Preview(ctx, merchant.User.ID, previewReq)
		if e != nil {
			t.Fatal(e)
		}
		_, e = svc.ConfirmAccumulation(ctx, merchant.User.ID, uuid.NewString(), uuid.NewString(), model.ConfirmAccumulationRequest{PreviewID: p.ID, QRToken: customer.QRToken, BranchID: merchant.Merchant.Branch.ID})
		if e != nil {
			t.Fatal(e)
		}
	}
	p1, _ := svc.Preview(ctx, merchant.User.ID, redemptionReq)
	p2, _ := svc.Preview(ctx, merchant.User.ID, redemptionReq)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, p := range []model.Preview{p1, p2} {
		wg.Add(1)
		go func(p model.Preview) {
			defer wg.Done()
			_, e := svc.ConfirmRedemption(ctx, merchant.User.ID, uuid.NewString(), uuid.NewString(), model.ConfirmRedemptionRequest{PreviewID: p.ID, QRToken: customer.QRToken, BranchID: merchant.Merchant.Branch.ID, BenefitID: merchant.Merchant.Benefit.ID})
			errs <- e
		}(p)
	}
	wg.Wait()
	close(errs)
	successes := 0
	for e := range errs {
		if e == nil {
			successes++
		} else if !errors.Is(e, repository.ErrPreviewChanged) && !errors.Is(e, repository.ErrInsufficientBalance) {
			t.Fatalf("unexpected concurrent error: %v", e)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent redemptions accepted=%d", successes)
	}
	var balance int64
	if err = pool.QueryRow(ctx, `SELECT saldo_sellos FROM tarjetas WHERE usuario_id=$1`, customer.ID).Scan(&balance); err != nil || balance != 0 {
		t.Fatalf("balance=%d err=%v", balance, err)
	}

	brandCustomers, customerPagination, err := svc.BrandCustomers(ctx, merchant.User.ID, merchant.Merchant.BrandID, 1, 20, " CLIENT@ ")
	if err != nil {
		t.Fatal(err)
	}
	if customerPagination.TotalItems != 1 || customerPagination.TotalPages != 1 || len(brandCustomers) != 1 {
		t.Fatalf("brand customer page=%+v items=%+v", customerPagination, brandCustomers)
	}
	brandCustomer := brandCustomers[0]
	if brandCustomer.CustomerID != customer.ID || brandCustomer.Email != "client@example.com" || brandCustomer.Name != "Client" ||
		brandCustomer.BalanceStamps != 0 || brandCustomer.MovementsCount != 12 || brandCustomer.LastMovementAt == nil || brandCustomer.JoinedAt.IsZero() {
		t.Fatalf("unexpected brand customer %+v", brandCustomer)
	}
	emptyCustomers, emptyPagination, err := svc.BrandCustomers(ctx, merchant.User.ID, merchant.Merchant.BrandID, 1, 20, "not-present")
	if err != nil || len(emptyCustomers) != 0 || emptyPagination.TotalItems != 0 || emptyPagination.TotalPages != 0 {
		t.Fatalf("filtered customers page=%+v items=%+v err=%v", emptyPagination, emptyCustomers, err)
	}
	metrics, err := svc.BrandMetrics(ctx, merchant.User.ID, merchant.Merchant.BrandID)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.ActiveCustomers != 1 || metrics.CurrentStampBalance != 0 || metrics.Accumulations != 10 || metrics.Redemptions != 2 ||
		metrics.StampsIssued != 10 || metrics.StampsRedeemed != 10 || metrics.LastMovementAt == nil {
		t.Fatalf("unexpected brand metrics %+v", metrics)
	}

	secondReq := model.RegisterDemoMerchantRequest{Email: "owner2@example.com", Password: "merchant-pass", OwnerName: "Owner2", BrandName: "Brand2", BranchName: "Other"}
	secondRaw, err := svc.RegisterDemoMerchant(ctx, uuid.NewString(), "demo-access-code", uuid.NewString(), secondReq)
	if err != nil {
		t.Fatal(err)
	}
	var second web.Envelope[model.DemoMerchantData]
	_ = json.Unmarshal(secondRaw.Body, &second)
	alien := previewReq
	alien.BranchID = second.Data.Merchant.Branch.ID
	if _, err = svc.Preview(ctx, merchant.User.ID, alien); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("ownership: %v", err)
	}
	if _, _, err = svc.BrandCustomers(ctx, second.Data.User.ID, merchant.Merchant.BrandID, 1, 20, ""); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("customer list ownership: %v", err)
	}
	if _, err = svc.BrandMetrics(ctx, second.Data.User.ID, merchant.Merchant.BrandID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("metrics ownership: %v", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE membresias_marca SET activo=false WHERE usuario_id=$1 AND marca_id=$2`, merchant.User.ID, merchant.Merchant.BrandID); err != nil {
		t.Fatal(err)
	}
	if _, _, err = svc.BrandCustomers(ctx, merchant.User.ID, merchant.Merchant.BrandID, 1, 20, ""); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("inactive membership customer list: %v", err)
	}
	if _, err = svc.BrandMetrics(ctx, merchant.User.ID, merchant.Merchant.BrandID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("inactive membership metrics: %v", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE membresias_marca SET activo=true WHERE usuario_id=$1 AND marca_id=$2`, merchant.User.ID, merchant.Merchant.BrandID); err != nil {
		t.Fatal(err)
	}
	if err = repo.CheckSchema(ctx, "0001"); err != nil {
		t.Fatal(err)
	}
	if err = repo.CheckSchema(ctx, "9999"); err == nil {
		t.Fatal("readiness accepted wrong schema")
	}
	t.Logf("verified brand=%d customer=%d movements persisted", merchant.Merchant.BrandID, customer.ID)
}
