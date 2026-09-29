package repository_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/repository"
	"github.com/google/uuid"
)

func TestPostgresSubscriptionPricesAndScope(t *testing.T) {
	pool := referralBillingPool(t)
	ctx := t.Context()
	repo := repository.New(pool)
	var admin, owner, brand, points int64
	if err := pool.QueryRow(ctx, `INSERT INTO backoffice_users(email,password_hash,totp_secret,role) VALUES('prices-admin@example.test','hash','secret','ADMIN_SISTEMA') RETURNING id`).Scan(&admin); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO usuarios(email,password_hash,nombre,tipo_cuenta) VALUES('prices-owner@example.test','hash','Owner','PERSONAL_MARCA') RETURNING id`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		name, program string
		id            *int64
	}{{"Stamps", "SELLOS", &brand}, {"Points", "PUNTOS", &points}} {
		if err := pool.QueryRow(ctx, `INSERT INTO marcas(nombre) VALUES($1) RETURNING id`, fixture.name).Scan(fixture.id); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO programas_fidelidad(marca_id,tipo,sellos_por_acumulacion,nombre_unidad) VALUES($1,$2,CASE WHEN $2='SELLOS' THEN 1 ELSE NULL END,'Unit')`, *fixture.id, fixture.program); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO membresias_marca(usuario_id,marca_id,rol) VALUES($1,$2,'PROPIETARIO');`, owner, *fixture.id); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO sucursales(marca_id,nombre) VALUES($1,'Main')`, *fixture.id); err != nil {
			t.Fatal(err)
		}
	}
	initial, err := repo.SubscriptionPrice(ctx, "SELLOS", 10000)
	if err != nil || initial.UnitPriceMinor != 10000 || initial.Version != 0 || initial.Customized {
		t.Fatalf("fallback=%+v err=%v", initial, err)
	}
	_, reserved, err := repo.ReserveSubscriptionCheckout(ctx, owner, brand, "existing-reference", 10000, 20000)
	if err != nil || reserved.Subscription.MonthlyAmountCents != 10000 {
		t.Fatalf("reserved=%+v err=%v", reserved, err)
	}
	key := uuid.New()
	change, err := repo.ChangeSubscriptionPrice(ctx, admin, key, "SELLOS", 30000, 0, 10000, false)
	if err != nil || len(change.Items) != 0 {
		t.Fatalf("future-only=%+v err=%v", change, err)
	}
	_, existing, err := repo.ReserveSubscriptionCheckout(ctx, owner, brand, "existing-reference", 10000, 20000)
	if err != nil || existing.Subscription.MonthlyAmountCents != 10000 || existing.Subscription.FullMonthlyAmountCents != 10000 {
		t.Fatalf("existing changed=%+v err=%v", existing, err)
	}
	replayed, err := repo.ChangeSubscriptionPrice(ctx, admin, key, "SELLOS", 30000, 0, 10000, false)
	if err != nil || replayed.PriceVersion != change.PriceVersion {
		t.Fatalf("replay=%+v err=%v", replayed, err)
	}
	if _, err = repo.ChangeSubscriptionPrice(ctx, admin, key, "SELLOS", 31000, 0, 10000, false); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("key reuse err=%v", err)
	}
	if _, err = repo.ChangeSubscriptionPrice(ctx, admin, uuid.New(), "SELLOS", 40000, 0, 10000, false); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("stale version err=%v", err)
	}
	var next int64
	if err = pool.QueryRow(ctx, `INSERT INTO marcas(nombre) VALUES('New stamps') RETURNING id`).Scan(&next); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{`INSERT INTO programas_fidelidad(marca_id,tipo,sellos_por_acumulacion,nombre_unidad) VALUES($1,'SELLOS',1,'Stamp')`, `INSERT INTO sucursales(marca_id,nombre) VALUES($1,'Main')`} {
		if _, err = pool.Exec(ctx, sql, next); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = pool.Exec(ctx, `INSERT INTO membresias_marca(usuario_id,marca_id,rol) VALUES($1,$2,'PROPIETARIO')`, owner, next); err != nil {
		t.Fatal(err)
	}
	_, newCheckout, err := repo.ReserveSubscriptionCheckout(ctx, owner, next, "new-reference", 10000, 20000)
	if err != nil || newCheckout.Subscription.MonthlyAmountCents != 30000 {
		t.Fatalf("new price not used=%+v err=%v", newCheckout, err)
	}
	_, pointCheckout, err := repo.ReserveSubscriptionCheckout(ctx, owner, points, "point-reference", 10000, 20000)
	if err != nil || pointCheckout.Subscription.MonthlyAmountCents != 20000 {
		t.Fatalf("points affected=%+v err=%v", pointCheckout, err)
	}
	customers, err := repo.ListBackofficeCustomers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, customer := range customers {
		if customer.BrandID == points && (customer.ProgramType == nil || *customer.ProgramType != "PUNTOS" || customer.MonthlyAmountMinor == nil || *customer.MonthlyAmountMinor != 20000) {
			t.Fatalf("points list=%+v", customer)
		}
	}
	for _, test := range []struct {
		program string
		amount  int64
	}{{"UNKNOWN", 10000}, {"SELLOS", 0}, {"SELLOS", repository.MaxSubscriptionAmountMinor + 1}} {
		if _, err = repo.ChangeSubscriptionPrice(ctx, admin, uuid.New(), test.program, test.amount, 1, 10000, false); !errors.Is(err, repository.ErrInvalidRequest) {
			t.Fatalf("invalid=%+v err=%v", test, err)
		}
	}
	var audits int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM backoffice_audit WHERE action='subscription_price.change'`).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("audits=%d err=%v", audits, err)
	}
	// Two administrators saving the same version must not overwrite each other.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := repo.ChangeSubscriptionPrice(ctx, admin, uuid.New(), "SELLOS", int64(40000+i), 1, 10000, false)
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	success, conflicts := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, repository.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("success=%d conflicts=%d", success, conflicts)
	}
}

func TestPostgresExistingSubscriptionPriceChanges(t *testing.T) {
	pool := referralBillingPool(t)
	ctx := t.Context()
	repo := repository.New(pool)
	var admin int64
	if err := pool.QueryRow(ctx, `INSERT INTO backoffice_users(email,password_hash,totp_secret,role) VALUES('bulk-admin@example.test','hash','secret','ADMIN_SISTEMA') RETURNING id`).Scan(&admin); err != nil {
		t.Fatal(err)
	}
	brands := map[string]int64{}
	for _, fixture := range []struct {
		name, program, status string
		active                bool
	}{{"Direct", "SELLOS", "AUTHORIZED", true}, {"Discounted", "SELLOS", "PAUSED", true}, {"Cancelled", "SELLOS", "CANCELLED", true}, {"Closed", "SELLOS", "AUTHORIZED", false}, {"Points", "PUNTOS", "AUTHORIZED", true}} {
		var brand int64
		if err := pool.QueryRow(ctx, `INSERT INTO marcas(nombre,activo) VALUES($1,$2) RETURNING id`, fixture.name, fixture.active).Scan(&brand); err != nil {
			t.Fatal(err)
		}
		brands[fixture.name] = brand
		if _, err := pool.Exec(ctx, `INSERT INTO suscripciones_marca(marca_id,proveedor,proveedor_suscripcion_id,referencia_externa,estado,moneda,precio_sucursal_minor,cantidad_sucursales,importe_mensual_minor,program_type) VALUES($1,'MERCADO_PAGO',$2,$2,$3,'ARS',10000,2,20000,$4)`, brand, "sub-"+fixture.name, fixture.status, fixture.program); err != nil {
			t.Fatal(err)
		}
	}
	var campaign, influencer, code int64
	if err := pool.QueryRow(ctx, `INSERT INTO referral_campaigns(name,program_type,discount_bps,discount_charges,reward_bps,reward_charges,starts_at,ends_at) VALUES('Promo','SELLOS',5000,3,2000,12,now()-interval '1 day',now()+interval '1 day') RETURNING id`).Scan(&campaign); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO referral_influencers(name,contact) VALUES('Referrer','test@example.test') RETURNING id`).Scan(&influencer); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO referral_codes(code,campaign_id,source_kind,influencer_id) VALUES('PROMO-CODE',$1,'INFLUENCER',$2) RETURNING id`, campaign, influencer).Scan(&code); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO referral_attributions(brand_id,code_id,campaign_id,source_kind,influencer_id,discount_bps,discount_charges,reward_bps,reward_charges) VALUES($1,$2,$3,'INFLUENCER',$4,5000,3,2000,12)`, brands["Discounted"], code, campaign, influencer); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE suscripciones_marca SET precio_sucursal_minor=5000,full_unit_price_minor=10000,importe_mensual_minor=10000 WHERE marca_id=$1`, brands["Discounted"]); err != nil {
		t.Fatal(err)
	}
	preview, err := repo.PreviewSubscriptionPrice(ctx, "SELLOS", 30000)
	if err != nil || len(preview) != 2 {
		t.Fatalf("preview=%+v err=%v", preview, err)
	}
	for _, item := range preview {
		want := int64(60000)
		if item.BrandID == brands["Discounted"] {
			want = 30000
		}
		if item.NewAmountMinor != want {
			t.Fatalf("preview item=%+v", item)
		}
	}
	job, err := repo.ChangeSubscriptionPrice(ctx, admin, uuid.New(), "SELLOS", 30000, 0, 10000, true)
	if err != nil || len(job.Items) != 2 {
		t.Fatalf("job=%+v err=%v", job, err)
	}
	handled, err := repo.ApplyNextSubscriptionPriceChange(ctx, func(context.Context, string, int64, string) error { return fmt.Errorf("simulated provider outage") })
	if !handled || err == nil {
		t.Fatalf("failure handled=%t err=%v", handled, err)
	}
	job, err = repo.SubscriptionPriceChange(ctx, uuid.MustParse(job.ID))
	if err != nil {
		t.Fatal(err)
	}
	failed := 0
	for _, item := range job.Items {
		if item.Status == "FAILED" {
			failed++
		}
	}
	if failed != 1 {
		t.Fatalf("failed items=%+v", job.Items)
	}
	if err = repo.RetrySubscriptionPriceChange(ctx, uuid.MustParse(job.ID), admin); err != nil {
		t.Fatal(err)
	}
	called := map[string]int64{}
	for range 2 {
		handled, err = repo.ApplyNextSubscriptionPriceChange(ctx, func(_ context.Context, id string, amount int64, key string) error {
			if key == "" {
				t.Fatal("missing idempotency")
			}
			called[id] = amount
			return nil
		})
		if !handled || err != nil {
			t.Fatalf("apply=%t err=%v", handled, err)
		}
	}
	if called["sub-Direct"] != 60000 || called["sub-Discounted"] != 30000 || len(called) != 2 {
		t.Fatalf("provider changes=%+v", called)
	}
	var history int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM subscription_price_history`).Scan(&history); err != nil || history != 2 {
		t.Fatalf("history=%d err=%v", history, err)
	}
	job, err = repo.SubscriptionPriceChange(ctx, uuid.MustParse(job.ID))
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range job.Items {
		if item.Status != "APPLIED" {
			t.Fatalf("not applied=%+v", item)
		}
	}
	// An older invoice is valid at its original price; a newly issued mismatch is not.
	var issued time.Time
	if err = pool.QueryRow(ctx, `SELECT valid_from+(valid_until-valid_from)/2 FROM subscription_price_history WHERE provider_subscription_id='sub-Direct'`).Scan(&issued); err != nil {
		t.Fatal(err)
	}
	invoice := model.BillingInvoice{ID: "old-invoice", SubscriptionID: "sub-Direct", PaymentID: "old-payment", Currency: "ARS", AmountMinor: 20000, CreatedAt: issued}
	payment := model.BillingPayment{ID: "old-payment", Status: "approved", Currency: "ARS", AmountMinor: 20000}
	if err = repo.RecordReferralInvoice(ctx, "old-event", invoice, payment, func(context.Context, string, int64, string) error { t.Fatal("direct transition"); return nil }); err != nil {
		t.Fatal(err)
	}
	invoice.ID = "wrong-new-invoice"
	invoice.PaymentID = "wrong-new-payment"
	invoice.CreatedAt = time.Now().Add(time.Hour)
	payment.ID = invoice.PaymentID
	if err = repo.RecordReferralInvoice(ctx, "wrong-event", invoice, payment, nil); err == nil {
		t.Fatal("new invoice accepted an old price")
	}
	// A newer catalog revision supersedes a queued change before provider dispatch.
	queued, err := repo.ChangeSubscriptionPrice(ctx, admin, uuid.New(), "SELLOS", 40000, 1, 10000, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.ChangeSubscriptionPrice(ctx, admin, uuid.New(), "SELLOS", 50000, 2, 10000, false); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		_, err = repo.ApplyNextSubscriptionPriceChange(ctx, func(context.Context, string, int64, string) error {
			t.Fatal("obsolete job called provider")
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	queued, err = repo.SubscriptionPriceChange(ctx, uuid.MustParse(queued.ID))
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range queued.Items {
		if item.Status != "SKIPPED" {
			t.Fatalf("obsolete item=%+v", item)
		}
	}
}
