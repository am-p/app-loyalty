package repository_test

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
	"time"

	"clientesFrecuentes/internal/mailer"
	"clientesFrecuentes/internal/repository"
)

// Upgrade and rollback must keep the deployed influencer and subscription mail
// rows. Testing with those rows catches a narrowed enum CHECK that an empty
// database would accept.
func TestPostgresEmailChangeMigrationPreservesDeployedMail(t *testing.T) {
	pool := referralBillingPool(t)
	ctx := t.Context()
	fixture := accountDeletionFixture(t, pool, "AUTHORIZED")
	repo := repository.New(pool, []byte("01234567890123456789012345678901"))
	if err := repo.RequestSubscriptionConfirmation(ctx, fixture.owner, fixture.brand); err != nil {
		t.Fatal(err)
	}
	var admin, campaign int64
	if err := pool.QueryRow(ctx, `INSERT INTO backoffice_users(email,password_hash,totp_secret,role) VALUES('email-upgrade-admin@example.test','hash','','ADMIN_SISTEMA') RETURNING id`).Scan(&admin); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO referral_campaigns(name,program_type,discount_bps,discount_charges,reward_bps,reward_charges,starts_at,ends_at) VALUES('Email upgrade campaign','PUNTOS',1000,1,500,1,now(),now()+interval '1 day') RETURNING id`).Scan(&campaign); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateReferralInfluencerWithCode(ctx, "Migration Person", "migration-person@example.test", "EMAIL-UPGRADE-30", campaign, admin); err != nil {
		t.Fatal(err)
	}
	apply := func(version, direction string) {
		t.Helper()
		body, err := os.ReadFile(filepath.Join("..", "..", "migrations", version+"."+direction+".sql"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, string(body)); err != nil {
			t.Fatalf("%s %s: %v", version, direction, err)
		}
	}
	assertDeployedMail := func() {
		t.Helper()
		for _, kind := range []string{"INFLUENCER_WELCOME", "SUBSCRIPTION_CONFIRMATION"} {
			var count int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM email_outbox WHERE tipo=$1`, kind).Scan(&count); err != nil || count != 1 {
				t.Fatalf("deployed %s count=%d err=%v", kind, count, err)
			}
		}
	}
	queueEmailChange := func(token string) {
		t.Helper()
		var version int
		if err := pool.QueryRow(ctx, `SELECT version FROM usuarios WHERE id=$1`, fixture.owner).Scan(&version); err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256([]byte(token))
		message := mailer.EmailChangeMessage("https://app.puntazo.test", "changed-owner@example.test", token)
		if err := repo.EnqueueEmailChange(ctx, fixture.owner, version, message.To, hash[:], time.Now().Add(time.Hour), message); err != nil {
			t.Fatal(err)
		}
	}
	queueEmailChange("before-rollback-token")
	apply("0031_card_templates", "down")
	apply("0030_email_change", "down")
	assertDeployedMail()
	var removed int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM email_outbox WHERE tipo='CHANGE_EMAIL'`).Scan(&removed); err != nil || removed != 0 {
		t.Fatalf("rollback email-change rows=%d err=%v", removed, err)
	}
	apply("0030_email_change", "up")
	apply("0031_card_templates", "up")
	assertDeployedMail()
	queueEmailChange("after-upgrade-token")
	if _, err := pool.Exec(ctx, `UPDATE marcas SET plantilla_tarjeta='ORBIT_CHECKS' WHERE id=$1`, fixture.brand); err != nil {
		t.Fatal(err)
	}
	apply("0031_card_templates", "down")
	var templateRemoved bool
	if err := pool.QueryRow(ctx, `SELECT plantilla_tarjeta IS NULL FROM marcas WHERE id=$1`, fixture.brand).Scan(&templateRemoved); err != nil || !templateRemoved {
		t.Fatalf("new template rollback=%t err=%v", templateRemoved, err)
	}
	assertDeployedMail()
}
