package repository

import (
	"context"
	"fmt"

	"clientesFrecuentes/internal/identitycode"
	"github.com/jackc/pgx/v5"
)

// The prefix row serializes concurrent allocations; rollback restores its value.
func allocateUserCode(ctx context.Context, tx pgx.Tx, name, lastName string) (string, error) {
	prefix, ok := identitycode.Prefix(name, lastName)
	if !ok {
		return "", ErrInvalidRequest
	}
	var number int64
	err := tx.QueryRow(ctx, `INSERT INTO contadores_codigo_usuario(prefijo,ultimo_numero) VALUES($1,1)
		ON CONFLICT(prefijo) DO UPDATE SET ultimo_numero=contadores_codigo_usuario.ultimo_numero+1
		RETURNING ultimo_numero`, prefix).Scan(&number)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%d", prefix, number), nil
}

func (r *Repository) CustomerIDByCode(ctx context.Context, code string) (int64, error) {
	var id int64
	err := r.Pool.QueryRow(ctx, `SELECT id FROM usuarios WHERE codigo_usuario=$1
		AND tipo_cuenta='CLIENTE_FINAL' AND activo AND deleted_at IS NULL`, code).Scan(&id)
	if err == pgx.ErrNoRows {
		return 0, ErrNotFound
	}
	return id, err
}
