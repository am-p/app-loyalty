package repository

import (
	"context"
	"crypto/hmac"
	"errors"
	"strings"
	"time"

	"clientesFrecuentes/internal/emailcode"
	"clientesFrecuentes/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// enqueueVerificationCode runs under the account lock held by registration or
// resend. Only the initial registration may install an automatic-login proof.
func enqueueVerificationCode(ctx context.Context, tx pgx.Tx, key []byte, userID int64, hash []byte, expires time.Time, message model.EmailMessage) error {
	if _, err := tx.Exec(ctx, `UPDATE tokens_identidad_email SET consumed_at=now() WHERE usuario_id=$1 AND proposito='VERIFY_EMAIL' AND consumed_at IS NULL`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE email_outbox SET estado='FAILED',ultimo_error='superseded',token_ciphertext=NULL,token_nonce=NULL,token_expires_at=NULL,lease_until=NULL,lease_owner=NULL WHERE usuario_id=$1 AND tipo='VERIFY_EMAIL' AND estado IN ('PENDING','SENDING')`, userID); err != nil {
		return err
	}
	var registrationExpiry *time.Time
	if len(message.RegistrationHash) > 0 {
		registrationExpiry = &message.RegistrationExpires
	}
	_, err := tx.Exec(ctx, `INSERT INTO email_verification_codes(usuario_id,code_hash,expires_at,registration_hash,registration_expires_at) VALUES($1,$2,$3,$4,$5)
 ON CONFLICT(usuario_id) DO UPDATE SET code_hash=EXCLUDED.code_hash,expires_at=EXCLUDED.expires_at,attempts=0,sent_at=now(),consumed_at=NULL`, userID, hash, expires, message.RegistrationHash, registrationExpiry)
	if err != nil {
		return err
	}
	id := uuid.NewString()
	code := strings.TrimPrefix(message.Token, "code:")
	linkHash := emailcode.RegistrationHash(emailcode.ActivationToken(key, message.To, code, id))
	if _, err = tx.Exec(ctx, `INSERT INTO tokens_identidad_email(id,usuario_id,proposito,token_hash,expires_at) VALUES($1,$2,'VERIFY_EMAIL',$3,$4)`, uuid.NewString(), userID, linkHash, expires); err != nil {
		return err
	}
	ciphertext, nonce, err := encryptOutboxToken(message.Token, id, key)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO email_outbox(id,usuario_id,tipo,destinatario,asunto,token_ciphertext,token_nonce,token_expires_at) VALUES($1,$2,'VERIFY_EMAIL',$3,$4,$5,$6,$7)`, id, userID, message.To, message.Subject, ciphertext, nonce, expires)
	return err
}

// EnqueueVerificationCode deliberately succeeds for unknown/verified accounts
// and during the cooldown, keeping the public request response generic.
func (r *Repository) EnqueueVerificationCode(ctx context.Context, email, code string, expires time.Time, message model.EmailMessage) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var userID int64
	err = tx.QueryRow(ctx, `SELECT id FROM usuarios WHERE email=$1 AND activo AND deleted_at IS NULL AND email_verified_at IS NULL AND password_hash IS NOT NULL FOR UPDATE`, email).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var previous []byte
	var cooldown bool
	err = tx.QueryRow(ctx, `SELECT code_hash,sent_at>now()-interval '60 seconds' FROM email_verification_codes WHERE usuario_id=$1`, userID).Scan(&previous, &cooldown)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if cooldown {
		return nil
	}
	// Rotation must not accidentally issue the exact same six digits.
	hash := emailcode.Hash(r.OutboxCipherKey, email, code)
	for hmac.Equal(previous, hash) {
		code, err = emailcode.Generate()
		if err != nil {
			return err
		}
		hash = emailcode.Hash(r.OutboxCipherKey, email, code)
	}
	message.Token = "code:" + code
	if err = enqueueVerificationCode(ctx, tx, r.OutboxCipherKey, userID, hash, expires, message); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type VerifiedCodeIdentity struct {
	UserID      int64
	AuthVersion int
}

func (r *Repository) ConfirmVerificationCode(ctx context.Context, email string, hash []byte, registrationToken string, now time.Time) (VerifiedCodeIdentity, bool, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return VerifiedCodeIdentity{}, false, err
	}
	defer tx.Rollback(ctx)
	var userID int64
	var authVersion int
	err = tx.QueryRow(ctx, `SELECT id,auth_version FROM usuarios WHERE email=$1 AND activo AND deleted_at IS NULL AND email_verified_at IS NULL FOR UPDATE`, email).Scan(&userID, &authVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return VerifiedCodeIdentity{}, false, ErrIdentityTokenInvalid
	}
	if err != nil {
		return VerifiedCodeIdentity{}, false, err
	}
	var expected, registrationHash []byte
	var expires time.Time
	var registrationExpires, consumed *time.Time
	var attempts int
	err = tx.QueryRow(ctx, `SELECT code_hash,expires_at,attempts,consumed_at,registration_hash,registration_expires_at FROM email_verification_codes WHERE usuario_id=$1 FOR UPDATE`, userID).Scan(&expected, &expires, &attempts, &consumed, &registrationHash, &registrationExpires)
	if errors.Is(err, pgx.ErrNoRows) {
		return VerifiedCodeIdentity{}, false, ErrIdentityTokenInvalid
	}
	if err != nil {
		return VerifiedCodeIdentity{}, false, err
	}
	if consumed != nil || !expires.After(now) || attempts >= 5 {
		return VerifiedCodeIdentity{}, false, ErrIdentityTokenInvalid
	}
	if !hmac.Equal(hash, expected) {
		if _, err = tx.Exec(ctx, `UPDATE email_verification_codes SET attempts=attempts+1 WHERE usuario_id=$1`, userID); err != nil {
			return VerifiedCodeIdentity{}, false, err
		}
		if attempts == 4 {
			if _, err = tx.Exec(ctx, `UPDATE tokens_identidad_email SET consumed_at=$2 WHERE usuario_id=$1 AND proposito='VERIFY_EMAIL' AND consumed_at IS NULL`, userID, now); err != nil {
				return VerifiedCodeIdentity{}, false, err
			}
		}
		if err = tx.Commit(ctx); err != nil {
			return VerifiedCodeIdentity{}, false, err
		}
		return VerifiedCodeIdentity{}, false, ErrIdentityTokenInvalid
	}
	if _, err = tx.Exec(ctx, `UPDATE usuarios SET email_verified_at=$2 WHERE id=$1`, userID, now); err != nil {
		return VerifiedCodeIdentity{}, false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE email_verification_codes SET consumed_at=$2,registration_hash=NULL,registration_expires_at=NULL WHERE usuario_id=$1`, userID, now); err != nil {
		return VerifiedCodeIdentity{}, false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE tokens_identidad_email SET consumed_at=$2 WHERE usuario_id=$1 AND proposito='VERIFY_EMAIL' AND consumed_at IS NULL`, userID, now); err != nil {
		return VerifiedCodeIdentity{}, false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE email_outbox SET estado='FAILED',ultimo_error='verification completed',token_ciphertext=NULL,token_nonce=NULL,token_expires_at=NULL,lease_until=NULL,lease_owner=NULL WHERE usuario_id=$1 AND tipo='VERIFY_EMAIL' AND estado IN ('PENDING','SENDING')`, userID); err != nil {
		return VerifiedCodeIdentity{}, false, err
	}
	automatic := registrationToken != "" && registrationExpires != nil && registrationExpires.After(now) && hmac.Equal(registrationHash, emailcode.RegistrationHash(registrationToken))
	if err = tx.Commit(ctx); err != nil {
		return VerifiedCodeIdentity{}, false, err
	}
	return VerifiedCodeIdentity{UserID: userID, AuthVersion: authVersion}, automatic, nil
}

func (r *Repository) validateClaimedCode(ctx context.Context, item model.OutboxEmail, code string) error {
	var valid bool
	err := r.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM email_verification_codes v JOIN usuarios u ON u.id=v.usuario_id JOIN email_outbox e ON e.usuario_id=v.usuario_id WHERE e.id=$1 AND e.estado='SENDING' AND e.lease_owner=$2 AND e.tipo='VERIFY_EMAIL' AND v.code_hash=$3 AND v.consumed_at IS NULL AND v.attempts<5 AND v.expires_at>now() AND u.email_verified_at IS NULL AND u.activo AND u.deleted_at IS NULL AND u.email=e.destinatario)`, item.ID, item.LeaseOwner, emailcode.Hash(r.OutboxCipherKey, item.To, code)).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return ErrIdentityTokenInvalid
	}
	return nil
}

// VerificationActivationToken omits links for deliveries queued by older
// versions, which never persisted a corresponding activation fingerprint.
func (r *Repository) VerificationActivationToken(ctx context.Context, item model.OutboxEmail, code string) (string, error) {
	token := emailcode.ActivationToken(r.OutboxCipherKey, item.To, code, item.ID)
	var exists bool
	err := r.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tokens_identidad_email WHERE token_hash=$1 AND proposito='VERIFY_EMAIL' AND consumed_at IS NULL AND expires_at>now())`, emailcode.RegistrationHash(token)).Scan(&exists)
	if err != nil || !exists {
		return "", err
	}
	return token, nil
}
