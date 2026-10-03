package repository

import "context"

const AdultPolicyVersion = "18-plus-2026-10"

func (r *Repository) AdultConfirmed(ctx context.Context, id int64) (bool, error) {
	var confirmed bool
	err := r.Pool.QueryRow(ctx, `SELECT adult_confirmed_at IS NOT NULL AND adult_policy_version=$2 FROM usuarios WHERE id=$1 AND activo AND deleted_at IS NULL`, id, AdultPolicyVersion).Scan(&confirmed)
	return confirmed, err
}

func (r *Repository) ConfirmAdult(ctx context.Context, id int64) error {
	tag, err := r.Pool.Exec(ctx, `UPDATE usuarios SET adult_confirmed_at=COALESCE(adult_confirmed_at,now()),adult_policy_version=$2 WHERE id=$1 AND activo AND deleted_at IS NULL`, id, AdultPolicyVersion)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}
