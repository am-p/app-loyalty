package repository

import (
	"context"
	"fmt"
)

func (r *Repository) CheckSchema(ctx context.Context, expected string) error {
	var got string
	err := r.Pool.QueryRow(ctx, `SELECT version FROM schema_migrations ORDER BY applied_at DESC LIMIT 1`).Scan(&got)
	if err != nil {
		return err
	}
	if got != expected {
		return fmt.Errorf("schema version %s, expected %s", got, expected)
	}
	return nil
}
