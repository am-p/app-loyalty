package repository

import (
	"context"

	"clientesFrecuentes/internal/model"
)

func (r *Repository) ListCustomerMovements(ctx context.Context, actorID, cardID int64, page, pageSize int) ([]model.Movement, int64, error) {
	var total int64
	err := r.Pool.QueryRow(ctx, `SELECT count(*) FROM historial_movimientos h JOIN tarjetas t ON t.id=h.tarjeta_id WHERE h.tarjeta_id=$1 AND t.usuario_id=$2`, cardID, actorID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	if total == 0 {
		var exists bool
		_ = r.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tarjetas WHERE id=$1 AND usuario_id=$2)`, cardID, actorID).Scan(&exists)
		if !exists {
			return nil, 0, ErrNotFound
		}
	}
	return r.listMovements(ctx, `h.tarjeta_id=$1 AND t.usuario_id=$2`, []any{cardID, actorID}, page, pageSize, total)
}

func (r *Repository) ListBrandMovements(ctx context.Context, actorID, brandID int64, page, pageSize int) ([]model.Movement, int64, error) {
	var owns bool
	_ = r.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM membresias_marca WHERE usuario_id=$1 AND marca_id=$2 AND activo)`, actorID, brandID).Scan(&owns)
	if !owns {
		return nil, 0, ErrNotFound
	}
	var total int64
	if err := r.Pool.QueryRow(ctx, `SELECT count(*) FROM historial_movimientos h WHERE h.marca_id=$1 AND EXISTS(SELECT 1 FROM membresias_marca mm JOIN membresias_sucursales ms ON ms.membresia_id=mm.id WHERE mm.usuario_id=$2 AND mm.marca_id=$1 AND mm.activo AND ms.activo AND ms.sucursal_id=h.sucursal_id)`, brandID, actorID).Scan(&total); err != nil {
		return nil, 0, err
	}
	return r.listMovements(ctx, `h.marca_id=$1 AND EXISTS(SELECT 1 FROM membresias_marca mm JOIN membresias_sucursales ms ON ms.membresia_id=mm.id WHERE mm.usuario_id=$2 AND mm.marca_id=$1 AND mm.activo AND ms.activo AND ms.sucursal_id=h.sucursal_id)`, []any{brandID, actorID}, page, pageSize, total)
}

func (r *Repository) listMovements(ctx context.Context, where string, args []any, page, pageSize int, total int64) ([]model.Movement, int64, error) {
	q := `SELECT h.id,h.operation_id,h.tarjeta_id,h.marca_id,m.nombre,h.sucursal_id,s.nombre,h.operacion,h.programa_tipo,h.sentido,h.cantidad,h.saldo_anterior,h.saldo_posterior,h.beneficio_nombre_snapshot,h.beneficio_requisito_snapshot,h.beneficio_requisito_puntos_snapshot,h.occurred_at FROM historial_movimientos h JOIN tarjetas t ON t.id=h.tarjeta_id JOIN marcas m ON m.id=h.marca_id JOIN sucursales s ON s.id=h.sucursal_id WHERE ` + where
	args = append(args, pageSize, (page-1)*pageSize)
	q += ` ORDER BY h.occurred_at DESC,h.id DESC LIMIT $` + itoa(len(args)-1) + ` OFFSET $` + itoa(len(args))
	rows, err := r.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]model.Movement, 0)
	for rows.Next() {
		var m model.Movement
		if err = rows.Scan(&m.ID, &m.OperationID, &m.CardID, &m.BrandID, &m.BrandName, &m.BranchID, &m.BranchName, &m.Operation, &m.ProgramType, &m.Direction, &m.Amount, &m.BalanceBefore, &m.BalanceAfter, &m.BenefitNameSnapshot, &m.BenefitRequiredStampsSnapshot, &m.BenefitRequiredPointsSnapshot, &m.OccurredAt); err != nil {
			return nil, 0, err
		}
		items = append(items, m)
	}
	return items, total, rows.Err()
}
