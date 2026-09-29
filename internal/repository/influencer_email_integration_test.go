package repository_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"clientesFrecuentes/internal/mailer"
	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/repository"
)

func TestPostgresInfluencerWelcomeSnapshotRetryAndScrubbing(t *testing.T) {
	pool := referralBillingPool(t)
	ctx := t.Context()
	key := []byte("01234567890123456789012345678901")
	repo := repository.New(pool, key)
	var admin, campaign int64
	if err := pool.QueryRow(ctx, `INSERT INTO backoffice_users(email,password_hash,totp_secret,role) VALUES('email-admin@example.test','hash','','ADMIN_SISTEMA') RETURNING id`).Scan(&admin); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO referral_campaigns(name,program_type,program_types,discount_bps,discount_charges,reward_bps,reward_charges,starts_at,ends_at) VALUES('Original both','SELLOS',ARRAY['SELLOS','PUNTOS'],5000,3,2000,12,now()-interval '1 day',now()+interval '90 days') RETURNING id`).Scan(&campaign); err != nil {
		t.Fatal(err)
	}
	created, err := repo.CreateReferralInfluencerWithCode(ctx, "Correo Local", "welcome@example.test", "WELCOME-LOCAL", campaign, admin)
	if err != nil {
		t.Fatal(err)
	}
	// Changing a campaign before delivery must not rewrite the welcome snapshot.
	if _, err = pool.Exec(ctx, `UPDATE referral_campaigns SET name='Changed campaign',discount_bps=1000,reward_bps=1500 WHERE id=$1`, campaign); err != nil {
		t.Fatal(err)
	}
	items, err := repo.ClaimEmails(ctx, 20)
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%v err=%v", items, err)
	}
	item := items[0]
	if item.Kind != "INFLUENCER_WELCOME" || item.To != "welcome@example.test" {
		t.Fatal(item)
	}
	if strings.Contains(string(item.Ciphertext), "WELCOME-LOCAL") {
		t.Fatal("plaintext payload leaked")
	}
	payload, err := repository.DecryptOutboxToken(item, key)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot model.InfluencerWelcomeDetails
	if err = json.Unmarshal([]byte(payload), &snapshot); err != nil || snapshot.DiscountBPS != 5000 || snapshot.RewardBPS != 2000 || snapshot.CampaignName != "Original both" || len(snapshot.ProgramTypes) != 2 {
		t.Fatalf("snapshot=%+v err=%v", snapshot, err)
	}
	if err = repo.ValidateClaimedEmail(ctx, item, payload); err != nil {
		t.Fatal(err)
	}
	wrong := item
	wrong.To = "other@example.test"
	if err = repo.ValidateClaimedEmail(ctx, wrong, payload); err == nil {
		t.Fatal("wrong recipient accepted")
	}
	if _, err = pool.Exec(ctx, `UPDATE referral_codes SET active=false WHERE id=$1`, created.Code.ID); err != nil {
		t.Fatal(err)
	}
	if err = repo.ValidateClaimedEmail(ctx, item, payload); err == nil {
		t.Fatal("disabled code accepted")
	}
	if _, err = pool.Exec(ctx, `UPDATE referral_codes SET active=true WHERE id=$1`, created.Code.ID); err != nil {
		t.Fatal(err)
	}
	if err = repo.MarkEmailFailed(ctx, item.ID, item.LeaseOwner, item.Attempts, errors.New("temporary failure")); err != nil {
		t.Fatal(err)
	}
	var state string
	var attempts int
	if err = pool.QueryRow(ctx, `SELECT estado,intentos FROM email_outbox WHERE id=$1`, item.ID).Scan(&state, &attempts); err != nil || state != "PENDING" || attempts != 1 {
		t.Fatalf("retry=%s/%d err=%v", state, attempts, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE email_outbox SET disponible_at=now() WHERE id=$1`, item.ID); err != nil {
		t.Fatal(err)
	}
	sender := &mailer.MemorySender{}
	worker := mailer.Worker{Repo: repo, Sender: sender, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Interval: time.Hour, PublicAppURL: "https://app.puntazo.test", CipherKey: key}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); worker.Run(runCtx) }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err = pool.QueryRow(ctx, `SELECT estado FROM email_outbox WHERE id=$1`, item.ID).Scan(&state); err != nil {
			t.Fatal(err)
		}
		if state == "SENT" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	messages := sender.Messages()
	if state != "SENT" || len(messages) != 1 || len(messages[0].InlineImages) != 1 || !strings.Contains(messages[0].Text, "50% en los primeros 3 cobros") || !strings.Contains(messages[0].Text, "20% en los primeros 12 cobros") || !strings.Contains(messages[0].Text, "Sellos y puntos") {
		t.Fatalf("state=%s messages=%+v", state, messages)
	}
	var scrubbed bool
	if err = pool.QueryRow(ctx, `SELECT token_ciphertext IS NULL AND token_nonce IS NULL AND token_expires_at IS NULL AND usuario_id IS NULL AND intentos=2 AND sent_at IS NOT NULL FROM email_outbox WHERE id=$1`, item.ID).Scan(&scrubbed); err != nil || !scrubbed {
		t.Fatalf("scrubbed=%t err=%v", scrubbed, err)
	}
	again, err := repo.ClaimEmails(ctx, 20)
	if err != nil || len(again) != 0 {
		t.Fatalf("duplicate claim=%v err=%v", again, err)
	}
}

func TestPostgresInfluencerMailMigrationUpgradeAndRollback(t *testing.T) {
	pool := referralBillingPool(t)
	ctx := t.Context()
	read := func(suffix string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0028_influencer_welcome_emails."+suffix+".sql"))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	if _, err := pool.Exec(ctx, read("down")); err != nil {
		t.Fatal(err)
	}
	var user int64
	if err := pool.QueryRow(ctx, `INSERT INTO usuarios(email,password_hash,nombre,tipo_cuenta) VALUES('old-mail@example.test','hash','Legacy','PERSONAL_MARCA') RETURNING id`).Scan(&user); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO email_outbox(id,usuario_id,tipo,destinatario,asunto,estado,sent_at) VALUES('00000000-0000-4000-8000-000000000027',$1,'RESET_PASSWORD','old-mail@example.test','Legacy reset','SENT',now())`, user); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, read("up")); err != nil {
		t.Fatal(err)
	}
	var retained int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM email_outbox WHERE usuario_id=$1 AND tipo='RESET_PASSWORD' AND estado='SENT'`, user).Scan(&retained); err != nil || retained != 1 {
		t.Fatalf("legacy=%d err=%v", retained, err)
	}
	var admin, campaign int64
	if err := pool.QueryRow(ctx, `INSERT INTO backoffice_users(email,password_hash,totp_secret,role) VALUES('rollback-admin@example.test','hash','','ADMIN_SISTEMA') RETURNING id`).Scan(&admin); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO referral_campaigns(name,program_type,discount_bps,discount_charges,reward_bps,reward_charges,starts_at,ends_at) VALUES('Rollback campaign','PUNTOS',1000,1,500,1,now(),now()+interval '1 day') RETURNING id`).Scan(&campaign); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.New(pool, []byte("01234567890123456789012345678901")).CreateReferralInfluencerWithCode(ctx, "Rollback Person", "rollback@example.test", "ROLLBACK-27", campaign, admin); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, read("down")); err == nil || !strings.Contains(err.Error(), "Cannot downgrade") {
		t.Fatalf("unsafe downgrade=%v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM email_outbox`).Scan(&retained); err != nil || retained != 2 {
		t.Fatalf("evidence=%d err=%v", retained, err)
	}
}
