package repository_test

import (
	"clientesFrecuentes/internal/config"
	"clientesFrecuentes/internal/mailer"
	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/repository"
	"clientesFrecuentes/internal/service"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPostgresSubscriptionConfirmationAtomicDeduplicatedAndPrivate(t *testing.T) {
	pool := referralBillingPool(t)
	ctx := t.Context()
	f := accountDeletionFixture(t, pool, "PENDING")
	key := []byte("01234567890123456789012345678901")
	repo := repository.New(pool, key)
	next := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	provider := model.BillingSubscriptionResult{ID: f.provider.id, ExternalReference: f.provider.external, Status: "authorized", NextPaymentDate: &next}
	if err := repository.New(pool, nil).RecordSubscriptionWebhookWithConfirmation(ctx, "atomic-failure", "subscription_preapproval", provider); !errors.Is(err, repository.ErrEmailUnavailable) {
		t.Fatalf("missing key: %v", err)
	}
	var state string
	var n int
	if err := pool.QueryRow(ctx, `SELECT estado FROM suscripciones_marca WHERE marca_id=$1`, f.brand).Scan(&state); err != nil || state != "PENDING" {
		t.Fatalf("rolled back=%s %v", state, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM eventos_mercado_pago WHERE notification_id='atomic-failure'`).Scan(&n); err != nil || n != 0 {
		t.Fatal("event escaped rollback")
	}
	// Concurrent distinct events and repeated IDs must queue one confirmation.
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for _, id := range []string{"subscription-authorized", "subscription-authorized", "subscription-second", "subscription-third"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			errs <- repo.RecordSubscriptionWebhookWithConfirmation(ctx, id, "subscription_preapproval", provider)
		}(id)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.RequestSubscriptionConfirmation(ctx, f.owner, f.brand); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM email_outbox WHERE tipo='SUBSCRIPTION_CONFIRMATION'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("outbox count=%d %v", n, err)
	}
	if _, err := repo.SubscriptionBrandForOwner(ctx, f.customer, f.provider.id); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("foreign reference leaked: %v", err)
	}
	if err := repo.RequestSubscriptionConfirmation(ctx, f.customer, f.brand); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("foreign request accepted: %v", err)
	}
	svc := service.New(repo, nil, config.Config{MercadoPagoBranchPrice: 15000, MailProvider: "capture"})
	result, err := svc.SubscriptionResult(ctx, f.owner, f.provider.id)
	if err != nil || result.Status != "AUTHORIZED" {
		t.Fatalf("server result=%+v %v", result, err)
	}
	if _, err = svc.SubscriptionResult(ctx, f.owner, "abc?status=authorized"); !errors.Is(err, service.ErrInvalidRequest) {
		t.Fatalf("malformed reference=%v", err)
	}
	items, err := repo.ClaimEmails(ctx, 20)
	if err != nil || len(items) != 1 {
		t.Fatalf("claim=%d %v", len(items), err)
	}
	item := items[0]
	payload, err := repository.DecryptOutboxToken(item, key)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot model.SubscriptionConfirmationDetails
	if err = json.Unmarshal([]byte(payload), &snapshot); err != nil || snapshot.ProviderID != f.provider.id || snapshot.MonthlyAmountMinor != 15000 || snapshot.NextPaymentDate == nil {
		t.Fatalf("snapshot=%+v %v", snapshot, err)
	}
	if strings.Contains(string(item.Ciphertext), f.provider.id) {
		t.Fatal("snapshot not encrypted")
	}
	if err = repo.ValidateClaimedEmail(ctx, item, payload); err != nil {
		t.Fatal(err)
	}
	wrong := item
	wrong.To = "other@example.test"
	if err = repo.ValidateClaimedEmail(ctx, wrong, payload); err == nil {
		t.Fatal("wrong recipient accepted")
	}
	if _, err = pool.Exec(ctx, `UPDATE suscripciones_marca SET estado='CANCELLED' WHERE marca_id=$1`, f.brand); err != nil {
		t.Fatal(err)
	}
	if err = repo.ValidateClaimedEmail(ctx, item, payload); err == nil {
		t.Fatal("canceled subscription accepted")
	}
	if _, err = pool.Exec(ctx, `UPDATE suscripciones_marca SET estado='AUTHORIZED' WHERE marca_id=$1`, f.brand); err != nil {
		t.Fatal(err)
	}
	if err = repo.MarkEmailFailed(ctx, item.ID, item.LeaseOwner, item.Attempts, errors.New("temporary SMTP failure")); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE email_outbox SET disponible_at=now() WHERE id=$1`, item.ID); err != nil {
		t.Fatal(err)
	}
	sender := &mailer.MemorySender{}
	worker := mailer.Worker{Repo: repo, Sender: sender, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Interval: time.Hour, PublicAppURL: "https://testing.puntazo.test", CipherKey: key}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { defer close(done); worker.Run(runCtx) }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err = pool.QueryRow(ctx, `SELECT estado FROM email_outbox WHERE id=$1`, item.ID).Scan(&state); err != nil {
			cancel()
			<-done
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
	if state != "SENT" || len(messages) != 1 || len(messages[0].InlineImages) != 1 || !strings.Contains(messages[0].Text, "No es un comprobante de una cuota pagada") {
		t.Fatalf("delivery=%s messages=%d", state, len(messages))
	}
	var scrubbed bool
	if err = pool.QueryRow(ctx, `SELECT token_ciphertext IS NULL AND token_nonce IS NULL AND token_expires_at IS NULL AND sent_at IS NOT NULL FROM email_outbox WHERE id=$1`, item.ID).Scan(&scrubbed); err != nil || !scrubbed {
		t.Fatal("payload not scrubbed")
	}
	if err = repo.RequestSubscriptionConfirmation(ctx, f.owner, f.brand); err != nil {
		t.Fatal(err)
	}
	if items, err = repo.ClaimEmails(ctx, 20); err != nil || len(items) != 0 {
		t.Fatal("confirmation queued twice after SENT")
	}
}

func TestPostgresSubscriptionMailMigrationUpgradeRollbackAndPending(t *testing.T) {
	pool := referralBillingPool(t)
	ctx := t.Context()
	f := accountDeletionFixture(t, pool, "PENDING")
	repo := repository.New(pool, []byte("01234567890123456789012345678901"))
	if err := repo.RequestSubscriptionConfirmation(ctx, f.owner, f.brand); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("pending accepted: %v", err)
	}
	read := func(suffix string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0029_subscription_confirmation_emails."+suffix+".sql"))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	if _, err := pool.Exec(ctx, read("down")); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO email_outbox(id,usuario_id,tipo,destinatario,asunto,estado,sent_at) VALUES('00000000-0000-4000-8000-000000000029',$1,'RESET_PASSWORD','old@example.test','Legacy','SENT',now())`, f.owner); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, read("up")); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE suscripciones_marca SET estado='AUTHORIZED' WHERE marca_id=$1`, f.brand); err != nil {
		t.Fatal(err)
	}
	if err := repo.RequestSubscriptionConfirmation(ctx, f.owner, f.brand); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, read("down")); err != nil {
		t.Fatal(err)
	}
	var retained int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM email_outbox WHERE tipo='RESET_PASSWORD'`).Scan(&retained); err != nil || retained != 1 {
		t.Fatal("legacy email lost")
	}
	if _, err := pool.Exec(ctx, read("up")); err != nil {
		t.Fatal(err)
	}
	if err := repo.RequestSubscriptionConfirmation(ctx, f.owner, f.brand); err != nil {
		t.Fatal(err)
	}
}
