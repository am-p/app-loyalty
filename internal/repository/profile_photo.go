package repository

import (
	"context"
	"errors"

	"clientesFrecuentes/internal/model"

	"github.com/jackc/pgx/v5"
)

func (r *Repository) ProfilePhotoReference(ctx context.Context, userID int64) (*string, error) {
	var reference *string
	err := r.Pool.QueryRow(ctx, `SELECT foto_url FROM usuarios WHERE id=$1 AND activo AND deleted_at IS NULL`, userID).Scan(&reference)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return reference, err
}

func (r *Repository) ReplaceProfilePhoto(ctx context.Context, userID int64, expectedVersion int, reference string) (*string, model.CurrentUser, error) {
	tx, err := r.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, model.CurrentUser{}, err
	}
	defer tx.Rollback(ctx)

	var currentVersion int
	var previous *string
	if err = tx.QueryRow(ctx, `SELECT version,foto_url FROM usuarios WHERE id=$1 AND activo AND deleted_at IS NULL FOR UPDATE`, userID).Scan(&currentVersion, &previous); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, model.CurrentUser{}, ErrNotFound
		}
		return nil, model.CurrentUser{}, err
	}
	if currentVersion != expectedVersion {
		return nil, model.CurrentUser{}, ErrPreconditionFailed
	}
	if _, err = tx.Exec(ctx, `UPDATE usuarios SET foto_url=$2,version=version+1 WHERE id=$1`, userID, reference); err != nil {
		return nil, model.CurrentUser{}, err
	}
	if previous != nil && *previous != reference {
		if _, err = tx.Exec(ctx, `INSERT INTO profile_media_deletions(object_key) SELECT substring($1::text from 14) WHERE $1::text LIKE 's3://puntazo/profiles/%' ON CONFLICT DO NOTHING`, *previous); err != nil {
			return nil, model.CurrentUser{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, model.CurrentUser{}, err
	}
	current, err := r.GetCurrentUser(ctx, userID)
	return previous, current, err
}
