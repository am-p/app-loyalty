package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"clientesFrecuentes/internal/model"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) EnqueueEmailChange(ctx context.Context, userID int64, version int, target string, tokenHash []byte, expiresAt time.Time, message model.EmailMessage) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var current string
	var currentVersion int
	if err = tx.QueryRow(ctx, `SELECT email::text,version FROM usuarios WHERE id=$1 AND activo AND deleted_at IS NULL FOR UPDATE`, userID).Scan(&current, &currentVersion); err != nil {
		return err
	}
	if currentVersion != version {
		return ErrPreconditionFailed
	}
	if strings.EqualFold(current, target) {
		return ErrInvalidRequest
	}
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM usuarios WHERE email=$1 AND id<>$2)`, target, userID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return ErrEmailExists
	}
	if _, err = tx.Exec(ctx, `UPDATE email_outbox SET estado='FAILED',ultimo_error='superseded',token_ciphertext=NULL,token_nonce=NULL,token_expires_at=NULL WHERE usuario_id=$1 AND tipo='CHANGE_EMAIL' AND estado='PENDING'`, userID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE tokens_identidad_email SET consumed_at=now() WHERE usuario_id=$1 AND proposito='CHANGE_EMAIL' AND consumed_at IS NULL`, userID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO tokens_identidad_email(id,usuario_id,proposito,token_hash,expires_at,pending_email,account_version) VALUES($1,$2,'CHANGE_EMAIL',$3,$4,$5,$6)`, uuid.New(), userID, tokenHash, expiresAt, target, version); err != nil {
		return err
	}
	outboxID := uuid.New()
	ciphertext, nonce, err := encryptOutboxToken(message.Token, outboxID.String(), r.OutboxCipherKey)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO email_outbox(id,usuario_id,tipo,destinatario,asunto,token_ciphertext,token_nonce,token_expires_at) VALUES($1,$2,'CHANGE_EMAIL',$3,$4,$5,$6,$7)`, outboxID, userID, target, message.Subject, ciphertext, nonce, expiresAt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) ConfirmEmailChange(ctx context.Context, tokenHash []byte, now time.Time) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var tokenID uuid.UUID
	var userID int64
	var target string
	var version int
	err = tx.QueryRow(ctx, `SELECT id,usuario_id,pending_email::text,account_version FROM tokens_identidad_email WHERE token_hash=$1 AND proposito='CHANGE_EMAIL' AND consumed_at IS NULL AND expires_at>$2 FOR UPDATE`, tokenHash, now).Scan(&tokenID, &userID, &target, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrIdentityTokenInvalid
	}
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE usuarios SET email=$2,email_verified_at=$3,auth_version=auth_version+1,version=version+1 WHERE id=$1 AND version=$4 AND activo AND deleted_at IS NULL`, userID, target, now, version)
	if err != nil {
		return normalize(err)
	}
	if tag.RowsAffected() != 1 {
		return ErrIdentityTokenInvalid
	}
	if _, err = tx.Exec(ctx, `UPDATE sesiones_auth SET revoked_at=COALESCE(revoked_at,$2) WHERE usuario_id=$1`, userID, now); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE tokens_identidad_email SET consumed_at=$2 WHERE usuario_id=$1 AND proposito='CHANGE_EMAIL' AND consumed_at IS NULL`, userID, now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
