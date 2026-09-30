package repository_test

import (
	"clientesFrecuentes/internal/auth"
	"clientesFrecuentes/internal/config"
	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/repository"
	"clientesFrecuentes/internal/service"
	"clientesFrecuentes/internal/web"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"sync"
	"testing"
	"time"
)

func TestPostgresUserCodes(t *testing.T) {
	pool := referralBillingPool(t)
	ctx := t.Context()
	repo := repository.New(pool)
	repo.OutboxCipherKey = []byte("01234567890123456789012345678901")
	cfg := config.Config{JWTSecret: "jwt-secret-0123456789012345678901", JWTIssuer: "puntazo", QRPepper: "qr-pepper-01234567890123456789012", DemoSignupEnabled: true, OutboxEncryptionKey: repo.OutboxCipherKey, PublicAppURL: "https://app.example.test"}
	svc := service.New(repo, auth.NewTokens(cfg.JWTSecret, cfg.JWTIssuer), cfg)
	create := func(name, last, email string) model.User {
		t.Helper()
		r, err := svc.RegisterCustomer(ctx, model.RegisterCustomerRequest{Name: name, LastName: last, Email: email, Password: "customer-password"})
		if err != nil {
			t.Fatal(err)
		}
		return r.User
	}
	gg := create("Gabriel", "Gonzalez", "gg@example.test")
	gg2 := create("Gabriel", "Gutierrez", "gg2@example.test")
	gt := create("Gonzalo", "Tevez", "gt@example.test")
	mp := create("  María José ", " Pérez López ", "mp@example.test")
	if gg.UserCode != "GG-1" || gg2.UserCode != "GG-2" || gt.UserCode != "GT-1" || mp.UserCode != "MP-1" {
		t.Fatalf("codes: %s %s %s %s", gg.UserCode, gg2.UserCode, gt.UserCode, mp.UserCode)
	}
	var next int64
	counter := func(prefix string) int64 {
		t.Helper()
		if err := pool.QueryRow(ctx, `SELECT ultimo_numero FROM contadores_codigo_usuario WHERE prefijo=$1`, prefix).Scan(&next); err != nil {
			t.Fatal(err)
		}
		return next
	}
	if _, err := svc.RegisterCustomer(ctx, model.RegisterCustomerRequest{Name: "Gabriel", LastName: "Gutierrez", Email: gg.Email, Password: "customer-password"}); !errors.Is(err, repository.ErrEmailExists) {
		t.Fatalf("duplicate email: %v", err)
	}
	if counter("GG") != 2 {
		t.Fatal("failed registration consumed counter")
	}
	// A late failure (after the INSERT) rolls back both user and code.
	_, err := repo.CreateCustomer(ctx, "late-failure@example.test", "hash", "Gabriel", "Gonzalez", make([]byte, 32), func(int64) []byte { return nil }, nil, nil, time.Time{}, nil)
	if err == nil || counter("GG") != 2 {
		t.Fatal("late failure did not roll back")
	}
	const count = 16
	var wg sync.WaitGroup
	results := make(chan struct {
		user model.User
		err  error
	}, count)
	for i := range count {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			u, e := repo.CreateCustomer(ctx, fmt.Sprintf("concurrent-%d@example.test", i), "hash", "Gabriel", "Gutierrez", make([]byte, 32), func(id int64) []byte { _, hash := svc.QRForUser(id); return hash }, nil, nil, time.Time{}, nil)
			results <- struct {
				user model.User
				err  error
			}{u, e}
		}(i)
	}
	wg.Wait()
	close(results)
	seen := map[string]bool{}
	for r := range results {
		if r.err != nil {
			t.Fatal(r.err)
		}
		if seen[r.user.UserCode] {
			t.Fatal("duplicate code")
		}
		seen[r.user.UserCode] = true
	}
	for i := 3; i <= count+2; i++ {
		if !seen[fmt.Sprintf("GG-%d", i)] {
			t.Fatalf("missing GG-%d", i)
		}
	}
	if counter("GG") != count+2 || counter("GT") != 1 {
		t.Fatal("counter independence")
	}
	// Email proprietor shares the global counter; idempotent replay does not allocate.
	merchantReq := model.RegisterDemoMerchantRequest{OwnerName: "Gabriel", OwnerLastName: "Gonzalez", Email: "owner@example.test", Password: "owner-password", BrandName: "Brand", BranchName: "Main", ProgramType: "SELLOS"}
	key := uuid.NewString()
	raw, err := svc.RegisterDemoMerchant(ctx, key, uuid.NewString(), merchantReq)
	if err != nil {
		t.Fatal(err)
	}
	var env web.Envelope[model.DemoMerchantData]
	if err = json.Unmarshal(raw.Body, &env); err != nil {
		t.Fatal(err)
	}
	owner := env.Data
	if owner.User.UserCode != fmt.Sprintf("GG-%d", count+3) {
		t.Fatal(owner.User.UserCode)
	}
	if replay, e := svc.RegisterDemoMerchant(ctx, key, uuid.NewString(), merchantReq); e != nil || !replay.Replayed || counter("GG") != count+3 {
		t.Fatalf("replay %v", e)
	}
	// Employee allocation happens only once even when an invitation is submitted twice.
	invite, err := svc.CreateInvitation(ctx, owner.User.ID, owner.Merchant.BrandID, uuid.NewString(), uuid.NewString(), model.CreateInvitationRequest{Email: "employee@example.test", Role: "OPERADOR", BranchIDs: []int64{owner.Merchant.Branch.ID}})
	if err != nil {
		t.Fatal(err)
	}
	var invitation web.Envelope[model.BrandInvitation]
	if err = json.Unmarshal(invite.Body, &invitation); err != nil {
		t.Fatal(err)
	}
	var mail model.OutboxEmail
	if err = pool.QueryRow(ctx, `SELECT id::text,token_ciphertext,token_nonce,token_expires_at FROM email_outbox WHERE invitation_id=$1`, invitation.Data.ID).Scan(&mail.ID, &mail.Ciphertext, &mail.Nonce, &mail.ExpiresAt); err != nil {
		t.Fatal(err)
	}
	token, err := repository.DecryptOutboxToken(mail, cfg.OutboxEncryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	employee, err := svc.RegisterInvitation(ctx, token, model.RegisterInvitationRequest{Name: "Gabriel", LastName: "Gutierrez", Password: "employee-password"})
	if err != nil {
		t.Fatal(err)
	}
	if employee.User.UserCode != fmt.Sprintf("GG-%d", count+4) {
		t.Fatal(employee.User.UserCode)
	}
	if _, err = svc.RegisterInvitation(ctx, token, model.RegisterInvitationRequest{Name: "Gabriel", LastName: "Gutierrez", Password: "employee-password"}); !errors.Is(err, service.ErrIdentityToken) || counter("GG") != count+4 {
		t.Fatalf("invite replay: %v", err)
	}
	staff, err := repo.ListStaff(ctx, owner.User.ID, owner.Merchant.BrandID)
	if err != nil || len(staff) != 2 {
		t.Fatalf("staff %v", err)
	}
	for _, person := range staff {
		if person.UserCode == "" {
			t.Fatal("staff code absent")
		}
	}
	// Profile changes cannot rename a public identifier, including direct SQL changes.
	edited, err := repo.UpdateAccount(ctx, gg.ID, gg.Version, model.UpdateAccountRequest{Name: model.StringPatch("Otro"), LastName: model.StringPatch("Nombre")})
	if err != nil || edited.User.UserCode != "GG-1" {
		t.Fatalf("edit code: %v", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE usuarios SET codigo_usuario='ON-1' WHERE id=$1`, gg.ID); err == nil {
		t.Fatal("code mutation allowed")
	}
	// Legacy logins remain possible without names; no allocation/backfill.
	var legacyID int64
	if err = pool.QueryRow(ctx, `INSERT INTO usuarios(email,google_id,nombre,tipo_cuenta,qr_hash,email_verified_at) VALUES('legacy@example.test','legacy','Legacy','CLIENTE_FINAL',$1,now()) RETURNING id`, make([]byte, 32)).Scan(&legacyID); err != nil {
		t.Fatal(err)
	}
	_, legacyHash := svc.QRForUser(legacyID)
	if _, err = pool.Exec(ctx, `UPDATE usuarios SET qr_hash=$1 WHERE id=$2`, legacyHash, legacyID); err != nil {
		t.Fatal(err)
	}
	svc.VerifyGoogleToken = func(context.Context, string) (auth.GoogleIdentity, error) {
		return auth.GoogleIdentity{GoogleID: "legacy", Email: "legacy@example.test"}, nil
	}
	login, err := svc.LoginGoogle(ctx, model.GoogleAuthRequest{IDToken: "verified"})
	if err != nil || login.User.UserCode != fmt.Sprintf("#USER-%04d", legacyID) {
		t.Fatalf("legacy login: %v", err)
	}
	// New and old codes support accumulation, exact/numeric search and redemption.
	benefit, err := svc.CreateBenefit(ctx, owner.User.ID, owner.Merchant.BrandID, model.CreateBenefitRequest{Name: "Reward", Requirement: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, person := range []model.User{gg, login.User} {
		code := person.UserCode
		if person.ID == gg.ID {
			code = " #gg-01 "
		}
		preview, err := svc.Preview(ctx, owner.User.ID, model.MovementPreviewRequest{Operation: "ACUMULACION", CustomerCode: code, BranchID: owner.Merchant.Branch.ID})
		if err != nil || preview.Customer.UserCode != person.UserCode {
			t.Fatalf("preview: %+v %v", preview, err)
		}
		if _, err = svc.ConfirmAccumulation(ctx, owner.User.ID, uuid.NewString(), uuid.NewString(), model.ConfirmAccumulationRequest{PreviewID: preview.ID, CustomerCode: code, BranchID: owner.Merchant.Branch.ID}); err != nil {
			t.Fatal(err)
		}
		for _, search := range []string{code, fmt.Sprint(person.ID)} {
			people, total, err := repo.ListBrandCustomers(ctx, owner.User.ID, owner.Merchant.BrandID, 1, 20, search)
			if err != nil || total != 1 || people[0].UserCode != person.UserCode {
				t.Fatalf("search %s: %v %d %v", search, people, total, err)
			}
		}
		redemption, err := svc.Preview(ctx, owner.User.ID, model.MovementPreviewRequest{Operation: "CANJE", CustomerCode: code, BranchID: owner.Merchant.Branch.ID, BenefitID: &benefit.ID})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = svc.ConfirmRedemption(ctx, owner.User.ID, uuid.NewString(), uuid.NewString(), model.ConfirmRedemptionRequest{PreviewID: redemption.ID, CustomerCode: code, BranchID: owner.Merchant.Branch.ID, BenefitID: benefit.ID}); err != nil {
			t.Fatal(err)
		}
	}
	for _, code := range []string{owner.User.UserCode, employee.User.UserCode, "GG-999999"} {
		if _, err = svc.Preview(ctx, owner.User.ID, model.MovementPreviewRequest{Operation: "ACUMULACION", CustomerCode: code, BranchID: owner.Merchant.Branch.ID}); err == nil {
			t.Fatal("accepted non-customer/unknown code")
		}
	}
	if _, err = svc.Preview(ctx, gg.ID, model.MovementPreviewRequest{Operation: "ACUMULACION", CustomerCode: "GG-1", BranchID: owner.Merchant.Branch.ID}); err == nil {
		t.Fatal("customer authorized merchant action")
	}
	// Deleted codes stay reserved and unusable.
	if _, err = repo.AnonymizeAccount(ctx, gg.ID, edited.User.Version); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err = pool.QueryRow(ctx, `SELECT codigo_usuario FROM usuarios WHERE id=$1`, gg.ID).Scan(&stored); err != nil || stored != "GG-1" {
		t.Fatal("anonymization lost code", err)
	}
	if _, err = repo.CustomerIDByCode(ctx, "GG-1"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatal("deleted code resolved", err)
	}
	after := create("Gabriel", "Gonzalez", "after-delete@example.test")
	if after.UserCode != fmt.Sprintf("GG-%d", count+5) {
		t.Fatal("deleted code reused", after.UserCode)
	}
	// Google identity profiles: verified claims prevail; missing fields require completion.
	clientType, merchantType := "CLIENTE_FINAL", "PERSONAL_MARCA"
	svc.VerifyGoogleToken = func(_ context.Context, token string) (auth.GoogleIdentity, error) {
		return auth.GoogleIdentity{GoogleID: token, Email: token + "@example.test", Name: "Gonzalo", LastName: "Tevez"}, nil
	}
	_, err = svc.LoginGoogle(ctx, model.GoogleAuthRequest{IDToken: "google-new"})
	var profile *service.SignupProfileError
	if !errors.As(err, &profile) || !profile.AccountTypeRequired || profile.Name != "Gonzalo" || profile.LastName != "Tevez" {
		t.Fatalf("Google selector profile: %v", err)
	}
	google, err := svc.LoginGoogle(ctx, model.GoogleAuthRequest{IDToken: "google-new", AccountType: &clientType, Name: "Override", LastName: "Ignored"})
	if err != nil || google.User.UserCode != "GT-2" {
		t.Fatal("verified Google", err, google.User.UserCode)
	}
	googleOwner, err := svc.LoginGoogle(ctx, model.GoogleAuthRequest{IDToken: "google-owner", AccountType: &merchantType, MerchantRegistration: &model.GoogleMerchantRegistration{BrandName: "Google Brand", BranchName: "Main", ProgramType: "SELLOS"}})
	if err != nil || googleOwner.User.UserCode != "GT-3" {
		t.Fatal("Google owner", err)
	}
	svc.VerifyGoogleToken = func(context.Context, string) (auth.GoogleIdentity, error) {
		return auth.GoogleIdentity{GoogleID: "missing", Email: "missing@example.test", Name: "María José"}, nil
	}
	if _, err = svc.LoginGoogle(ctx, model.GoogleAuthRequest{IDToken: "missing", AccountType: &clientType}); !errors.Is(err, service.ErrRegistrationProfileRequired) {
		t.Fatal("missing Google surname", err)
	}
	completed, err := svc.LoginGoogle(ctx, model.GoogleAuthRequest{IDToken: "missing", AccountType: &clientType, LastName: "Pérez López"})
	if err != nil || completed.User.UserCode != "MP-2" {
		t.Fatal("Google completion", err)
	}
	if again, err := svc.LoginGoogle(ctx, model.GoogleAuthRequest{IDToken: "missing"}); err != nil || again.User.UserCode != "MP-2" || counter("MP") != 2 {
		t.Fatal("Google retry reallocated", err)
	}
}

func TestPostgresConcurrentGoogleCodes(t *testing.T) {
	pool := referralBillingPool(t)
	repo := repository.New(pool)
	cfg := config.Config{DemoSignupEnabled: true, JWTSecret: "jwt-secret-0123456789012345678901", JWTIssuer: "puntazo", QRPepper: "qr-pepper-01234567890123456789012"}
	svc := service.New(repo, auth.NewTokens(cfg.JWTSecret, cfg.JWTIssuer), cfg)
	svc.VerifyGoogleToken = func(_ context.Context, token string) (auth.GoogleIdentity, error) {
		return auth.GoogleIdentity{GoogleID: token, Email: token + "@example.test", Name: "Concurrent", LastName: "Google"}, nil
	}
	const total = 16
	clientType := "CLIENTE_FINAL"
	results := make(chan struct {
		code string
		err  error
	}, total)
	var wg sync.WaitGroup
	for i := range total {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r, e := svc.LoginGoogle(t.Context(), model.GoogleAuthRequest{IDToken: fmt.Sprintf("concurrent-%d", i), AccountType: &clientType})
			results <- struct {
				code string
				err  error
			}{r.User.UserCode, e}
		}(i)
	}
	wg.Wait()
	close(results)
	seen := map[string]bool{}
	for r := range results {
		if r.err != nil {
			t.Fatal(r.err)
		}
		if seen[r.code] {
			t.Fatal("duplicate code")
		}
		seen[r.code] = true
	}
	for i := 1; i <= total; i++ {
		if !seen[fmt.Sprintf("CG-%d", i)] {
			t.Fatal("missing code", i)
		}
	}
}
