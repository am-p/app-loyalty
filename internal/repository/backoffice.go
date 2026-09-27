package repository

import (
	"context"
	"crypto/sha256"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type BackofficeUser struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	Role         string `json:"role"`
	PasswordHash string `json:"-"`
	TOTPSecret   string `json:"-"`
}

func (r *Repository) BackofficeUserByEmail(ctx context.Context, email string) (BackofficeUser, error) {
	var u BackofficeUser
	err := r.Pool.QueryRow(ctx, `SELECT id,email::text,role,password_hash,totp_secret FROM backoffice_users WHERE email=$1 AND active`, email).Scan(&u.ID, &u.Email, &u.Role, &u.PasswordHash, &u.TOTPSecret)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

func (r *Repository) CreateBackofficeSession(ctx context.Context, userID int64, token string, expires time.Time, totpStep int64) error {
	digest := sha256.Sum256([]byte(token))
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE backoffice_users SET last_totp_step=$2 WHERE id=$1 AND last_totp_step<$2 AND active`, userID, totpStep)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	if _, err = tx.Exec(ctx, `INSERT INTO backoffice_sessions(token_hash,user_id,expires_at) VALUES($1,$2,$3)`, digest[:], userID, expires); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) BackofficeSession(ctx context.Context, token string) (BackofficeUser, error) {
	var u BackofficeUser
	digest := sha256.Sum256([]byte(token))
	err := r.Pool.QueryRow(ctx, `SELECT u.id,u.email::text,u.role FROM backoffice_sessions s JOIN backoffice_users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.expires_at>now() AND u.active`, digest[:]).Scan(&u.ID, &u.Email, &u.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

func (r *Repository) DeleteBackofficeSession(ctx context.Context, token string) error {
	digest := sha256.Sum256([]byte(token))
	_, err := r.Pool.Exec(ctx, `DELETE FROM backoffice_sessions WHERE token_hash=$1`, digest[:])
	return err
}

func (r *Repository) SettleReferralReward(ctx context.Context, invoice string, userID int64) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE referral_rewards SET status='SETTLED',settled_at=now() WHERE provider_invoice_id=$1 AND source_kind='INFLUENCER' AND status='PENDING'`, invoice)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	if err = auditReferral(ctx, tx, userID, "reward.settle", "invoice", invoice); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
