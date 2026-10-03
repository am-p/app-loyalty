package repository_test

import (
	"clientesFrecuentes/internal/repository"
	"testing"
	"time"
)

func TestClosedTestAdultAndDeletion(t *testing.T) {
	pool := referralBillingPool(t)
	f := accountDeletionFixture(t, pool, "AUTHORIZED")
	repo := repository.New(pool)
	confirmed, err := repo.AdultConfirmed(t.Context(), f.customer)
	if err != nil || confirmed {
		t.Fatalf("new account confirmed=%t err=%v", confirmed, err)
	}
	if err = repo.ConfirmAdult(t.Context(), f.customer); err != nil {
		t.Fatal(err)
	}
	if err = repo.ConfirmAdult(t.Context(), f.customer); err != nil {
		t.Fatal(err)
	}
	current, err := repo.GetCurrentUser(t.Context(), f.customer)
	if err != nil || !current.AdultConfirmed {
		t.Fatalf("confirmation not persisted: %v", err)
	}
	if _, err = pool.Exec(t.Context(), `UPDATE usuarios SET foto_url='s3://puntazo/profiles/test/avatar.jpg' WHERE id=$1`, f.customer); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.AnonymizeAccount(t.Context(), f.customer, current.User.Version); err != nil {
		t.Fatal(err)
	}
	var cards, ledger, journal, photos, owners int
	err = pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM tarjetas WHERE usuario_id=$1),(SELECT count(*) FROM historial_movimientos WHERE tarjeta_id=$2),(SELECT count(*) FROM account_deletion_journal WHERE user_id=$1),(SELECT count(*) FROM profile_media_deletions WHERE object_key='profiles/test/avatar.jpg'),(SELECT count(*) FROM usuarios WHERE id=$3 AND activo)`, f.customer, f.card, f.owner).Scan(&cards, &ledger, &journal, &photos, &owners)
	if err != nil || cards != 0 || ledger != 0 || journal != 1 || photos != 1 || owners != 1 {
		t.Fatalf("deletion cards=%d ledger=%d journal=%d photos=%d owners=%d err=%v", cards, ledger, journal, photos, owners, err)
	}
}

func TestClosedTestPreviewRetention(t *testing.T) {
	pool := referralBillingPool(t)
	f := accountDeletionFixture(t, pool, "")
	now := time.Now().UTC()
	if _, err := pool.Exec(t.Context(), `UPDATE previews_movimiento SET expires_at=$1 WHERE marca_id=$2`, now.Add(-8*24*time.Hour), f.brand); err != nil {
		t.Fatal(err)
	}
	stats, err := repository.New(pool).ApplyRetention(t.Context(), repository.RetentionPolicy{Now: now, BatchSize: 100, PreviewRetention: 7 * 24 * time.Hour, IdempotencyRetention: 30 * 24 * time.Hour, SessionRetention: 30 * 24 * time.Hour, IdentityRetention: 7 * 24 * time.Hour, OutboxRedactAfter: 7 * 24 * time.Hour, OutboxRetention: 30 * 24 * time.Hour})
	if err != nil || stats.PreviewsDeleted != 1 {
		t.Fatalf("preview retention: %+v err=%v", stats, err)
	}
}

func TestClosedTestDeletionPreservesOtherCustomerBalance(t *testing.T) {
	pool := referralBillingPool(t)
	f := accountDeletionFixture(t, pool, "")
	repo := repository.New(pool)
	if _, err := repo.AnonymizeAccount(t.Context(), f.owner, 1); err != nil {
		t.Fatal(err)
	}
	var balance, movements int
	var attributed bool
	err := pool.QueryRow(t.Context(), `SELECT saldo_sellos,(SELECT count(*) FROM historial_movimientos WHERE tarjeta_id=$1),EXISTS(SELECT 1 FROM historial_movimientos WHERE tarjeta_id=$1 AND usuario_operador_id IS NOT NULL) FROM tarjetas WHERE id=$1`, f.card).Scan(&balance, &movements, &attributed)
	if err != nil || balance != 1 || movements != 1 || attributed {
		t.Fatalf("other customer balance=%d movements=%d attributed=%t err=%v", balance, movements, attributed, err)
	}
}
