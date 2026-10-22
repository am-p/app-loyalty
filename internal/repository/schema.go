package repository

import (
	"clientesFrecuentes/migrations"
	"context"
	"fmt"
)

func (r *Repository) CheckSchema(ctx context.Context, expected string) error {
	wanted, err := migrations.VersionsThrough(expected)
	if err != nil {
		return err
	}
	rows, err := r.Pool.Query(ctx, `SELECT version::text FROM schema_migrations ORDER BY version`)
	if err != nil {
		return err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var got string
		if err = rows.Scan(&got); err != nil {
			return err
		}
		if count >= len(wanted) || got != wanted[count] {
			return fmt.Errorf("schema migration %s differs from source manifest for %s", got, expected)
		}
		count++
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if count != len(wanted) {
		return fmt.Errorf("schema has %d migrations, expected %d for %s", count, len(wanted), expected)
	}
	return nil
}
