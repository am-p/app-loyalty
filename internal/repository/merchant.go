package repository

import (
	"context"
	"errors"

	"clientesFrecuentes/internal/model"

	"github.com/jackc/pgx/v5"
)

const merchantContextSelect = `SELECT m.id,m.nombre,mm.rol,s.id,s.marca_id,s.nombre,s.direccion,s.activo,
	p.id,p.marca_id,p.tipo,p.sellos_por_acumulacion,p.activo,
	b.id,b.programa_id,b.nombre,b.requisito_sellos,b.activo,
	a.tipo,a.precio_minor,a.moneda,a.cobro_automatico,a.activo,a.started_at
	FROM membresias_marca mm JOIN marcas m ON m.id=mm.marca_id AND m.activo
	JOIN membresias_sucursales ms ON ms.membresia_id=mm.id AND ms.activo
	JOIN sucursales s ON s.id=ms.sucursal_id AND s.activo
	JOIN programas_fidelidad p ON p.marca_id=m.id AND p.activo AND p.tipo='SELLOS'
	JOIN beneficios b ON b.programa_id=p.id AND b.activo
	JOIN accesos_demo a ON a.marca_id=m.id AND a.activo
	WHERE mm.usuario_id=$1 AND mm.activo`

func scanMerchant(row pgx.Row) (model.MerchantContext, error) {
	var m model.MerchantContext
	err := row.Scan(&m.BrandID, &m.BrandName, &m.Role, &m.Branch.ID, &m.Branch.BrandID, &m.Branch.Name, &m.Branch.Address, &m.Branch.Active,
		&m.Program.ID, &m.Program.BrandID, &m.Program.Type, &m.Program.StampsPerAccumulation, &m.Program.Active,
		&m.Benefit.ID, &m.Benefit.ProgramID, &m.Benefit.Name, &m.Benefit.RequiredStamps, &m.Benefit.Active,
		&m.DemoAccess.Kind, &m.DemoAccess.PriceMinor, &m.DemoAccess.Currency, &m.DemoAccess.AutomaticCharge, &m.DemoAccess.Active, &m.DemoAccess.StartedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.MerchantContext{}, ErrNotFound
	}
	return m, err
}

func (r *Repository) ListMerchantContexts(ctx context.Context, actorID int64) ([]model.MerchantContext, error) {
	rows, err := r.Pool.Query(ctx, merchantContextSelect+` ORDER BY m.id,s.id`, actorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.MerchantContext, 0)
	for rows.Next() {
		m, err := scanMerchant(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, m)
	}
	return items, rows.Err()
}

func (r *Repository) GetMerchantContext(ctx context.Context, actorID, brandID int64) (model.MerchantContext, error) {
	return scanMerchant(r.Pool.QueryRow(ctx, merchantContextSelect+` AND m.id=$2 ORDER BY s.id LIMIT 1`, actorID, brandID))
}

