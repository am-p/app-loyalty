package repository

import (
	"context"

	"clientesFrecuentes/internal/model"

	"github.com/jackc/pgx/v5"
)

func (r *Repository) ExportAccount(ctx context.Context, id int64) (model.AccountExport, error) {
	tx, err := r.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return model.AccountExport{}, err
	}
	defer tx.Rollback(ctx)

	var out model.AccountExport
	err = tx.QueryRow(ctx, `SELECT now(),id,email::text,nombre,apellido,alias,foto_url,tipo_cuenta,activo,(email_verified_at IS NOT NULL),auth_version,version,created_at FROM usuarios WHERE id=$1 AND activo AND deleted_at IS NULL`, id).
		Scan(&out.ExportedAt, &out.User.ID, &out.User.Email, &out.User.Name, &out.User.LastName, &out.User.Alias, &out.User.PhotoURL, &out.User.AccountType, &out.User.Active, &out.User.EmailVerified, &out.User.AuthVersion, &out.User.Version, &out.User.CreatedAt)
	if err != nil {
		return model.AccountExport{}, err
	}

	rows, err := tx.Query(ctx, `SELECT m.id,m.nombre,mm.rol,COALESCE(array_agg(ms.sucursal_id ORDER BY ms.sucursal_id) FILTER(WHERE ms.activo),'{}')
		FROM membresias_marca mm JOIN marcas m ON m.id=mm.marca_id
		LEFT JOIN membresias_sucursales ms ON ms.membresia_id=mm.id
		WHERE mm.usuario_id=$1 AND mm.activo GROUP BY m.id,m.nombre,mm.rol ORDER BY m.id`, id)
	if err != nil {
		return model.AccountExport{}, err
	}
	out.Memberships = make([]model.Membership, 0)
	for rows.Next() {
		var membership model.Membership
		if err = rows.Scan(&membership.BrandID, &membership.BrandName, &membership.Role, &membership.BranchIDs); err != nil {
			rows.Close()
			return model.AccountExport{}, err
		}
		out.Memberships = append(out.Memberships, membership)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return model.AccountExport{}, err
	}
	rows.Close()

	rows, err = tx.Query(ctx, `SELECT t.id,t.marca_id,m.nombre,t.saldo_sellos,t.saldo_puntos,t.activo,t.version,t.created_at
		FROM tarjetas t JOIN marcas m ON m.id=t.marca_id WHERE t.usuario_id=$1 ORDER BY t.id`, id)
	if err != nil {
		return model.AccountExport{}, err
	}
	out.Cards = make([]model.AccountExportCard, 0)
	for rows.Next() {
		var card model.AccountExportCard
		if err = rows.Scan(&card.ID, &card.BrandID, &card.BrandName, &card.BalanceStamps, &card.BalancePoints, &card.Active, &card.Version, &card.CreatedAt); err != nil {
			rows.Close()
			return model.AccountExport{}, err
		}
		out.Cards = append(out.Cards, card)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return model.AccountExport{}, err
	}
	rows.Close()

	rows, err = tx.Query(ctx, `SELECT h.id,h.operation_id,h.tarjeta_id,h.marca_id,h.marca_nombre_snapshot,h.sucursal_id,h.sucursal_nombre_snapshot,h.operacion,h.programa_tipo,h.programa_id_snapshot,h.sentido,h.cantidad,h.saldo_anterior,h.saldo_posterior,h.beneficio_nombre_snapshot,h.beneficio_requisito_snapshot,h.beneficio_requisito_puntos_snapshot,h.occurred_at
		FROM historial_movimientos h JOIN tarjetas t ON t.id=h.tarjeta_id
		WHERE t.usuario_id=$1 OR h.usuario_operador_id=$1 ORDER BY h.occurred_at,h.id`, id)
	if err != nil {
		return model.AccountExport{}, err
	}
	out.Movements = make([]model.Movement, 0)
	for rows.Next() {
		var movement model.Movement
		if err = rows.Scan(&movement.ID, &movement.OperationID, &movement.CardID, &movement.BrandID, &movement.BrandName, &movement.BranchID, &movement.BranchName, &movement.Operation, &movement.ProgramType, &movement.ProgramIDSnapshot, &movement.Direction, &movement.Amount, &movement.BalanceBefore, &movement.BalanceAfter, &movement.BenefitNameSnapshot, &movement.BenefitRequiredStampsSnapshot, &movement.BenefitRequiredPointsSnapshot, &movement.OccurredAt); err != nil {
			rows.Close()
			return model.AccountExport{}, err
		}
		out.Movements = append(out.Movements, movement)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return model.AccountExport{}, err
	}
	rows.Close()
	if err = tx.Commit(ctx); err != nil {
		return model.AccountExport{}, err
	}
	return out, nil
}
