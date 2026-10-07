package repository

import (
	"clientesFrecuentes/internal/model"
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"time"
)

// RenewAuthentication serializes identity changes with reset, refresh and login.
// The proof snapshot is checked again after taking the account lock.
func (r *Repository) RenewAuthentication(ctx context.Context, proof AuthUser, expectedVersion int, oldSession, googleID, googleEmail string, link bool, id string, hash []byte, expires, now time.Time) (model.User, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return model.User{}, err
	}
	defer tx.Rollback(ctx)
	var version int
	var password, subject *string
	var email string
	err = tx.QueryRow(ctx, `SELECT auth_version,password_hash,google_id,email::text FROM usuarios WHERE id=$1 AND activo AND deleted_at IS NULL FOR UPDATE`, proof.User.ID).Scan(&version, &password, &subject, &email)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.User{}, ErrNotFound
	}
	if err != nil {
		return model.User{}, err
	}
	if version != expectedVersion || version != proof.User.AuthVersion || !equalNullable(password, proof.PasswordHash) || !equalNullable(subject, proof.GoogleID) {
		return model.User{}, ErrNotFound
	}
	var valid bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sesiones_auth WHERE id=$1 AND usuario_id=$2 AND revoked_at IS NULL AND expires_at>now())`, oldSession, proof.User.ID).Scan(&valid); err != nil {
		return model.User{}, err
	}
	if !valid {
		return model.User{}, ErrNotFound
	}
	if link {
		if email != googleEmail || (subject != nil && *subject != googleID) {
			return model.User{}, ErrGoogleIdentityConflict
		}
		_, err = tx.Exec(ctx, `UPDATE usuarios SET google_id=$2,email_verified_at=COALESCE(email_verified_at,$3) WHERE id=$1`, proof.User.ID, googleID, now)
		if IsUniqueViolation(err) {
			return model.User{}, ErrGoogleIdentityConflict
		}
		if err != nil {
			return model.User{}, err
		}
	}
	if link {
		if _, err = tx.Exec(ctx, `UPDATE tokens_identidad_email SET consumed_at=COALESCE(consumed_at,$2) WHERE usuario_id=$1 AND consumed_at IS NULL`, proof.User.ID, now); err != nil {
			return model.User{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE email_outbox SET estado='FAILED',ultimo_error='identity method changed',token_ciphertext=NULL,token_nonce=NULL,token_expires_at=NULL,lease_until=NULL,lease_owner=NULL WHERE usuario_id=$1 AND tipo IN ('VERIFY_EMAIL','RESET_PASSWORD','CHANGE_EMAIL') AND estado IN ('PENDING','SENDING')`, proof.User.ID); err != nil {
			return model.User{}, err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE usuarios SET auth_version=auth_version+1 WHERE id=$1`, proof.User.ID); err != nil {
		return model.User{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE sesiones_auth SET revoked_at=COALESCE(revoked_at,$2) WHERE usuario_id=$1`, proof.User.ID, now); err != nil {
		return model.User{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO sesiones_auth(id,usuario_id,refresh_hash,expires_at,family_id,auth_time) VALUES($1,$2,$3,$4,$1,$5)`, id, proof.User.ID, hash, expires, now); err != nil {
		return model.User{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return model.User{}, err
	}
	u := proof.User
	u.AuthVersion++
	if link {
		u.EmailVerified = true
	}
	return u, nil
}
func equalNullable(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}
