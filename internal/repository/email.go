package repository

import (
	"context"
	"errors"
	"time"

	"clientesFrecuentes/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrIdentityTokenInvalid = errors.New("identity token invalid")

func enqueueIdentityEmail(ctx context.Context, tx pgx.Tx, userID int64, tokenHash []byte, expiresAt time.Time, message model.EmailMessage) error {
	if _, err := tx.Exec(ctx, `UPDATE email_outbox SET estado='FAILED',ultimo_error='superseded',lease_until=NULL WHERE usuario_id=$1 AND tipo=$2 AND estado IN ('PENDING','SENDING')`, userID, message.Kind); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE tokens_identidad_email SET consumed_at=now() WHERE usuario_id=$1 AND proposito=$2 AND consumed_at IS NULL`, userID, message.Kind); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO tokens_identidad_email(id,usuario_id,proposito,token_hash,expires_at) VALUES($1,$2,$3,$4,$5)`, uuid.New(), userID, message.Kind, tokenHash, expiresAt); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO email_outbox(id,usuario_id,tipo,destinatario,asunto,cuerpo_texto,cuerpo_html) VALUES($1,$2,$3,$4,$5,$6,$7)`, uuid.New(), userID, message.Kind, message.To, message.Subject, message.Text, message.HTML)
	return err
}

func (r *Repository) EnqueueVerification(ctx context.Context, email string, tokenHash []byte, expiresAt time.Time, message model.EmailMessage) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var userID int64
	err = tx.QueryRow(ctx, `SELECT id FROM usuarios WHERE email=$1 AND activo AND deleted_at IS NULL AND email_verified_at IS NULL FOR UPDATE`, email).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if err = enqueueIdentityEmail(ctx, tx, userID, tokenHash, expiresAt, message); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) EnqueuePasswordReset(ctx context.Context, email string, tokenHash []byte, expiresAt time.Time, message model.EmailMessage) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var userID int64
	err = tx.QueryRow(ctx, `SELECT id FROM usuarios WHERE email=$1 AND activo AND deleted_at IS NULL AND email_verified_at IS NOT NULL AND password_hash IS NOT NULL FOR UPDATE`, email).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if err = enqueueIdentityEmail(ctx, tx, userID, tokenHash, expiresAt, message); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) VerifyEmail(ctx context.Context, tokenHash []byte, now time.Time) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var tokenID uuid.UUID
	var userID int64
	err = tx.QueryRow(ctx, `SELECT id,usuario_id FROM tokens_identidad_email WHERE token_hash=$1 AND proposito='VERIFY_EMAIL' AND consumed_at IS NULL AND expires_at>$2 FOR UPDATE`, tokenHash, now).Scan(&tokenID, &userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrIdentityTokenInvalid
	}
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE usuarios SET email_verified_at=COALESCE(email_verified_at,$2) WHERE id=$1 AND activo AND deleted_at IS NULL`, userID, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrIdentityTokenInvalid
	}
	if _, err = tx.Exec(ctx, `UPDATE tokens_identidad_email SET consumed_at=$2 WHERE id=$1`, tokenID, now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) ResetPassword(ctx context.Context, tokenHash []byte, passwordHash string, now time.Time) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var tokenID uuid.UUID
	var userID int64
	err = tx.QueryRow(ctx, `SELECT id,usuario_id FROM tokens_identidad_email WHERE token_hash=$1 AND proposito='RESET_PASSWORD' AND consumed_at IS NULL AND expires_at>$2 FOR UPDATE`, tokenHash, now).Scan(&tokenID, &userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrIdentityTokenInvalid
	}
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE usuarios SET password_hash=$2,auth_version=auth_version+1 WHERE id=$1 AND activo AND deleted_at IS NULL`, userID, passwordHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrIdentityTokenInvalid
	}
	if _, err = tx.Exec(ctx, `UPDATE tokens_identidad_email SET consumed_at=$2 WHERE usuario_id=$1 AND proposito='RESET_PASSWORD' AND consumed_at IS NULL`, userID, now); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE sesiones_auth SET revoked_at=COALESCE(revoked_at,$2) WHERE usuario_id=$1`, userID, now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) ClaimEmails(ctx context.Context, limit int) ([]model.OutboxEmail, error) {
	rows, err := r.Pool.Query(ctx, `WITH candidates AS (SELECT id FROM email_outbox WHERE (estado='PENDING' AND disponible_at<=now()) OR (estado='SENDING' AND lease_until<now()) ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT $1), claimed AS (UPDATE email_outbox e SET estado='SENDING',intentos=e.intentos+1,lease_until=now()+interval '2 minutes' FROM candidates c WHERE e.id=c.id RETURNING e.id::text,e.destinatario::text,e.asunto,e.cuerpo_texto,e.cuerpo_html,e.intentos) SELECT * FROM claimed`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.OutboxEmail, 0)
	for rows.Next() {
		var item model.OutboxEmail
		if err = rows.Scan(&item.ID, &item.To, &item.Subject, &item.Text, &item.HTML, &item.Attempts); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
func (r *Repository) MarkEmailSent(ctx context.Context, id string) error {
	_, err := r.Pool.Exec(ctx, `UPDATE email_outbox SET estado='SENT',sent_at=now(),lease_until=NULL,ultimo_error=NULL WHERE id=$1`, id)
	return err
}
func (r *Repository) MarkEmailFailed(ctx context.Context, id string, attempts int, sendErr error) error {
	delay := time.Duration(1<<min(attempts, 8)) * time.Minute
	state := "PENDING"
	if attempts >= 5 {
		state = "FAILED"
	}
	message := sendErr.Error()
	if len(message) > 1000 {
		message = message[:1000]
	}
	_, err := r.Pool.Exec(ctx, `UPDATE email_outbox SET estado=$2,disponible_at=$3,lease_until=NULL,ultimo_error=$4 WHERE id=$1`, id, state, r.Now().Add(delay), message)
	return err
}