func (r *Repository) ListBrandCustomers(ctx context.Context, actorID, brandID int64, page, pageSize int, search string) ([]model.BrandCustomer, int64, error) {
	tx, err := r.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback(ctx)

	var authorized bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM usuarios u
		JOIN membresias_marca mm ON mm.usuario_id=u.id AND mm.activo AND mm.rol='PROPIETARIO'
		JOIN marcas m ON m.id=mm.marca_id AND m.activo AND m.deleted_at IS NULL
		WHERE u.id=$1 AND u.tipo_cuenta='PERSONAL_MARCA' AND u.activo AND u.deleted_at IS NULL AND m.id=$2
	)`, actorID, brandID).Scan(&authorized)
	if err != nil {
		return nil, 0, err
	}
	if !authorized {
		return nil, 0, ErrNotFound
	}

	const cardsFrom = ` FROM tarjetas t
		JOIN usuarios u ON u.id=t.usuario_id AND u.tipo_cuenta='CLIENTE_FINAL' AND u.activo AND u.deleted_at IS NULL`
	const cardsWhere = ` WHERE t.marca_id=$1 AND t.activo AND t.deleted_at IS NULL
		AND ($2='' OR strpos(lower(u.nombre),lower($2))>0 OR strpos(lower(u.email::text),lower($2))>0)`
	var total int64
	if err = tx.QueryRow(ctx, `SELECT count(*)`+cardsFrom+cardsWhere, brandID, search).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := tx.Query(ctx, `SELECT u.id,t.id,u.nombre,u.email::text,t.saldo_sellos,
		count(h.id),max(h.occurred_at),t.created_at`+cardsFrom+`
		LEFT JOIN historial_movimientos h ON h.tarjeta_id=t.id AND h.marca_id=t.marca_id`+cardsWhere+`
		GROUP BY u.id,t.id,u.nombre,u.email,t.saldo_sellos,t.created_at
		ORDER BY max(h.occurred_at) DESC NULLS LAST,t.id DESC LIMIT $3 OFFSET $4`, brandID, search, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	items := make([]model.BrandCustomer, 0)
	for rows.Next() {
		var item model.BrandCustomer
		if err = rows.Scan(&item.CustomerID, &item.CardID, &item.Name, &item.Email, &item.BalanceStamps, &item.MovementsCount, &item.LastMovementAt, &item.JoinedAt); err != nil {
			rows.Close()
			return nil, 0, err
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, 0, err
	}
	rows.Close()
	if err = tx.Commit(ctx); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *Repository) BrandMetricsSummary(ctx context.Context, actorID, brandID int64) (model.BrandMetricsSummary, error) {
	const query = `WITH authorized AS (
		SELECT 1 FROM usuarios u
		JOIN membresias_marca mm ON mm.usuario_id=u.id AND mm.activo AND mm.rol='PROPIETARIO'
		JOIN marcas m ON m.id=mm.marca_id AND m.activo AND m.deleted_at IS NULL
		WHERE u.id=$1 AND u.tipo_cuenta='PERSONAL_MARCA' AND u.activo AND u.deleted_at IS NULL AND m.id=$2
	), active_cards AS (
		SELECT count(*) AS active_customers,COALESCE(sum(t.saldo_sellos),0)::bigint AS current_stamp_balance
		FROM tarjetas t JOIN usuarios u ON u.id=t.usuario_id AND u.tipo_cuenta='CLIENTE_FINAL' AND u.activo AND u.deleted_at IS NULL
		WHERE t.marca_id=$2 AND t.activo AND t.deleted_at IS NULL AND EXISTS(SELECT 1 FROM authorized)
	), ledger AS (
		SELECT count(*) FILTER(WHERE h.operacion='ACUMULACION') AS accumulations,
			count(*) FILTER(WHERE h.operacion='CANJE') AS redemptions,
			COALESCE(sum(h.cantidad) FILTER(WHERE h.operacion='ACUMULACION' AND h.sentido='CREDITO'),0)::bigint AS stamps_issued,
			COALESCE(sum(h.cantidad) FILTER(WHERE h.operacion='CANJE' AND h.sentido='DEBITO'),0)::bigint AS stamps_redeemed,
			max(h.occurred_at) AS last_movement_at
		FROM historial_movimientos h WHERE h.marca_id=$2 AND EXISTS(SELECT 1 FROM authorized)
	)
	SELECT EXISTS(SELECT 1 FROM authorized),active_cards.active_customers,active_cards.current_stamp_balance,
		ledger.accumulations,ledger.redemptions,ledger.stamps_issued,ledger.stamps_redeemed,ledger.last_movement_at
	FROM active_cards CROSS JOIN ledger`
	var authorized bool
	var result model.BrandMetricsSummary
	err := r.Pool.QueryRow(ctx, query, actorID, brandID).Scan(&authorized, &result.ActiveCustomers, &result.CurrentStampBalance,
		&result.Accumulations, &result.Redemptions, &result.StampsIssued, &result.StampsRedeemed, &result.LastMovementAt)
	if err != nil {
		return model.BrandMetricsSummary{}, err
	}
	if !authorized {
		return model.BrandMetricsSummary{}, ErrNotFound
	}
	return result, nil
}
