package repository

import (
	"context"
	"errors"
	"math"

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

func (r *Repository) GetCustomer(ctx context.Context, actorID int64) (model.User, error) {
	u, err := r.GetUserByID(ctx, actorID)
	if err != nil {
		return model.User{}, err
	}
	if u.AccountType != "CLIENTE_FINAL" {
		return model.User{}, ErrNotFound
	}
	return u, nil
}

func (r *Repository) ListCards(ctx context.Context, actorID int64, page, pageSize int) ([]model.Card, int64, error) {
	var total int64
	if err := r.Pool.QueryRow(ctx, `SELECT count(*) FROM tarjetas WHERE usuario_id=$1 AND activo`, actorID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.Pool.Query(ctx, `SELECT t.id,m.id,m.nombre,t.saldo_sellos,b.id,b.programa_id,b.nombre,b.requisito_sellos,b.activo
		FROM tarjetas t JOIN marcas m ON m.id=t.marca_id JOIN programas_fidelidad p ON p.marca_id=m.id AND p.activo
		JOIN beneficios b ON b.programa_id=p.id AND b.activo WHERE t.usuario_id=$1 AND t.activo ORDER BY t.id LIMIT $2 OFFSET $3`, actorID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]model.Card, 0)
	for rows.Next() {
		var c model.Card
		if err = rows.Scan(&c.ID, &c.BrandID, &c.BrandName, &c.BalanceStamps, &c.Benefit.ID, &c.Benefit.ProgramID, &c.Benefit.Name, &c.Benefit.RequiredStamps, &c.Benefit.Active); err != nil {
			return nil, 0, err
		}
		items = append(items, c)
	}
	return items, total, rows.Err()
}

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
	q := `SELECT h.id,h.operation_id,h.tarjeta_id,h.marca_id,m.nombre,h.sucursal_id,s.nombre,h.operacion,h.sentido,h.cantidad,h.saldo_anterior,h.saldo_posterior,h.beneficio_nombre_snapshot,h.beneficio_requisito_snapshot,h.occurred_at FROM historial_movimientos h JOIN tarjetas t ON t.id=h.tarjeta_id JOIN marcas m ON m.id=h.marca_id JOIN sucursales s ON s.id=h.sucursal_id WHERE ` + where
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
		if err = rows.Scan(&m.ID, &m.OperationID, &m.CardID, &m.BrandID, &m.BrandName, &m.BranchID, &m.BranchName, &m.Operation, &m.Direction, &m.Amount, &m.BalanceBefore, &m.BalanceAfter, &m.BenefitNameSnapshot, &m.BenefitRequiredStampsSnapshot, &m.OccurredAt); err != nil {
			return nil, 0, err
		}
		items = append(items, m)
	}
	return items, total, rows.Err()
}

func totalPages(total int64, size int) int64 {
	if total == 0 {
		return 0
	}
	return int64(math.Ceil(float64(total) / float64(size)))
}
func itoa(v int) string {
	if v < 10 {
		return string(rune('0' + v))
	}
	return fmtInt(v)
}
func fmtInt(v int) string {
	digits := ""
	for v > 0 {
		digits = string(rune('0'+v%10)) + digits
		v /= 10
	}
	return digits
}
