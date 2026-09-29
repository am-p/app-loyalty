package repository_test

import (
	"context"
	"testing"

	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/repository"
)

func TestPostgresBackofficeCustomers(t *testing.T) {
	pool := referralBillingPool(t)
	ctx := t.Context()
	repo := repository.New(pool)
	empty, err := repo.ListBackofficeCustomers(ctx)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty=%+v err=%v", empty, err)
	}
	var referred, direct, campaign, code, influencer int64
	for _, fixture := range []struct {
		name string
		id   *int64
	}{{"Referred", &referred}, {"Direct", &direct}} {
		if err := pool.QueryRow(ctx, `INSERT INTO marcas(nombre) VALUES($1) RETURNING id`, fixture.name).Scan(fixture.id); err != nil {
			t.Fatal(err)
		}
	}
	if err := pool.QueryRow(ctx, `INSERT INTO referral_influencers(name,contact) VALUES('Influencer','test@example.test') RETURNING id`).Scan(&influencer); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO referral_campaigns(name,program_type,discount_bps,discount_charges,reward_bps,reward_charges,starts_at,ends_at,active) VALUES('Changed campaign','SELLOS',1000,9,2000,12,now()-interval '1 day',now()+interval '1 day',false) RETURNING id`).Scan(&campaign); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO referral_codes(code,campaign_id,source_kind,influencer_id,active) VALUES('ENTRY-CODE',$1,'INFLUENCER',$2,false) RETURNING id`, campaign, influencer).Scan(&code); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO referral_attributions(brand_id,code_id,campaign_id,source_kind,influencer_id,discount_bps,discount_charges,reward_bps,reward_charges) VALUES($1,$2,$3,'INFLUENCER',$4,5000,3,2000,12)`, referred, code, campaign, influencer); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO programas_fidelidad(marca_id,tipo,sellos_por_acumulacion,nombre_unidad,activo) VALUES($1,'SELLOS',1,'Sello',true),($1,'PUNTOS',NULL,'Punto',false)`, referred); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE marcas SET activo=false,deleted_at=now() WHERE id=$1`, referred); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO suscripciones_marca(marca_id,proveedor,proveedor_suscripcion_id,referencia_externa,estado,moneda,precio_sucursal_minor,cantidad_sucursales,importe_mensual_minor) VALUES($1,'MERCADO_PAGO','closed-sub','closed-ref','CANCELLED','ARS',10000,1,10000)`, referred); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO referral_charges(provider_invoice_id,brand_id,amount_minor,full_amount_minor,currency,status,paid_index) VALUES('paid',$1,5000,10000,'ARS','APPROVED',1),('refunded',$1,5000,10000,'ARS','REFUNDED',2),('free',$2,0,0,'ARS','APPROVED',1)`, referred, direct); err != nil {
		t.Fatal(err)
	}
	items, err := repo.ListBackofficeCustomers(ctx)
	if err != nil || len(items) != 2 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	for _, item := range items {
		if item.BrandID == referred {
			if item.Active || item.PaidCharges != 1 || item.PaidAmountMinor != 5000 || item.RefundedCharges != 1 || item.LastPaymentAt == nil || item.DiscountBPS != 5000 || item.DiscountCharges != 3 || item.DiscountRemainingCharges != 1 || item.ReferralCode == nil || *item.ReferralCode != "ENTRY-CODE" || item.SubscriptionStatus == nil || *item.SubscriptionStatus != "CANCELLED" || item.SourceName == nil || *item.SourceName != "Influencer" {
				t.Fatalf("referred=%+v", item)
			}
		} else if item.PaidCharges != 0 || item.PaidAmountMinor != 0 || item.LastPaymentAt != nil || item.ReferralCode != nil || item.CampaignName != nil || item.DiscountRemainingCharges != 0 || item.SubscriptionStatus != nil {
			t.Fatalf("direct=%+v", item)
		}
	}
}

func TestPostgresPaymentsWithoutReferral(t *testing.T) {
	pool := referralBillingPool(t)
	ctx := t.Context()
	repo := repository.New(pool)
	var brand int64
	if err := pool.QueryRow(ctx, `INSERT INTO marcas(nombre) VALUES('Direct paying customer') RETURNING id`).Scan(&brand); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO suscripciones_marca(marca_id,proveedor,proveedor_suscripcion_id,referencia_externa,estado,moneda,precio_sucursal_minor,cantidad_sucursales,importe_mensual_minor) VALUES($1,'MERCADO_PAGO','direct-sub','direct-ref','AUTHORIZED','ARS',10000,1,10000)`, brand); err != nil {
		t.Fatal(err)
	}
	advance := func(context.Context, string, int64, string) error {
		t.Fatal("direct payment must not transition referral pricing")
		return nil
	}
	invoice := model.BillingInvoice{ID: "direct-invoice", SubscriptionID: "direct-sub", PaymentID: "direct-payment", Currency: "ARS", AmountMinor: 10000}
	payment := model.BillingPayment{ID: invoice.PaymentID, Status: "approved", Currency: "ARS", AmountMinor: 10000}
	for _, status := range []string{"rejected", "pending"} {
		failed := payment
		failed.Status = status
		if err := repo.RecordReferralInvoice(ctx, status, invoice, failed, advance); err != nil {
			t.Fatal(err)
		}
	}
	free := payment
	free.AmountMinor = 0
	if err := repo.RecordReferralInvoice(ctx, "free", invoice, free, advance); err != nil {
		t.Fatal(err)
	}
	items, err := repo.ListBackofficeCustomers(ctx)
	if err != nil || len(items) != 1 || items[0].PaidCharges != 0 {
		t.Fatalf("unpaid=%+v err=%v", items, err)
	}
	for _, event := range []string{"verified", "duplicate"} {
		if err := repo.RecordReferralInvoice(ctx, event, invoice, payment, advance); err != nil {
			t.Fatal(err)
		}
	}
	items, err = repo.ListBackofficeCustomers(ctx)
	if err != nil || items[0].PaidCharges != 1 || items[0].PaidAmountMinor != 10000 || items[0].ReferralCode != nil {
		t.Fatalf("paid=%+v err=%v", items, err)
	}
	metrics, err := repo.ReferralMetrics(ctx)
	if err != nil || metrics.PayingBrands != 0 {
		t.Fatalf("referral cohort=%+v err=%v", metrics, err)
	}
	var rewards int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM referral_rewards`).Scan(&rewards); err != nil || rewards != 0 {
		t.Fatalf("rewards=%d err=%v", rewards, err)
	}
	payment.Status = "refunded"
	if err := repo.RecordReferralInvoice(ctx, "refund", invoice, payment, advance); err != nil {
		t.Fatal(err)
	}
	items, err = repo.ListBackofficeCustomers(ctx)
	if err != nil || items[0].PaidCharges != 0 || items[0].PaidAmountMinor != 0 || items[0].RefundedCharges != 1 || items[0].LastPaymentAt != nil {
		t.Fatalf("refunded=%+v err=%v", items, err)
	}
}

func TestPostgresBackofficeCustomerBranches(t *testing.T) {
	pool := referralBillingPool(t)
	ctx := t.Context()
	repo := repository.New(pool)
	var direct, closed, empty int64
	for _, fixture := range []struct {
		name string
		id   *int64
	}{{"Direct without subscription", &direct}, {"Closed without subscription", &closed}, {"Without branches", &empty}} {
		if err := pool.QueryRow(ctx, `INSERT INTO marcas(nombre) VALUES($1) RETURNING id`, fixture.name).Scan(fixture.id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE marcas SET activo=false,deleted_at=now() WHERE id=$1`, closed); err != nil {
		t.Fatal(err)
	}
	var other, primary, inactive, deleted int64
	for _, fixture := range []struct {
		name    string
		primary bool
		active  bool
		deleted bool
		id      *int64
	}{{"North", false, true, false, &other}, {"Main", true, true, false, &primary}, {"Inactive", false, false, false, &inactive}, {"Deleted", false, true, true, &deleted}} {
		if err := pool.QueryRow(ctx, `INSERT INTO sucursales(marca_id,nombre,direccion,localidad,provincia,codigo_postal,principal,activo,deleted_at) VALUES($1,$2,'Av. Prueba 123','Localidad de prueba','Provincia de prueba','1000',$3,$4,CASE WHEN $5 THEN now() END) RETURNING id`, direct, fixture.name, fixture.primary, fixture.active, fixture.deleted).Scan(fixture.id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sucursales(marca_id,nombre,activo,deleted_at) VALUES($1,'Closed branch',false,now())`, closed); err != nil {
		t.Fatal(err)
	}
	items, err := repo.ListBackofficeCustomers(ctx)
	if err != nil || len(items) != 3 {
		t.Fatalf("all registered brands=%+v err=%v", items, err)
	}
	seen := make(map[int64]bool)
	for _, item := range items {
		if seen[item.BrandID] || item.SubscriptionStatus != nil || item.ReferralCode != nil || item.PaidCharges != 0 || item.Branches == nil {
			t.Fatalf("duplicate, omitted empty array, or invented contract=%+v", item)
		}
		seen[item.BrandID] = true
		switch item.BrandID {
		case direct:
			if !item.Active || len(item.Branches) != 4 {
				t.Fatalf("direct=%+v", item)
			}
			for index, id := range []int64{primary, other, inactive, deleted} {
				branch := item.Branches[index]
				if branch.ID != id || branch.Active != (index < 2) || branch.Primary != (index == 0) || branch.Address == nil || *branch.Address != "Av. Prueba 123" || branch.Locality == nil || *branch.Locality != "Localidad de prueba" || branch.Province == nil || *branch.Province != "Provincia de prueba" || branch.PostalCode == nil || *branch.PostalCode != "1000" {
					t.Fatalf("branch order, address or effective status=%+v", branch)
				}
			}
		case closed:
			if item.Active || len(item.Branches) != 1 || item.Branches[0].Name != "Closed branch" || item.Branches[0].Active || item.Branches[0].Address != nil {
				t.Fatalf("closed history or branch isolation=%+v", item)
			}
		case empty:
			if len(item.Branches) != 0 {
				t.Fatalf("without branches=%+v", item)
			}
		default:
			t.Fatalf("unexpected brand=%+v", item)
		}
	}
}
