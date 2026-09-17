package repository

import (
	"context"
	"fmt"
	"strconv"
)

func (r *Repository) CheckSchema(ctx context.Context, expected string) error {
	var got string
	var applied int
	err := r.Pool.QueryRow(ctx, `SELECT COALESCE(max(version),''),count(*) FROM schema_migrations`).Scan(&got, &applied)
	if err != nil {
		return err
	}
	expectedCount, err := strconv.Atoi(expected)
	if err != nil || got != expected || applied != expectedCount {
		return fmt.Errorf("schema version %s with %d migrations, expected %s with %d", got, applied, expected, expectedCount)
	}
	return nil
}
