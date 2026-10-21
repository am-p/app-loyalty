package repository

import (
	"clientesFrecuentes/internal/model"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/jackc/pgx/v5"
	"time"
)

// Browser endpoints, auth secrets and VAPID private keys never leave storage in plaintext.
func (r *Repository) WebPushKeys(ctx context.Context, generate func() (string, string, error)) (string, string, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(735035)`); err != nil {
		return "", "", err
	}
	var public string
	var encrypted, nonce []byte
	err = tx.QueryRow(ctx, `SELECT public_key,private_ciphertext,nonce FROM web_push_keys WHERE id=true`).Scan(&public, &encrypted, &nonce)
	if errors.Is(err, pgx.ErrNoRows) {
		var private string
		private, public, err = generate()
		if err != nil {
			return "", "", err
		}
		encrypted, nonce, err = encryptOutboxToken(private, "web-push-vapid", r.OutboxCipherKey)
		if err != nil {
			return "", "", err
		}
		_, err = tx.Exec(ctx, `INSERT INTO web_push_keys(id,public_key,private_ciphertext,nonce) VALUES(true,$1,$2,$3)`, public, encrypted, nonce)
	}
	if err != nil {
		return "", "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", "", err
	}
	private, err := DecryptOutboxToken(model.OutboxEmail{ID: "web-push-vapid", Ciphertext: encrypted, Nonce: nonce}, r.OutboxCipherKey)
	return public, private, err
}
func endpointHash(endpoint string) string {
	digest := sha256.Sum256([]byte(endpoint))
	return hex.EncodeToString(digest[:])
}
func (r *Repository) SaveWebPushSubscription(ctx context.Context, customerID int64, endpoint, payload string) error {
	hash := endpointHash(endpoint)
	encrypted, nonce, err := encryptOutboxToken(payload, "web-push-subscription:"+hash, r.OutboxCipherKey)
	if err != nil {
		return err
	}
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, hash); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, customerID); err != nil {
		return err
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM web_push_subscriptions WHERE usuario_id=$1 AND endpoint_hash<>$2`, customerID, hash).Scan(&count); err != nil {
		return err
	}
	if count >= 16 {
		return errors.New("web push device limit reached")
	}
	// Account changes remove queued notifications before transferring the subscription.
	if _, err = tx.Exec(ctx, `DELETE FROM web_push_notifications WHERE subscription_id IN (SELECT id FROM web_push_subscriptions WHERE endpoint_hash=$1 AND usuario_id<>$2)`, hash, customerID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO web_push_subscriptions(usuario_id,endpoint_hash,ciphertext,nonce) VALUES($1,$2,$3,$4)
 ON CONFLICT(endpoint_hash) DO UPDATE SET usuario_id=EXCLUDED.usuario_id,ciphertext=EXCLUDED.ciphertext,nonce=EXCLUDED.nonce,updated_at=now()`, customerID, hash, encrypted, nonce)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (r *Repository) DeleteWebPushSubscription(ctx context.Context, customerID int64, endpoint string) error {
	_, err := r.Pool.Exec(ctx, `DELETE FROM web_push_subscriptions WHERE usuario_id=$1 AND endpoint_hash=$2`, customerID, endpointHash(endpoint))
	return err
}

type WebPushJob struct {
	ID, SubscriptionID, CustomerID, CardID int64
	Attempts                               int
	OperationID, Payload                   string
	SubscriptionUpdatedAt                  time.Time
}

func (r *Repository) ClaimWebPushJobs(ctx context.Context) ([]WebPushJob, error) {
	rows, err := r.Pool.Query(ctx, `WITH next AS (SELECT n.id FROM web_push_notifications n
 JOIN web_push_subscriptions s ON s.id=n.subscription_id JOIN usuarios u ON u.id=s.usuario_id AND u.activo AND u.deleted_at IS NULL
 WHERE (n.estado='PENDING' AND n.disponible_at<=now()) OR (n.estado='SENDING' AND n.lease_until<now())
 ORDER BY n.id LIMIT 5 FOR UPDATE OF n SKIP LOCKED),claimed AS (
 UPDATE web_push_notifications n SET estado='SENDING',intentos=intentos+1,lease_until=now()+interval '90 seconds',updated_at=now()
 FROM next WHERE n.id=next.id RETURNING n.*)
 SELECT n.id,s.id,s.usuario_id,n.card_id,n.intentos,n.operation_id::text,s.endpoint_hash,s.ciphertext,s.nonce,s.updated_at
 FROM claimed n JOIN web_push_subscriptions s ON s.id=n.subscription_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []WebPushJob{}
	for rows.Next() {
		var job WebPushJob
		var hash string
		var encrypted, nonce []byte
		if err = rows.Scan(&job.ID, &job.SubscriptionID, &job.CustomerID, &job.CardID, &job.Attempts, &job.OperationID, &hash, &encrypted, &nonce, &job.SubscriptionUpdatedAt); err != nil {
			return nil, err
		}
		job.Payload, err = DecryptOutboxToken(model.OutboxEmail{ID: "web-push-subscription:" + hash, Ciphertext: encrypted, Nonce: nonce}, r.OutboxCipherKey)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}
func (r *Repository) FinishWebPushJob(ctx context.Context, job WebPushJob, sent, expired bool) error {
	if expired {
		_, err := r.Pool.Exec(ctx, `DELETE FROM web_push_subscriptions WHERE id=$1 AND usuario_id=$2 AND updated_at=$3`, job.SubscriptionID, job.CustomerID, job.SubscriptionUpdatedAt)
		return err
	}
	state := "PENDING"
	if sent {
		state = "SENT"
	} else if job.Attempts >= 5 {
		state = "FAILED"
	}
	_, err := r.Pool.Exec(ctx, `UPDATE web_push_notifications SET estado=$2,lease_until=NULL,disponible_at=now()+interval '10 seconds' * power(2,LEAST(intentos,5)),updated_at=now() WHERE id=$1 AND intentos=$3 AND estado='SENDING'`, job.ID, state, job.Attempts)
	return err
}
func (r *Repository) WebPushJobActive(ctx context.Context, job WebPushJob) bool {
	var exists bool
	err := r.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM web_push_notifications n JOIN web_push_subscriptions s ON s.id=n.subscription_id JOIN usuarios u ON u.id=s.usuario_id WHERE n.id=$1 AND n.estado='SENDING' AND n.intentos=$2 AND s.usuario_id=$3 AND u.activo AND u.deleted_at IS NULL)`, job.ID, job.Attempts, job.CustomerID).Scan(&exists)
	return err == nil && exists
}
