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
	programMigration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0002_program_types.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(programMigration)); err != nil {
		t.Fatalf("migration 0002: %v", err)
	}
	tenantMigration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0003_tenant_roles.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(tenantMigration)); err != nil {
		t.Fatalf("migration 0003: %v", err)
	}
	sessionMigration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0004_auth_sessions.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(sessionMigration)); err != nil {
		t.Fatalf("migration 0004: %v", err)
	}
	benefitMigration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0005_benefit_versions.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(benefitMigration)); err != nil {
		t.Fatalf("migration 0005: %v", err)
	}
	familyMigration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0006_session_families.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(familyMigration)); err != nil {
		t.Fatalf("migration 0006: %v", err)
	}
	snapshotMigration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0007_movement_snapshots.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(snapshotMigration)); err != nil {
		t.Fatalf("migration 0007: %v", err)
	}
	identityMigration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0008_email_identity.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(identityMigration)); err != nil {
		t.Fatalf("migration 0008: %v", err)
	}
	if _, err = pool.Exec(ctx, `CREATE TABLE schema_migrations(version CHAR(4) PRIMARY KEY,applied_at TIMESTAMPTZ NOT NULL DEFAULT now()); INSERT INTO schema_migrations(version) VALUES('0001'),('0002'),('0003'),('0004'),('0005'),('0006'),('0007'),('0008')`); err != nil {
		t.Fatal(err)
	}
	demoHash, _ := bcrypt.GenerateFromPassword([]byte("demo-access-code"), bcrypt.MinCost)
	cfg := config.Config{JWTSecret: "jwt-secret-0123456789012345678901", JWTIssuer: "puntazo", QRPepper: "qr-pepper-01234567890123456789012", DemoAccessCodeHash: string(demoHash), DemoSignupEnabled: true, ExpectedSchemaVersion: "0008", PublicAppURL: "https://app.puntazo.test"}
	repo := repository.New(pool)
	tokens := auth.NewTokens(cfg.JWTSecret, cfg.JWTIssuer)
	svc := service.New(repo, tokens, cfg)
	identityCfg := cfg
	identityCfg.EmailVerificationRequired = true
	identitySvc := service.New(repo, tokens, identityCfg)
	pendingRegistration, err := identitySvc.RegisterCustomer(ctx, model.RegisterCustomerRequest{Email: "pending@example.com", Password: "pending-pass", Name: "Pending"})
	if err != nil || pendingRegistration.Session != nil || !pendingRegistration.VerificationRequired {
		t.Fatalf("pending registration=%+v err=%v", pendingRegistration, err)
	}
	if _, err = identitySvc.Login(ctx, model.LoginRequest{Email: "pending@example.com", Password: "pending-pass"}); !errors.Is(err, service.ErrEmailUnverified) {
		t.Fatalf("unverified login: %v", err)
	}
	var verificationBody string
	if err = pool.QueryRow(ctx, `SELECT cuerpo_texto FROM email_outbox WHERE destinatario='pending@example.com' AND tipo='VERIFY_EMAIL' ORDER BY created_at DESC LIMIT 1`).Scan(&verificationBody); err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(verificationBody, "?token=")
	if len(parts) != 2 {
		t.Fatal("verification token missing from outbox")
	}
	verificationToken := strings.Fields(parts[1])[0]
	if err = identitySvc.ConfirmEmailVerification(ctx, model.TokenRequest{Token: verificationToken}); err != nil {
		t.Fatal(err)
	}
	if err = identitySvc.ConfirmEmailVerification(ctx, model.TokenRequest{Token: verificationToken}); !errors.Is(err, service.ErrIdentityToken) {
		t.Fatalf("verification token reused: %v", err)
	}
	pendingLogin, err := identitySvc.Login(ctx, model.LoginRequest{Email: "pending@example.com", Password: "pending-pass"})
	if err != nil {
		t.Fatalf("verified login: %v", err)
	}
	if err = identitySvc.RequestPasswordReset(ctx, model.EmailRequest{Email: "pending@example.com"}); err != nil {
		t.Fatal(err)
	}
	var resetBody string
	if err = pool.QueryRow(ctx, `SELECT cuerpo_texto FROM email_outbox WHERE destinatario='pending@example.com' AND tipo='RESET_PASSWORD' ORDER BY created_at DESC LIMIT 1`).Scan(&resetBody); err != nil {
		t.Fatal(err)
	}
	parts = strings.Split(resetBody, "?token=")
	if len(parts) != 2 {
		t.Fatal("reset token missing")
	}
	resetToken := strings.Fields(parts[1])[0]
	if err = identitySvc.ConfirmPasswordReset(ctx, model.PasswordResetConfirmRequest{Token: resetToken, NewPassword: "new-pending-pass"}); err != nil {
		t.Fatal(err)
	}
	if w := authorizedRequestFor(tokens, repo, pendingLogin.Session.AccessToken); w.Code != http.StatusUnauthorized {
		t.Fatalf("reset did not revoke access: %d", w.Code)
	}
	if err = identitySvc.ConfirmPasswordReset(ctx, model.PasswordResetConfirmRequest{Token: resetToken, NewPassword: "another-password"}); !errors.Is(err, service.ErrIdentityToken) {
		t.Fatalf("reset reused: %v", err)
	}
	if _, err = identitySvc.Login(ctx, model.LoginRequest{Email: "pending@example.com", Password: "pending-pass"}); !errors.Is(err, service.ErrInvalidCredentials) {
		t.Fatalf("old password login: %v", err)
	}
	if _, err = identitySvc.Login(ctx, model.LoginRequest{Email: "pending@example.com", Password: "new-pending-pass"}); err != nil {
		t.Fatalf("new password login: %v", err)
	}

	customerAuth, err := svc.RegisterCustomer(ctx, model.RegisterCustomerRequest{Email: "client@example.com", Password: "customer-pass", Name: "Client"})
	if err != nil {
		t.Fatal(err)
	}
	if customerAuth.Session == nil {
		t.Fatal("registration omitted session while verification is disabled")
	}
	gin.SetMode(gin.TestMode)
	protected := gin.New()
	protected.GET("/protected", middleware.RequireAuth(tokens, repo), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	authorizedRequest := func(accessToken string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)
		w := httptest.NewRecorder()
		protected.ServeHTTP(w, req)
		return w
	}
	if w := authorizedRequest(customerAuth.Session.AccessToken); w.Code != http.StatusNoContent {
		t.Fatalf("active token status=%d body=%s", w.Code, w.Body.String())
	}
	originalAccess := customerAuth.Session.AccessToken
	originalRefresh := customerAuth.Session.RefreshToken
	refreshedAuth, err := svc.Refresh(ctx, customerAuth.Session.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	customerAuth.Session = &refreshedAuth.Session
	customerAuth.User = refreshedAuth.User
	if w := authorizedRequest(originalAccess); w.Code != http.StatusUnauthorized {
		t.Fatalf("rotated access token status=%d body=%s", w.Code, w.Body.String())
	}
	if _, err = svc.Refresh(ctx, originalRefresh); !errors.Is(err, service.ErrInvalidCredentials) {
		t.Fatalf("refresh token replay: %v", err)
	}
	if w := authorizedRequest(customerAuth.Session.AccessToken); w.Code != http.StatusUnauthorized {
		t.Fatalf("session family access after reuse status=%d body=%s", w.Code, w.Body.String())
	}
	if _, err = svc.Refresh(ctx, customerAuth.Session.RefreshToken); !errors.Is(err, service.ErrInvalidCredentials) {
		t.Fatalf("session family refresh after reuse: %v", err)
	}
	loggedInAuth, err := svc.Login(ctx, model.LoginRequest{Email: "client@example.com", Password: "customer-pass"})
	if err != nil {
		t.Fatal(err)
	}
	customerAuth.Session = &loggedInAuth.Session
	customerAuth.User = loggedInAuth.User
	if _, err = pool.Exec(ctx, `UPDATE usuarios SET activo=false WHERE id=$1`, customerAuth.User.ID); err != nil {
		t.Fatal(err)
	}
	w := authorizedRequest(customerAuth.Session.AccessToken)
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
	_, _, sessionID, err := tokens.ParseSession(customerAuth.Session.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Logout(ctx, customerAuth.User.ID, sessionID); err != nil {
		t.Fatal(err)
	}
	if w = authorizedRequest(customerAuth.Session.AccessToken); w.Code != http.StatusUnauthorized {
		t.Fatalf("logged out token status=%d body=%s", w.Code, w.Body.String())
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

	merchantReq := model.RegisterDemoMerchantRequest{Email: "owner@example.com", Password: "merchant-pass", OwnerName: "Owner", BrandName: "Brand", BranchName: "Main", ProgramType: "SELLOS", AccessCode: "demo-access-code"}
	merchantKey := uuid.NewString()
	created, err := svc.RegisterDemoMerchant(ctx, merchantKey, uuid.NewString(), merchantReq)
	if err != nil {
		t.Fatal(err)
	}
	var createdEnvelope web.Envelope[model.DemoMerchantData]
	if err = json.Unmarshal(created.Body, &createdEnvelope); err != nil {
		t.Fatal(err)
	}
	merchant := createdEnvelope.Data
	if merchant.OnboardingComplete || merchant.Merchant.Benefit != nil || len(merchant.Merchant.Benefits) != 0 {
		t.Fatalf("merchant signup invented onboarding data: %+v", merchant)
	}
	replayed, err := svc.RegisterDemoMerchant(ctx, merchantKey, uuid.NewString(), merchantReq)
	if err != nil || !replayed.Replayed || bytes.Equal(created.Body, replayed.Body) {
		t.Fatalf("merchant replay: replay=%v err=%v", replayed.Replayed, err)
	}
	var persistedMerchantResponse []byte
	if err = pool.QueryRow(ctx, `SELECT response_body FROM solicitudes_idempotentes WHERE idempotency_key=$1`, merchantKey).Scan(&persistedMerchantResponse); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(persistedMerchantResponse, []byte("refresh_token")) || bytes.Contains(persistedMerchantResponse, []byte(merchant.Session.RefreshToken)) {
		t.Fatal("merchant refresh secret persisted in idempotency response")
	}
	changed := merchantReq
	changed.BrandName = "Other"
	if _, err = svc.RegisterDemoMerchant(ctx, merchantKey, uuid.NewString(), changed); !errors.Is(err, repository.ErrIdempotencyConflict) {
		t.Fatalf("merchant conflict: %v", err)
	}
	var brandsBefore int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM marcas`).Scan(&brandsBefore); err != nil {
		t.Fatal(err)
	}
	duplicate := merchantReq
	duplicate.BrandName = "Rollback"
	if _, err = svc.RegisterDemoMerchant(ctx, uuid.NewString(), uuid.NewString(), duplicate); !errors.Is(err, repository.ErrEmailExists) {
		t.Fatalf("atomic duplicate: %v", err)
	}
	var brandsAfter, pending int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM marcas`).Scan(&brandsAfter)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM solicitudes_idempotentes WHERE estado='PENDING'`).Scan(&pending)
	if brandsAfter != brandsBefore || pending != 0 {
		t.Fatalf("rollback leaked brand/idempotency: %d/%d pending=%d", brandsBefore, brandsAfter, pending)
	}
	benefit, err := svc.CreateBenefit(ctx, merchant.User.ID, merchant.Merchant.BrandID, model.CreateBenefitRequest{Name: "Beneficio de prueba", Requirement: 5})
	if err != nil || benefit.RequiredStamps == nil || *benefit.RequiredStamps != 5 || benefit.RequiredPoints != nil || benefit.Version != 1 {
		t.Fatalf("create Sellos benefit=%+v err=%v", benefit, err)
	}
	benefits, err := svc.Benefits(ctx, merchant.User.ID, merchant.Merchant.BrandID)
	if err != nil || len(benefits) != 1 || benefits[0].ID != benefit.ID {
		t.Fatalf("list Sellos benefits=%+v err=%v", benefits, err)
	}
	if _, err = svc.CreateBenefit(ctx, customer.ID, merchant.Merchant.BrandID, model.CreateBenefitRequest{Name: "Ajeno", Requirement: 1}); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("customer created brand benefit: %v", err)
	}
	if _, err = svc.CreateBenefit(ctx, merchant.User.ID, merchant.Merchant.BrandID, model.CreateBenefitRequest{Name: "Excesivo", Requirement: 10000001}); !errors.Is(err, service.ErrInvalidRequest) {
		t.Fatalf("oversized benefit: %v", err)
	}
	benefitID := benefit.ID
	if benefit.RequiredStamps == nil {
		t.Fatal(err)
	}
	secondBenefit, err := svc.CreateBenefit(ctx, merchant.User.ID, merchant.Merchant.BrandID, model.CreateBenefitRequest{Name: "Segundo beneficio", Requirement: 10})
	if err != nil {
		t.Fatal(err)
	}
	contexts, err := svc.ListBrands(ctx, merchant.User.ID)
	if err != nil || len(contexts) != 1 || len(contexts[0].Benefits) != 2 || contexts[0].Benefits[1].ID != secondBenefit.ID {
		t.Fatalf("multi-benefit contexts=%+v err=%v", contexts, err)
	}
	requiredStamps := int64(5)
	merchant.Merchant.Benefit = &model.Benefit{ID: benefitID, ProgramID: merchant.Merchant.Program.ID, Name: "Beneficio de prueba", RequiredStamps: &requiredStamps, Active: true, Version: 1}
	currentMerchant, err := svc.CurrentUser(ctx, merchant.User.ID)
	if err != nil || !currentMerchant.OnboardingComplete {
		t.Fatalf("configured merchant onboarding=%v err=%v", currentMerchant.OnboardingComplete, err)
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
	if _, err = pool.Exec(ctx, `UPDATE marcas SET nombre='Brand renombrada' WHERE id=$1`, merchant.Merchant.BrandID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE sucursales SET nombre='Sucursal renombrada' WHERE id=$1`, merchant.Merchant.Branch.ID); err != nil {
		t.Fatal(err)
	}
	historical, _, err := svc.BrandMovements(ctx, merchant.User.ID, merchant.Merchant.BrandID, 1, 20)
	if err != nil || len(historical) != 1 || historical[0].BrandName != "Brand" || historical[0].BranchName != "Main" || historical[0].ProgramIDSnapshot != merchant.Merchant.Program.ID || historical[0].ProgramType != "SELLOS" {
		t.Fatalf("movement snapshots=%+v err=%v", historical, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE marcas SET nombre='Brand' WHERE id=$1`, merchant.Merchant.BrandID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE sucursales SET nombre='Main' WHERE id=$1`, merchant.Merchant.Branch.ID); err != nil {
		t.Fatal(err)
	}
	customerCards, cardPage, err := svc.Cards(ctx, customer.ID, 1, 20)
	if err != nil || cardPage.TotalItems != 1 || len(customerCards) != 1 || customerCards[0].Benefit.ID != benefit.ID || customerCards[0].Benefit.Version != 1 {
		t.Fatalf("multi-benefit cards=%+v page=%+v err=%v", customerCards, cardPage, err)
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

	secondReq := model.RegisterDemoMerchantRequest{Email: "owner2@example.com", Password: "merchant-pass", OwnerName: "Owner2", BrandName: "Brand2", BranchName: "Other", ProgramType: "PUNTOS", AccessCode: "demo-access-code"}
	secondRaw, err := svc.RegisterDemoMerchant(ctx, uuid.NewString(), uuid.NewString(), secondReq)
	if err != nil {
		t.Fatal(err)
	}
	var second web.Envelope[model.DemoMerchantData]
	_ = json.Unmarshal(secondRaw.Body, &second)
	if second.Data.OnboardingComplete || second.Data.Merchant.Program.Type != "PUNTOS" {
		t.Fatalf("unexpected PUNTOS registration %+v", second.Data)
	}
	var firstMembershipID int64
	if err = pool.QueryRow(ctx, `SELECT id FROM membresias_marca WHERE usuario_id=$1 AND marca_id=$2`, merchant.User.ID, merchant.Merchant.BrandID).Scan(&firstMembershipID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO membresias_sucursales(membresia_id,sucursal_id,marca_id) VALUES($1,$2,$3)`, firstMembershipID, second.Data.Merchant.Branch.ID, merchant.Merchant.BrandID); err == nil {
		t.Fatal("cross-brand branch membership was accepted")
	}
	pointsBenefit, err := svc.CreateBenefit(ctx, second.Data.User.ID, second.Data.Merchant.BrandID, model.CreateBenefitRequest{Name: "Beneficio Puntos", Requirement: 10000000})
	if err != nil || pointsBenefit.RequiredPoints == nil || *pointsBenefit.RequiredPoints != 10000000 || pointsBenefit.RequiredStamps != nil {
		t.Fatalf("create Puntos benefit=%+v err=%v", pointsBenefit, err)
	}
	pointsPreview := model.MovementPreviewRequest{Operation: "ACUMULACION", QRToken: customer.QRToken, BranchID: second.Data.Merchant.Branch.ID}
	if _, err = svc.Preview(ctx, second.Data.User.ID, pointsPreview); !errors.Is(err, repository.ErrInvalidRequest) {
		t.Fatalf("PUNTOS preview without amount: %v", err)
	}
	tooManyPoints := int64(100001)
	pointsPreview.PointsAmount = &tooManyPoints
	if _, err = svc.Preview(ctx, second.Data.User.ID, pointsPreview); !errors.Is(err, repository.ErrInvalidRequest) {
		t.Fatalf("PUNTOS preview over limit: %v", err)
	}
	manualPoints := int64(100000)
	pointsPreview.PointsAmount = &manualPoints
	pointSnapshot, err := svc.Preview(ctx, second.Data.User.ID, pointsPreview)
	if err != nil || pointSnapshot.Amount != manualPoints || pointSnapshot.ProgramType != "PUNTOS" {
		t.Fatalf("PUNTOS preview=%+v err=%v", pointSnapshot, err)
	}
	pointMovement, err := svc.ConfirmAccumulation(ctx, second.Data.User.ID, uuid.NewString(), uuid.NewString(), model.ConfirmAccumulationRequest{PreviewID: pointSnapshot.ID, QRToken: customer.QRToken, BranchID: second.Data.Merchant.Branch.ID})
	if err != nil {
		t.Fatal(err)
	}
	var pointEnvelope web.Envelope[model.Movement]
	if err = json.Unmarshal(pointMovement.Body, &pointEnvelope); err != nil || pointEnvelope.Data.Amount != manualPoints || pointEnvelope.Data.BalanceAfter != manualPoints || pointEnvelope.Data.ProgramType != "PUNTOS" {
		t.Fatalf("PUNTOS confirmation=%+v err=%v", pointEnvelope, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE programas_fidelidad SET tipo='SELLOS',sellos_por_acumulacion=1 WHERE id=$1`, second.Data.Merchant.Program.ID); err == nil {
		t.Fatal("program type changed after first movement")
	}
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
	if err = repo.CheckSchema(ctx, "0008"); err != nil {
		t.Fatal(err)
	}
	if err = repo.CheckSchema(ctx, "9999"); err == nil {
		t.Fatal("readiness accepted wrong schema")
	}
	t.Logf("verified brand=%d customer=%d movements persisted", merchant.Merchant.BrandID, customer.ID)
}

func authorizedRequestFor(tokens *auth.Tokens, repo *repository.Repository, accessToken string) *httptest.ResponseRecorder {
	router := gin.New()
	router.GET("/protected", middleware.RequireAuth(tokens, repo), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}
