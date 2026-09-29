package repository_test

import (
	"errors"
	"testing"

	"clientesFrecuentes/internal/repository"
)

func TestPostgresInfluencerEarnings(t *testing.T) {
	pool := referralBillingPool(t)
	ctx := t.Context()
	repo := repository.New(pool)
	var influencer, other, campaign, codeA, codeB, brandA, brandB, unpaid int64
	for _, x := range []struct {
		name string
		id   *int64
	}{{"Report influencer", &influencer}, {"Other influencer", &other}} {
		if err := pool.QueryRow(ctx, `INSERT INTO referral_influencers(name,contact) VALUES($1,'report@example.test') RETURNING id`, x.name).Scan(x.id); err != nil {
			t.Fatal(err)
		}
	}
	if err := pool.QueryRow(ctx, `INSERT INTO referral_campaigns(name,program_type,discount_bps,discount_charges,reward_bps,reward_charges,starts_at,ends_at) VALUES('Report campaign','SELLOS',0,0,9999,36,now()-interval '1 day',now()+interval '1 day') RETURNING id`).Scan(&campaign); err != nil {
		t.Fatal(err)
	}
	for _, x := range []struct {
		code string
		id   *int64
	}{{"REPORT-A", &codeA}, {"REPORT-B", &codeB}} {
		if err := pool.QueryRow(ctx, `INSERT INTO referral_codes(code,campaign_id,source_kind,influencer_id,active) VALUES($1,$2,'INFLUENCER',$3,false) RETURNING id`, x.code, campaign, influencer).Scan(x.id); err != nil {
			t.Fatal(err)
		}
	}
	for _, x := range []struct {
		name string
		id   *int64
		code int64
	}{{"Client A", &brandA, codeA}, {"Client B", &brandB, codeB}, {"Without payments", &unpaid, codeA}} {
		if err := pool.QueryRow(ctx, `INSERT INTO marcas(nombre) VALUES($1) RETURNING id`, x.name).Scan(x.id); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO referral_attributions(brand_id,code_id,campaign_id,source_kind,influencer_id,discount_bps,discount_charges,reward_bps,reward_charges) VALUES($1,$2,$3,'INFLUENCER',$4,0,0,2000,12)`, *x.id, x.code, campaign, influencer); err != nil {
			t.Fatal(err)
		}
	}
	// Closed clients and paused codes retain their historical commissions.
	if _, err := pool.Exec(ctx, `UPDATE marcas SET activo=false,deleted_at=now() WHERE id=$1`, brandA); err != nil {
		t.Fatal(err)
	}
	fixtures := []struct {
		invoice        string
		brand          int64
		at, status     string
		amount, reward int64
	}{
		{"jan-a", brandA, "2026-02-01T02:59:59Z", "PENDING", 10000, 2000},
		{"feb-a", brandA, "2026-02-01T03:00:00Z", "SETTLED", 20000, 4000},
		{"feb-b", brandB, "2026-02-12T12:00:00Z", "PENDING", 30000, 6000},
		{"feb-void", brandB, "2026-02-13T12:00:00Z", "VOID", 10000, 2000},
		{"feb-recover", brandA, "2026-02-14T12:00:00Z", "RECOVERY_DUE", 20000, 4000},
		{"march-zero", brandB, "2026-03-05T12:00:00Z", "", 50000, 0},
	}
	for _, x := range fixtures {
		chargeStatus := "APPROVED"
		if x.status == "VOID" || x.status == "RECOVERY_DUE" {
			chargeStatus = "REFUNDED"
		}
		if _, err := pool.Exec(ctx, `INSERT INTO referral_charges(provider_invoice_id,brand_id,amount_minor,full_amount_minor,currency,status,created_at) VALUES($1,$2,$3,$3,'ARS',$4,$5::timestamptz)`, x.invoice, x.brand, x.amount, chargeStatus, x.at); err != nil {
			t.Fatal(err)
		}
		if x.reward > 0 {
			// Reward timestamps and settlement date intentionally differ from charge month.
			if _, err := pool.Exec(ctx, `INSERT INTO referral_rewards(provider_invoice_id,brand_id,source_kind,influencer_id,amount_minor,status,created_at,settled_at) VALUES($1,$2,'INFLUENCER',$3,$4,$5,'2026-09-20T12:00:00Z','2026-09-25T12:00:00Z')`, x.invoice, x.brand, influencer, x.reward, x.status); err != nil {
				t.Fatal(err)
			}
		}
	}
	report, err := repo.InfluencerEarnings(ctx, influencer, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if report.Currency != "ARS" || report.DateBasis != "CHARGE_RECORDED_AT" || report.TimeZone != "America/Argentina/Buenos_Aires" || report.InfluencerName != "Report influencer" {
		t.Fatalf("identity=%+v", report)
	}
	want := repository.InfluencerEarningsTotals{PaidCharges: 4, PaidAmountMinor: 110000, GeneratedMinor: 18000, PendingMinor: 8000, SettledMinor: 4000, VoidMinor: 2000, RecoveryDueMinor: 4000}
	if report.Totals != want {
		t.Fatalf("totals=%+v want=%+v", report.Totals, want)
	}
	if len(report.Clients) != 3 || report.Clients[0].BrandID != brandA || report.Clients[0].RewardBPS != 2000 || report.Clients[0].PendingMinor != 2000 || report.Clients[1].Code != "REPORT-B" || report.Clients[2].GeneratedMinor != 0 {
		t.Fatalf("clients=%+v", report.Clients)
	}
	if len(report.Months) != 2 || report.Months[0].Month != "2026-03" || report.Months[0].Totals.GeneratedMinor != 0 || report.Months[1].Month != "2026-02" || report.Months[1].Totals.PendingMinor != 6000 || len(report.Months[1].Clients) != 2 || report.NextCursor == nil || *report.NextCursor != "2026-02" {
		t.Fatalf("page=%+v", report)
	}
	older, err := repo.InfluencerEarnings(ctx, influencer, *report.NextCursor, 2)
	if err != nil || len(older.Months) != 1 || older.Months[0].Month != "2026-01" || older.Months[0].Totals.PendingMinor != 2000 || older.NextCursor != nil || older.Totals != want {
		t.Fatalf("older=%+v err=%v", older, err)
	}
	empty, err := repo.InfluencerEarnings(ctx, other, "", 12)
	if err != nil || empty.Clients == nil || len(empty.Clients) != 0 || empty.Months == nil || len(empty.Months) != 0 || empty.Totals.PendingMinor != 0 {
		t.Fatalf("empty=%+v err=%v", empty, err)
	}
	if _, err = repo.InfluencerEarnings(ctx, 999999, "", 12); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("missing err=%v", err)
	}
	for _, before := range []string{"2026-13", "2026-1", "2026-02-01", "0000-01", "bad"} {
		if _, err = repo.InfluencerEarnings(ctx, influencer, before, 12); !errors.Is(err, repository.ErrInvalidRequest) {
			t.Fatalf("before=%s err=%v", before, err)
		}
	}
	for _, limit := range []int{0, 25} {
		if _, err = repo.InfluencerEarnings(ctx, influencer, "", limit); !errors.Is(err, repository.ErrInvalidRequest) {
			t.Fatalf("limit=%d err=%v", limit, err)
		}
	}
	// The report must not inherit the legacy rewards listing's 1,000-record cutoff.
	if _, err := pool.Exec(ctx, `INSERT INTO referral_charges(provider_invoice_id,brand_id,amount_minor,full_amount_minor,currency,status,created_at) SELECT 'bulk-'||n,$1,100,100,'ARS','APPROVED','2025-12-15T12:00:00Z' FROM generate_series(1,1001) n`, brandB); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO referral_rewards(provider_invoice_id,brand_id,source_kind,influencer_id,amount_minor,status) SELECT 'bulk-'||n,$1,'INFLUENCER',$2,20,'PENDING' FROM generate_series(1,1001) n`, brandB, influencer); err != nil {
		t.Fatal(err)
	}
	report, err = repo.InfluencerEarnings(ctx, influencer, "", 1)
	if err != nil || report.Totals.PendingMinor != 28020 || len(report.Months) != 1 || report.Months[0].Month != "2026-03" {
		t.Fatalf("bulk=%+v err=%v", report, err)
	}
	var finance int64
	if err := pool.QueryRow(ctx, `INSERT INTO backoffice_users(email,password_hash,totp_secret,role) VALUES('report-finance@example.test','hash','secret','FINANZAS') RETURNING id`).Scan(&finance); err != nil {
		t.Fatal(err)
	}
	if err := repo.SettleReferralReward(ctx, "jan-a", finance); err != nil {
		t.Fatal(err)
	}
	report, err = repo.InfluencerEarnings(ctx, influencer, "2026-02", 12)
	if err != nil || report.Totals.PendingMinor != 26020 || report.Months[0].Month != "2026-01" || report.Months[0].Totals.PendingMinor != 0 || report.Months[0].Totals.SettledMinor != 2000 {
		t.Fatalf("settled=%+v err=%v", report, err)
	}
}
