package repository_test

import (
	"errors"
	"sync"
	"testing"

	"clientesFrecuentes/internal/repository"
)

func TestPostgresInfluencerRegistrationWithCode(t *testing.T) {
	pool := referralBillingPool(t)
	ctx := t.Context()
	repo := repository.New(pool, []byte("01234567890123456789012345678901"))
	var admin, campaign int64
	if err := pool.QueryRow(ctx, `INSERT INTO backoffice_users(email,password_hash,totp_secret,role) VALUES('influencer-admin@example.test','hash','secret','ADMIN_SISTEMA') RETURNING id`).Scan(&admin); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO referral_campaigns(name,program_type,discount_bps,discount_charges,reward_bps,reward_charges,starts_at,ends_at) VALUES('Registration campaign','SELLOS',5000,3,2000,12,now()-interval '1 day',now()+interval '90 days') RETURNING id`).Scan(&campaign); err != nil {
		t.Fatal(err)
	}
	out, err := repo.CreateReferralInfluencerWithCode(ctx, "  María Local  ", "  MARIA@EXAMPLE.TEST  ", " maria-barrio ", campaign, admin)
	if err != nil || out.Name != "María Local" || out.Email != "maria@example.test" || out.Contact != out.Email || out.Code.Code != "MARIA-BARRIO" || out.Code.CampaignID != campaign || out.Code.InfluencerID == nil || *out.Code.InfluencerID != out.ID || !out.Code.Active || !out.EmailQueued {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	validation, err := repo.ValidateReferralCode(ctx, out.Code.Code, "SELLOS")
	if err != nil || validation.Code != out.Code.Code {
		t.Fatalf("usable=%+v err=%v", validation, err)
	}
	assertCounts := func(influencers, codes, audits int64) {
		t.Helper()
		var people, codeCount, auditCount, emails int64
		if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM referral_influencers),(SELECT count(*) FROM referral_codes),(SELECT count(*) FROM backoffice_audit),(SELECT count(*) FROM email_outbox WHERE tipo='INFLUENCER_WELCOME')`).Scan(&people, &codeCount, &auditCount, &emails); err != nil {
			t.Fatal(err)
		}
		if people != influencers || codeCount != codes || auditCount != audits || emails != codes {
			t.Fatalf("counts=%d/%d/%d expected=%d/%d/%d", people, codeCount, auditCount, influencers, codes, audits)
		}
	}
	assertCounts(1, 1, 2)
	// Losing the encryption dependency must leave no partial registration.
	if _, err := repository.New(pool).CreateReferralInfluencerWithCode(ctx, "No Mail", "nomail@example.test", "NO-MAIL", campaign, admin); !errors.Is(err, repository.ErrEmailUnavailable) {
		t.Fatalf("missing key=%v", err)
	}
	assertCounts(1, 1, 2)
	if _, err = repo.CreateReferralInfluencerWithCode(ctx, "Duplicate Person", "other@example.test", out.Code.Code, campaign, admin); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("duplicate=%v", err)
	}
	assertCounts(1, 1, 2)
	for _, fixture := range []struct {
		email, code string
		campaign    int64
	}{
		{"invalid", "INVALID-CODE", campaign}, {"Person <mail@example.test>", "BAD-MAIL", campaign}, {"other@example.test", "BAD CODE", campaign}, {"other@example.test", "PZ-M-1-1", campaign}, {"other@example.test", "NO-CAMPAIGN", 99999},
	} {
		if _, err = repo.CreateReferralInfluencerWithCode(ctx, "Invalid Person", fixture.email, fixture.code, fixture.campaign, admin); !errors.Is(err, repository.ErrInvalidRequest) {
			t.Fatalf("invalid=%+v err=%v", fixture, err)
		}
		assertCounts(1, 1, 2)
	}
	// A failure to record its audit must also roll back profile and code.
	if _, err = repo.CreateReferralInfluencerWithCode(ctx, "Missing Admin", "noadmin@example.test", "NO-ADMIN", campaign, 99999); err == nil {
		t.Fatal("expected failed audit")
	}
	assertCounts(1, 1, 2)
	// Simultaneous attempts to reserve a code create exactly one person.
	var group sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := repo.CreateReferralInfluencerWithCode(ctx, "Concurrent Person", "concurrent@example.test", "CONCURRENT", campaign, admin)
			results <- err
		}()
	}
	group.Wait()
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
		t.Fatalf("concurrent=%d/%d", success, conflicts)
	}
	assertCounts(2, 2, 4)
	// Existing non-email contacts remain visible without being mislabelled.
	legacy, err := repo.CreateReferralInfluencer(ctx, "Legacy Person", "@legacy-social", admin)
	if err != nil {
		t.Fatal(err)
	}
	people, err := repo.ListReferralInfluencers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, person := range people {
		if person["id"] == legacy && (person["email"] != nil || person["contact"] != "@legacy-social") {
			t.Fatalf("legacy=%+v", person)
		}
		if person["id"] == out.ID {
			if person["email"] != out.Email {
				t.Fatalf("email=%+v", person)
			}
		}
	}
}
