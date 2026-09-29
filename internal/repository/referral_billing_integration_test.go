package repository_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Uses an isolated schema and the same ordered migration files as a new install.
func referralBillingPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := t.Context()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	if _, err = admin.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS citext`); err != nil {
		t.Fatal(err)
	}
	schema := "test_referral_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`) })
	pc, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	pc.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	paths, err := filepath.Glob(filepath.Join("..", "..", "migrations", "*.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `CREATE TABLE schema_migrations(version CHAR(4) PRIMARY KEY,applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		sql, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		body := strings.Replace(string(sql), "CREATE EXTENSION IF NOT EXISTS citext;", "", 1)
		if _, err = pool.Exec(ctx, body); err != nil {
			t.Fatalf("migration %s: %v", filepath.Base(path), err)
		}
		if _, err = pool.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES($1)`, filepath.Base(path)[:4]); err != nil {
			t.Fatal(err)
		}
	}
	return pool
}

func TestPostgresReferralBillingSequence(t *testing.T) {
	pool := referralBillingPool(t)
	ctx := t.Context()
	repo := repository.New(pool)
	var sourceBrand, targetBrand, ownerID int64
	for _, b := range []struct {
		name string
		id   *int64
	}{{"Referral source", &sourceBrand}, {"Referred target", &targetBrand}} {
		if err := pool.QueryRow(ctx, `INSERT INTO marcas(nombre) VALUES($1) RETURNING id`, b.name).Scan(b.id); err != nil {
			t.Fatal(err)
		}
	}
	if err := pool.QueryRow(ctx, `INSERT INTO usuarios(email,password_hash,nombre,tipo_cuenta) VALUES('referral-owner@example.test','hash','Owner','PERSONAL_MARCA') RETURNING id`).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO membresias_marca(usuario_id,marca_id,rol) VALUES($1,$2,'PROPIETARIO')`, []any{ownerID, targetBrand}},
		{`INSERT INTO sucursales(marca_id,nombre) VALUES($1,'Principal')`, []any{targetBrand}},
		{`INSERT INTO programas_fidelidad(marca_id,tipo,sellos_por_acumulacion,nombre_unidad) VALUES($1,'SELLOS',1,'Sello')`, []any{targetBrand}},
	} {
		if _, err := pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	var campaignID, codeID int64
	if err := pool.QueryRow(ctx, `INSERT INTO referral_campaigns(name,program_type,discount_bps,discount_charges,reward_bps,reward_charges,starts_at,ends_at) VALUES('Pilot','SELLOS',5000,3,2000,12,now()-interval '1 day',now()+interval '1 month') RETURNING id`).Scan(&campaignID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO referral_codes(code,campaign_id,source_kind,source_brand_id) VALUES('SOURCE-123',$1,'MERCHANT',$2) RETURNING id`, campaignID, sourceBrand).Scan(&codeID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO referral_attributions(brand_id,code_id,campaign_id,source_kind,source_brand_id,discount_bps,discount_charges,reward_bps,reward_charges) VALUES($1,$2,$3,'MERCHANT',$4,5000,3,2000,12)`, targetBrand, codeID, campaignID, sourceBrand); err != nil {
		t.Fatal(err)
	}
	_, reserved, err := repo.ReserveSubscriptionCheckout(ctx, ownerID, targetBrand, fmt.Sprintf("puntazo:brand:%d:%s", targetBrand, uuid.NewString()), 1500000, 2000000)
	if err != nil || reserved.Subscription.MonthlyAmountCents != 750000 || reserved.Subscription.FullMonthlyAmountCents != 1500000 || reserved.Subscription.DiscountRemainingCharges != 3 {
		t.Fatalf("discounted checkout=%+v err=%v", reserved, err)
	}
	claimed, err := repo.ClaimSubscriptionProviderCall(ctx, targetBrand, reserved.ExternalReference)
	if err != nil || !claimed {
		t.Fatalf("provider claim=%t err=%v", claimed, err)
	}
	if _, err = repo.SaveSubscriptionCheckout(ctx, targetBrand, model.BillingSubscriptionResult{ID: "target-sub", Status: "authorized", ExternalReference: reserved.ExternalReference}); err != nil {
		t.Fatal(err)
	}
	priceUpdates := 0
	advance := func(_ context.Context, id string, amount int64, _ string) error {
		priceUpdates++
		if id != "target-sub" || amount != 1500000 {
			t.Fatalf("price transition %s %d", id, amount)
		}
		return nil
	}
	for i := 1; i <= 13; i++ {
		amount := int64(1500000)
		if i <= 3 {
			amount = 750000
		}
		invoice := model.BillingInvoice{ID: fmt.Sprint(1000 + i), SubscriptionID: "target-sub", PaymentID: fmt.Sprint(2000 + i), Currency: "ARS", AmountMinor: amount}
		payment := model.BillingPayment{ID: invoice.PaymentID, Status: "approved", Currency: "ARS", AmountMinor: amount}
		if i == 1 {
			failed := payment
			failed.Status = "rejected"
			if err = repo.RecordReferralInvoice(ctx, "failed", invoice, failed, advance); err != nil {
				t.Fatal(err)
			}
			free := payment
			free.AmountMinor = 0
			if err = repo.RecordReferralInvoice(ctx, "free", invoice, free, advance); err != nil {
				t.Fatal(err)
			}
		}
		if err = repo.RecordReferralInvoice(ctx, fmt.Sprintf("approved-%d", i), invoice, payment, advance); err != nil {
			t.Fatalf("invoice %d: %v", i, err)
		}
		if i == 1 {
			if err = repo.RecordReferralInvoice(ctx, "duplicate", invoice, payment, advance); err != nil {
				t.Fatal(err)
			}
		}
	}
	if priceUpdates != 1 {
		t.Fatalf("full-price transition count=%d", priceUpdates)
	}
	var charges, rewards, thirdPrice, fourthPrice int
	if err = pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE paid_index<=3 AND amount_minor=750000),count(*) FILTER (WHERE paid_index>=4 AND amount_minor=1500000) FROM referral_charges WHERE brand_id=$1`, targetBrand).Scan(&charges, &thirdPrice, &fourthPrice); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM referral_rewards WHERE brand_id=$1`, targetBrand).Scan(&rewards); err != nil {
		t.Fatal(err)
	}
	if charges != 13 || thirdPrice != 3 || fourthPrice != 10 || rewards != 12 {
		t.Fatalf("charges=%d discount=%d full=%d rewards=%d", charges, thirdPrice, fourthPrice, rewards)
	}
	var financeUserID, influencerID int64
	if err = pool.QueryRow(ctx, `INSERT INTO backoffice_users(email,password_hash,totp_secret,role) VALUES('finance-referral@example.test','hash','secret','FINANZAS') RETURNING id`).Scan(&financeUserID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO referral_influencers(name,contact) VALUES('Pilot influencer','test@example.test') RETURNING id`).Scan(&influencerID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE referral_rewards SET source_kind='INFLUENCER',influencer_id=$1,source_brand_id=NULL WHERE provider_invoice_id='1001'`, influencerID); err != nil {
		t.Fatal(err)
	}
	if err = repo.SettleReferralReward(ctx, "1001", financeUserID); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		payment int
		status  string
	}{{2001, "refunded"}, {2002, "charged_back"}} {
		payment := model.BillingPayment{ID: fmt.Sprint(tc.payment), Status: tc.status, Currency: "ARS", AmountMinor: 750000, RefundedMinor: 750000}
		if err = repo.ReverseReferralPayment(ctx, fmt.Sprintf("reversal-%d", tc.payment), payment); err != nil {
			t.Fatal(err)
		}
	}
	var reversed, recoveryDue int
	if err = pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE status='VOID'),count(*) FILTER (WHERE status='RECOVERY_DUE' AND recovery_due_at IS NOT NULL) FROM referral_rewards WHERE brand_id=$1`, targetBrand).Scan(&reversed, &recoveryDue); err != nil || reversed != 1 || recoveryDue != 1 {
		t.Fatalf("void=%d recovery_due=%d err=%v", reversed, recoveryDue, err)
	}
	if _, err = repo.ReferralInvoiceForPayment(ctx, "not-ours"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("unknown payment=%v", err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO suscripciones_marca(marca_id,proveedor,proveedor_suscripcion_id,referencia_externa,estado,moneda,precio_sucursal_minor,cantidad_sucursales,importe_mensual_minor,full_unit_price_minor) VALUES($1,'MERCADO_PAGO','source-sub','source-ref','AUTHORIZED','ARS',1500000,1,1500000,1500000)`, sourceBrand); err != nil {
		t.Fatal(err)
	}
	creditInvoice := model.BillingInvoice{ID: "9999", SubscriptionID: "source-sub", PaymentID: "8888", Currency: "ARS", AmountMinor: 1500000}
	creditPayment := model.BillingPayment{ID: "8888", Status: "approved", Currency: "ARS", AmountMinor: 1500000}
	key := uuid.New()
	allocation, err := repo.RecordMerchantCreditAllocation(ctx, sourceBrand, financeUserID, 100000, key, "BANK-TRANSFER-123", creditInvoice, creditPayment)
	if err != nil || allocation.Status != "RECORDED" {
		t.Fatalf("allocation=%+v err=%v", allocation, err)
	}
	duplicate, err := repo.RecordMerchantCreditAllocation(ctx, sourceBrand, financeUserID, 100000, key, "BANK-TRANSFER-123", creditInvoice, creditPayment)
	if err != nil || duplicate.ID != allocation.ID {
		t.Fatalf("idempotent allocation=%+v err=%v", duplicate, err)
	}
	if _, err = repo.RecordMerchantCreditAllocation(ctx, sourceBrand, financeUserID, 100001, key, "BANK-TRANSFER-123", creditInvoice, creditPayment); !errors.Is(err, repository.ErrIdempotencyConflict) {
		t.Fatalf("key reused with changed amount: %v", err)
	}
	if _, err = repo.RecordMerchantCreditAllocation(ctx, sourceBrand, financeUserID, 2000000, uuid.New(), "BANK-TRANSFER-456", creditInvoice, creditPayment); !errors.Is(err, repository.ErrInsufficientBalance) {
		t.Fatalf("overdraw credit: %v", err)
	}
	account, err := repo.MerchantCreditAccount(ctx, sourceBrand)
	if err != nil || account.BalanceMinor != 2750000 || len(account.Allocations) != 1 {
		t.Fatalf("account=%+v err=%v", account, err)
	}
	creditPayment.Status = "refunded"
	creditPayment.RefundedMinor = creditPayment.AmountMinor
	if err = repo.ReverseReferralPayment(ctx, "credit-refund", creditPayment); err != nil {
		t.Fatal(err)
	}
	account, err = repo.MerchantCreditAccount(ctx, sourceBrand)
	if err != nil || account.Allocations[0].Status != "RECOVERY_DUE" || account.Allocations[0].RecoveryDueAt == nil || account.BalanceMinor != 2750000 {
		t.Fatalf("reversed credit=%+v err=%v", account, err)
	}
}
