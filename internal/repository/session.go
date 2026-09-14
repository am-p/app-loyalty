package repository

import (
	"context"
	"errors"
	"time"

	"clientesFrecuentes/internal/model"

	"github.com/jackc/pgx/v5"
)

type RotatedSession struct {
	User     model.User
	AuthTime time.Time
}

func (r *Repository) CreateSession(ctx context.Context, id string, userID int64, refreshHash []byte, expiresAt, authTime time.Time) error {
	_, err := r.Pool.Exec(ctx, `INSERT INTO sesiones_auth(id,usuario_id,refresh_hash,expires_at,family_id,auth_time) VALUES($1,$2,$3,$4,$1,$5)`, id, userID, refreshHash, expiresAt, authTime)
	return err
}

func (r *Repository) RotateSession(ctx context.Context, refreshHash []byte, replacementID string, replacementHash []byte, replacementExpiresAt time.Time) (RotatedSession, error) {
	tx, err := r.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return RotatedSession{}, err
	}
	defer tx.Rollback(ctx)
	var oldID, familyID string
	var revokedAt *time.Time
	var expiresAt, authTime time.Time
	var u model.User
	err = tx.QueryRow(ctx, `SELECT s.id,s.family_id,s.revoked_at,s.expires_at,s.auth_time,u.id,u.email::text,u.nombre,u.tipo_cuenta,u.activo,u.created_at
		FROM sesiones_auth s JOIN usuarios u ON u.id=s.usuario_id
		WHERE s.refresh_hash=$1 AND u.activo AND u.deleted_at IS NULL FOR UPDATE OF s`, refreshHash).
		Scan(&oldID, &familyID, &revokedAt, &expiresAt, &authTime, &u.ID, &u.Email, &u.Name, &u.AccountType, &u.Active, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return RotatedSession{}, ErrNotFound
	}
	if err != nil {
		return RotatedSession{}, err
	}
	if revokedAt != nil {
		if _, err = tx.Exec(ctx, `UPDATE sesiones_auth SET revoked_at=COALESCE(revoked_at,now()) WHERE family_id=$1`, familyID); err != nil {
			return RotatedSession{}, err
		}
		if err = tx.Commit(ctx); err != nil {
			return RotatedSession{}, err
		}
		return RotatedSession{}, ErrSessionReuse
	}
	if !expiresAt.After(time.Now()) {
		return RotatedSession{}, ErrNotFound
	}
	if _, err = tx.Exec(ctx, `INSERT INTO sesiones_auth(id,usuario_id,refresh_hash,expires_at,family_id,auth_time) VALUES($1,$2,$3,$4,$5,$6)`, replacementID, u.ID, replacementHash, replacementExpiresAt, familyID, authTime); err != nil {
		return RotatedSession{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE sesiones_auth SET revoked_at=now(),replaced_by=$2,last_used_at=now() WHERE id=$1`, oldID, replacementID); err != nil {
		return RotatedSession{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return RotatedSession{}, err
	}
	return RotatedSession{User: u, AuthTime: authTime}, nil
}

func (r *Repository) RevokeSession(ctx context.Context, userID int64, sessionID string) error {
	command, err := r.Pool.Exec(ctx, `UPDATE sesiones_auth SET revoked_at=COALESCE(revoked_at,now()) WHERE id=$1 AND usuario_id=$2`, sessionID, userID)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) ActiveSessionAccountType(ctx context.Context, userID int64, sessionID string) (string, error) {
	var accountType string
	err := r.Pool.QueryRow(ctx, `SELECT u.tipo_cuenta FROM sesiones_auth s JOIN usuarios u ON u.id=s.usuario_id
		WHERE s.id=$1 AND s.usuario_id=$2 AND s.revoked_at IS NULL AND s.expires_at>now() AND u.activo AND u.deleted_at IS NULL`, sessionID, userID).Scan(&accountType)
	return accountType, err
}
