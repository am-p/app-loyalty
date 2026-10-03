package repository_test

import (
	"bytes"
	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/push"
	"clientesFrecuentes/internal/repository"
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"testing"
)

func TestWebPushCommitReplayRetryAndOwnership(t *testing.T) {
	ctx := context.Background()
	pool := reviewsDB(t)
	applyReviews(t, pool)
	f := fixtureReviews(t, pool)
	repo := repository.New(pool, bytes.Repeat([]byte{42}, 32))
	public, private, err := repo.WebPushKeys(ctx, push.GenerateWebPushKeys)
	if err != nil {
		t.Fatal(err)
	}
	public2, private2, err := repo.WebPushKeys(ctx, func() (string, string, error) { t.Fatal("regenerated keys"); return "", "", nil })
	if err != nil || public != public2 || private != private2 {
		t.Fatal("unstable keys", err)
	}
	endpoint := "https://fcm.googleapis.com/fcm/send/test-device"
	payload := `{"endpoint":"` + endpoint + `","keys":{"auth":"test"}}`
	if err = repo.SaveWebPushSubscription(ctx, f.customer, endpoint, payload); err != nil {
		t.Fatal(err)
	}
	var stored []byte
	if err = pool.QueryRow(ctx, `SELECT ciphertext FROM web_push_subscriptions`).Scan(&stored); err != nil || bytes.Contains(stored, []byte(endpoint)) {
		t.Fatal("plaintext subscription", err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO accesos_demo(marca_id,tipo,precio_minor,moneda,cobro_automatico) VALUES($1,'SELLOS_FREE_TRIAL',0,'ARS',false)`, f.brand); err != nil {
		t.Fatal(err)
	}
	var hash []byte
	if err = pool.QueryRow(ctx, `SELECT qr_hash FROM usuarios WHERE id=$1`, f.customer).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	amount := int64(3)
	fingerprint := []byte("confirmed-movement")
	preview, err := repo.CreatePreview(ctx, f.owner, model.MovementPreviewRequest{Operation: "ACUMULACION", BranchID: f.branch, PointsAmount: &amount}, hash, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	input := repository.ConfirmInput{ActorID: f.owner, Key: uuid.NewString(), Fingerprint: fingerprint, PreviewID: preview.ID, QRHash: hash, BranchID: f.branch, Operation: "ACUMULACION"}
	if _, err = repo.ConfirmMovement(ctx, input, func(model.Movement) ([]byte, error) { return nil, errors.New("rollback") }); err == nil {
		t.Fatal("rollback accepted")
	}
	jobs, err := repo.ClaimWebPushJobs(ctx)
	if err != nil || len(jobs) != 0 {
		t.Fatal("push before commit", err)
	}
	build := func(m model.Movement) ([]byte, error) { return json.Marshal(m) }
	if _, err = repo.ConfirmMovement(ctx, input, build); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.ConfirmMovement(ctx, input, build); err != nil {
		t.Fatal("replay", err)
	}
	jobs, err = repo.ClaimWebPushJobs(ctx)
	if err != nil || len(jobs) != 1 || jobs[0].Payload != payload || jobs[0].CardID != f.card {
		t.Fatalf("jobs %+v err %v", jobs, err)
	}
	if !repo.WebPushJobActive(ctx, jobs[0]) {
		t.Fatal("active job rejected")
	}
	if err = repo.FinishWebPushJob(ctx, jobs[0], false, false); err != nil {
		t.Fatal(err)
	}
	var state string
	if err = pool.QueryRow(ctx, `SELECT estado FROM web_push_notifications`).Scan(&state); err != nil || state != "PENDING" {
		t.Fatal("retry not queued", state, err)
	}
	if err = repo.SaveWebPushSubscription(ctx, f.other, endpoint, payload); err != nil {
		t.Fatal(err)
	}
	if repo.WebPushJobActive(ctx, jobs[0]) {
		t.Fatal("notification crossed account")
	}
	if err = repo.FinishWebPushJob(ctx, jobs[0], false, true); err != nil {
		t.Fatal(err)
	}
	if err = repo.DeleteWebPushSubscription(ctx, f.customer, endpoint); err != nil {
		t.Fatal(err)
	}
	var owner int64
	if err = pool.QueryRow(ctx, `SELECT usuario_id FROM web_push_subscriptions`).Scan(&owner); err != nil || owner != f.other {
		t.Fatal("old account removed new subscription", err)
	}
	if err = repo.DeleteWebPushSubscription(ctx, f.other, endpoint); err != nil {
		t.Fatal(err)
	}
}
