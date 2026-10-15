package repository

import (
	"context"
	"errors"
	"strconv"

	"clientesFrecuentes/internal/model"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ReverseBrandMovement(ctx context.Context, actorID, brandID int64, originalOperationID string) (model.MovementReversal, error) {
	tx, err := r.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return model.MovementReversal{}, err
	}
	defer tx.Rollback(ctx)

	var movementID, cardID, customerID, amount, currentBalance int64
	var operation, programType string
	err = tx.QueryRow(ctx, `SELECT h.id,h.tarjeta_id,t.usuario_id,h.operacion,h.programa_tipo,h.cantidad,
		CASE WHEN h.programa_tipo='PUNTOS' THEN t.saldo_puntos ELSE t.saldo_sellos END
		FROM historial_movimientos h
		JOIN tarjetas t ON t.id=h.tarjeta_id AND t.activo AND t.deleted_at IS NULL
		WHERE h.marca_id=$1 AND h.operation_id=$2
		AND EXISTS(SELECT 1 FROM membresias_marca mm WHERE mm.usuario_id=$3 AND mm.marca_id=$1 AND mm.activo
			AND (mm.rol IN ('PROPIETARIO','ADMINISTRADOR') OR EXISTS(
				SELECT 1 FROM membresias_sucursales ms WHERE ms.membresia_id=mm.id AND ms.marca_id=mm.marca_id
				AND ms.activo AND ms.sucursal_id=h.sucursal_id)))
		FOR UPDATE OF t`, brandID, originalOperationID, actorID).
		Scan(&movementID, &cardID, &customerID, &operation, &programType, &amount, &currentBalance)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.MovementReversal{}, ErrNotFound
	}
	if err != nil {
		return model.MovementReversal{}, err
	}

	var existing model.MovementReversal
	err = tx.QueryRow(ctx, `SELECT operation_id::text,movimiento_id,saldo_anterior,saldo_posterior,occurred_at
		FROM reversiones_movimiento WHERE movimiento_id=$1`, movementID).
		Scan(&existing.OperationID, &existing.MovementID, &existing.BalanceBefore, &existing.BalanceAfter, &existing.OccurredAt)
	if err == nil {
		if err = tx.Commit(ctx); err != nil {
			return model.MovementReversal{}, normalize(err)
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return model.MovementReversal{}, err
	}

	if programType != "SELLOS" && programType != "PUNTOS" {
		return model.MovementReversal{}, ErrConflict
	}
	after := currentBalance
	switch operation {
	case "ACUMULACION":
		if currentBalance < amount {
			return model.MovementReversal{}, ErrInsufficientBalance
		}
		after = currentBalance - amount
	case "CANJE":
		after = currentBalance + amount
	default:
		return model.MovementReversal{}, ErrConflict
	}

	balanceColumn := "saldo_sellos"
	if programType == "PUNTOS" {
		balanceColumn = "saldo_puntos"
	}
	if _, err = tx.Exec(ctx, `UPDATE tarjetas SET `+balanceColumn+`=$1,version=version+1 WHERE id=$2`, after, cardID); err != nil {
		return model.MovementReversal{}, err
	}

	reversal := model.MovementReversal{
		OperationID:   uuid.NewString(),
		MovementID:    movementID,
		BalanceBefore: currentBalance,
		BalanceAfter:  after,
	}
	err = tx.QueryRow(ctx, `INSERT INTO reversiones_movimiento(operation_id,movimiento_id,usuario_operador_id,saldo_anterior,saldo_posterior)
		VALUES($1,$2,$3,$4,$5) RETURNING occurred_at`, reversal.OperationID, movementID, actorID, currentBalance, after).
		Scan(&reversal.OccurredAt)
	if err != nil {
		return model.MovementReversal{}, normalize(err)
	}
	if _, err = tx.Exec(ctx, `SELECT pg_notify('puntazo_customer_cards',$1)`, strconv.FormatInt(customerID, 10)); err != nil {
		return model.MovementReversal{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return model.MovementReversal{}, normalize(err)
	}
	return reversal, nil
}
