package repository

import (
	"context"

	"clientesFrecuentes/internal/model"
)

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
	rows, err := r.Pool.Query(ctx, `SELECT t.id,m.id,m.nombre,p.tipo,t.saldo_sellos,t.saldo_puntos,b.id,b.programa_id,b.nombre,b.requisito_sellos,b.requisito_puntos,b.activo
		FROM tarjetas t JOIN marcas m ON m.id=t.marca_id JOIN programas_fidelidad p ON p.marca_id=m.id AND p.activo
		JOIN beneficios b ON b.programa_id=p.id AND b.activo WHERE t.usuario_id=$1 AND t.activo ORDER BY t.id LIMIT $2 OFFSET $3`, actorID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]model.Card, 0)
	for rows.Next() {
		var c model.Card
		if err = rows.Scan(&c.ID, &c.BrandID, &c.BrandName, &c.ProgramType, &c.BalanceStamps, &c.BalancePoints, &c.Benefit.ID, &c.Benefit.ProgramID, &c.Benefit.Name, &c.Benefit.RequiredStamps, &c.Benefit.RequiredPoints, &c.Benefit.Active); err != nil {
			return nil, 0, err
		}
		items = append(items, c)
	}
	return items, total, rows.Err()
}
