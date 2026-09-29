package repository_test

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"clientesFrecuentes/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

func campaignAdmin(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(t.Context(), `INSERT INTO backoffice_users(email,password_hash,totp_secret,role) VALUES('campaign-admin@example.test','unused','unused','ADMIN_SISTEMA') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func campaignFixture() repository.ReferralCampaign {
	return repository.ReferralCampaign{Name: "Both programs", ProgramTypes: []string{"SELLOS", "PUNTOS"}, DiscountBPS: 5000, DiscountCharges: 3, RewardBPS: 2000, RewardCharges: 12, StartsAt: time.Now().Add(-time.Hour), EndsAt: time.Now().Add(24 * time.Hour)}
}

func TestPostgresReferralCampaignEditing(t *testing.T) {
	pool := referralBillingPool(t)
	ctx := t.Context()
	repo := repository.New(pool, []byte("01234567890123456789012345678901"))
	admin := campaignAdmin(t, pool)
	var stamps, points int64
	for _, fixture := range []struct {
		program string
		id      *int64
	}{{"SELLOS", &stamps}, {"PUNTOS", &points}} {
		if err := pool.QueryRow(ctx, `INSERT INTO marcas(nombre) VALUES($1) RETURNING id`, fixture.program).Scan(fixture.id); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO programas_fidelidad(marca_id,tipo,sellos_por_acumulacion,nombre_unidad) VALUES($1,$2,CASE WHEN $2='SELLOS' THEN 1 END,'Unidad')`, *fixture.id, fixture.program); err != nil {
			t.Fatal(err)
		}
	}
	campaign, err := repo.CreateReferralCampaign(ctx, campaignFixture(), admin)
	if err != nil || campaign.Version != 1 || campaign.ProgramType != "SELLOS" || len(campaign.ProgramTypes) != 2 {
		t.Fatalf("campaign=%+v err=%v", campaign, err)
	}
	var merchantCodes int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM referral_codes WHERE campaign_id=$1 AND source_kind='MERCHANT'`, campaign.ID).Scan(&merchantCodes); err != nil || merchantCodes != 2 {
		t.Fatalf("merchant codes=%d err=%v", merchantCodes, err)
	}
	influencer, err := repo.CreateReferralInfluencerWithCode(ctx, "Test influencer", "campaign@example.test", "BOTH-CODE", campaign.ID, admin)
	if err != nil {
		t.Fatal(err)
	}
	for _, program := range []string{"", "SELLOS", "PUNTOS"} {
		valid, err := repo.ValidateReferralCode(ctx, influencer.Code.Code, program)
		expected := program
		if expected == "" {
			expected = "SELLOS"
		}
		if err != nil || valid.ProgramType != expected {
			t.Fatalf("validation %s=%+v err=%v", program, valid, err)
		}
	}
	for _, program := range []string{"SELLOS", "PUNTOS"} {
		var target int64
		if err = pool.QueryRow(ctx, `INSERT INTO marcas(nombre) VALUES($1) RETURNING id`, "Target "+program).Scan(&target); err != nil {
			t.Fatal(err)
		}
		tx, e := pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		e = repository.AttributeReferral(ctx, tx, target, program, influencer.Code.Code)
		if e != nil {
			tx.Rollback(ctx)
			t.Fatal(e)
		}
		if e = tx.Commit(ctx); e != nil {
			t.Fatal(e)
		}
	}
	changed := campaign
	changed.Name = "Edited campaign"
	changed.ProgramType = "PUNTOS"
	changed.ProgramTypes = []string{"PUNTOS"}
	changed.DiscountBPS = 2500
	changed.RewardBPS = 1000
	updated, err := repo.UpdateReferralCampaign(ctx, campaign.ID, admin, changed)
	if err != nil || updated.Version != 2 || !updated.Active || updated.ProgramType != "PUNTOS" {
		t.Fatalf("updated=%+v err=%v", updated, err)
	}
	if _, err = repo.ValidateReferralCode(ctx, "BOTH-CODE", "SELLOS"); !errors.Is(err, repository.ErrReferralCodeInvalid) {
		t.Fatalf("removed program accepted: %v", err)
	}
	if _, err = repo.ValidateReferralCode(ctx, "BOTH-CODE", "PUNTOS"); err != nil {
		t.Fatal(err)
	}
	var preserved int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM referral_attributions WHERE campaign_id=$1 AND discount_bps=5000 AND reward_bps=2000`, campaign.ID).Scan(&preserved); err != nil || preserved != 2 {
		t.Fatalf("snapshots changed count=%d err=%v", preserved, err)
	}
	if _, err = repo.UpdateReferralCampaign(ctx, campaign.ID, admin, changed); !errors.Is(err, repository.ErrCampaignChanged) {
		t.Fatalf("stale update=%v", err)
	}
	rollback := updated
	rollback.Name = "Must roll back"
	rollback.ProgramType = "SELLOS"
	rollback.ProgramTypes = []string{"SELLOS", "PUNTOS"}
	if _, err = repo.UpdateReferralCampaign(ctx, campaign.ID, 99999, rollback); err == nil {
		t.Fatal("missing audit actor accepted")
	}
	items, err := repo.ListReferralCampaigns(ctx)
	if err != nil || len(items) != 1 || items[0].Name != "Edited campaign" || items[0].Version != 2 {
		t.Fatalf("rollback=%+v err=%v", items, err)
	}
	invalid := updated
	invalid.ProgramTypes = []string{"SELLOS", "SELLOS"}
	if _, err = repo.UpdateReferralCampaign(ctx, campaign.ID, admin, invalid); !errors.Is(err, repository.ErrInvalidRequest) {
		t.Fatalf("duplicate programs=%v", err)
	}
	invalid = updated
	invalid.EndsAt = invalid.StartsAt
	if _, err = repo.UpdateReferralCampaign(ctx, campaign.ID, admin, invalid); !errors.Is(err, repository.ErrInvalidRequest) {
		t.Fatalf("invalid window=%v", err)
	}
	if _, err = repo.UpdateReferralCampaign(ctx, 99999, admin, updated); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("missing campaign=%v", err)
	}
	overlap := campaignFixture()
	if _, err = repo.CreateReferralCampaign(ctx, overlap, admin); !errors.Is(err, repository.ErrCampaignOverlap) {
		t.Fatalf("dual overlap accepted=%v", err)
	}
	if err = repo.SetReferralCampaignActive(ctx, campaign.ID, admin, false); err != nil {
		t.Fatal(err)
	}
	overlap.ProgramTypes = []string{"SELLOS"}
	other, err := repo.CreateReferralCampaign(ctx, overlap, admin)
	if err != nil {
		t.Fatal(err)
	}
	items, err = repo.ListReferralCampaigns(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.ID == campaign.ID {
			changed = item
		}
	}
	changed.ProgramType = "SELLOS"
	changed.ProgramTypes = []string{"SELLOS", "PUNTOS"}
	paused, err := repo.UpdateReferralCampaign(ctx, campaign.ID, admin, changed)
	if err != nil || paused.Active {
		t.Fatalf("paused edit=%+v err=%v", paused, err)
	}
	if err = repo.SetReferralCampaignActive(ctx, campaign.ID, admin, true); !errors.Is(err, repository.ErrCampaignOverlap) {
		t.Fatalf("overlapping activation=%v", err)
	}
	if err = repo.SetReferralCampaignActive(ctx, other.ID, admin, false); err != nil {
		t.Fatal(err)
	}
	if err = repo.SetReferralCampaignActive(ctx, campaign.ID, admin, true); err != nil {
		t.Fatal(err)
	}
	var updates int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM backoffice_audit WHERE action='campaign.update'`).Scan(&updates); err != nil || updates != 2 {
		t.Fatalf("update audit=%d err=%v", updates, err)
	}
}

func TestPostgresReferralCampaignConcurrency(t *testing.T) {
	pool := referralBillingPool(t)
	ctx := t.Context()
	repo := repository.New(pool)
	admin := campaignAdmin(t, pool)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, programs := range [][]string{{"SELLOS", "PUNTOS"}, {"PUNTOS"}} {
		wg.Add(1)
		go func(programs []string) {
			defer wg.Done()
			x := campaignFixture()
			x.ProgramTypes = programs
			_, err := repo.CreateReferralCampaign(ctx, x, admin)
			results <- err
		}(programs)
	}
	wg.Wait()
	close(results)
	success, conflicts := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, repository.ErrCampaignOverlap) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("create success=%d conflicts=%d", success, conflicts)
	}
	items, err := repo.ListReferralCampaigns(ctx)
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	results = make(chan error, 2)
	for _, name := range []string{"Edit A", "Edit B"} {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			x := items[0]
			x.Name = name
			_, err := repo.UpdateReferralCampaign(ctx, x.ID, admin, x)
			results <- err
		}(name)
	}
	wg.Wait()
	close(results)
	success, conflicts = 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, repository.ErrCampaignChanged) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("edit success=%d conflicts=%d", success, conflicts)
	}
	items, err = repo.ListReferralCampaigns(ctx)
	if err != nil || items[0].Version != 2 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
}

func TestPostgresReferralCampaignMigration(t *testing.T) {
	pool := referralBillingPool(t)
	ctx := t.Context()
	migration := func(suffix string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0027_referral_campaign_editing."+suffix+".sql"))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	if _, err := pool.Exec(ctx, migration("down")); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO referral_campaigns(name,program_type,discount_bps,discount_charges,reward_bps,reward_charges,starts_at,ends_at) VALUES('Legacy points','PUNTOS',2500,2,1000,6,now(),now()+interval '1 day') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, migration("up")); err != nil {
		t.Fatal(err)
	}
	repo := repository.New(pool)
	items, err := repo.ListReferralCampaigns(ctx)
	if err != nil || len(items) != 1 || items[0].ProgramType != "PUNTOS" || len(items[0].ProgramTypes) != 1 || items[0].ProgramTypes[0] != "PUNTOS" || items[0].Version != 1 {
		t.Fatalf("backfill=%+v err=%v", items, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE referral_campaigns SET program_type='SELLOS',program_types=ARRAY['SELLOS','PUNTOS'] WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, migration("down")); err == nil {
		t.Fatal("dual campaign coverage silently lost")
	}
	items, err = repo.ListReferralCampaigns(ctx)
	if err != nil || len(items[0].ProgramTypes) != 2 {
		t.Fatalf("failed rollback modified schema=%+v err=%v", items, err)
	}
}
