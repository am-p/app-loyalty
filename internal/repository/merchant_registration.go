package repository

import (
	"context"
	"time"

	"clientesFrecuentes/internal/model"

	"github.com/jackc/pgx/v5"
)

func (r *Repository) CreateDemoMerchant(ctx context.Context, key string, fingerprint []byte, email, passwordHash, ownerName, brandName, branchName string, branchAddress *string, programType string, build func(model.User, model.MerchantContext) ([]byte, error)) (IdempotentResult, error) {
	tx, err := r.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return IdempotentResult{}, err
	}
	defer tx.Rollback(ctx)
	actorScope := "demo-email:" + email
	claimed, err := claimIdempotency(ctx, tx, key, actorScope, "REGISTER_DEMO_MERCHANT", fingerprint)
	if err != nil {
		return IdempotentResult{}, err
	}
	if claimed != nil {
		return *claimed, nil
	}
	var u model.User
	err = tx.QueryRow(ctx, `INSERT INTO usuarios(email,password_hash,nombre,tipo_cuenta) VALUES($1,$2,$3,'PERSONAL_MARCA') RETURNING id,email::text,nombre,tipo_cuenta,activo,created_at`, email, passwordHash, ownerName).
		Scan(&u.ID, &u.Email, &u.Name, &u.AccountType, &u.Active, &u.CreatedAt)
	if err != nil {
		return IdempotentResult{}, normalize(err)
	}
	var brandID, membershipID, branchID, programID int64
	var started time.Time
	if err = tx.QueryRow(ctx, `INSERT INTO marcas(nombre) VALUES($1) RETURNING id`, brandName).Scan(&brandID); err != nil {
		return IdempotentResult{}, err
	}
	if err = tx.QueryRow(ctx, `INSERT INTO membresias_marca(usuario_id,marca_id,rol) VALUES($1,$2,'PROPIETARIO') RETURNING id`, u.ID, brandID).Scan(&membershipID); err != nil {
		return IdempotentResult{}, err
	}
	if err = tx.QueryRow(ctx, `INSERT INTO sucursales(marca_id,nombre,direccion) VALUES($1,$2,$3) RETURNING id`, brandID, branchName, branchAddress).Scan(&branchID); err != nil {
		return IdempotentResult{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO membresias_sucursales(membresia_id,sucursal_id,marca_id) VALUES($1,$2,$3)`, membershipID, branchID, brandID); err != nil {
		return IdempotentResult{}, err
	}
	var stampsPerAccumulation *int64
	if programType == "SELLOS" {
		one := int64(1)
		stampsPerAccumulation = &one
	}
	if err = tx.QueryRow(ctx, `INSERT INTO programas_fidelidad(marca_id,tipo,sellos_por_acumulacion) VALUES($1,$2,$3) RETURNING id`, brandID, programType, stampsPerAccumulation).Scan(&programID); err != nil {
		return IdempotentResult{}, err
	}
	demoKind := programType + "_FREE_TRIAL"
	if err = tx.QueryRow(ctx, `INSERT INTO accesos_demo(marca_id,tipo,precio_minor,moneda,cobro_automatico) VALUES($1,$2,0,'ARS',false) RETURNING started_at`, brandID, demoKind).Scan(&started); err != nil {
		return IdempotentResult{}, err
	}
	merchant := model.MerchantContext{BrandID: brandID, BrandName: brandName, Role: "PROPIETARIO", Branch: model.Branch{ID: branchID, BrandID: brandID, Name: branchName, Address: branchAddress, Active: true}, Program: model.Program{ID: programID, BrandID: brandID, Type: programType, StampsPerAccumulation: stampsPerAccumulation, Active: true}, Benefits: []model.Benefit{}, DemoAccess: model.DemoAccess{Kind: demoKind, PriceMinor: 0, Currency: "ARS", AutomaticCharge: false, Active: true, StartedAt: started}}
	body, err := build(u, merchant)
	if err != nil {
		return IdempotentResult{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE solicitudes_idempotentes SET estado='COMPLETED',response_status=201,response_body=$2,completed_at=now() WHERE idempotency_key=$1`, key, []byte(body)); err != nil {
		return IdempotentResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return IdempotentResult{}, normalize(err)
	}
	return IdempotentResult{Status: 201, Body: body}, nil
}
