package repository

import (
	"context"
	"errors"

	"clientesFrecuentes/internal/model"
	"github.com/jackc/pgx/v5"
)

type BillingContext struct {
	PayerEmail     string
	ActiveBranches int64
	ProgramType    string
}
type SubscriptionRecord struct {
	Subscription                  model.Subscription
	ProviderID, ExternalReference string
	TrialMonths                   int
}

// The caller holds the brand row lock, so a checkout cannot race a branch change or deletion.
func requireNoActiveSubscription(ctx context.Context, tx pgx.Tx, brandID int64) error {
	var active bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM suscripciones_marca WHERE marca_id=$1 AND estado<>'CANCELLED')`, brandID).Scan(&active); err != nil {
		return err
	}
	if active {
		return ErrSubscriptionChangeRequired
	}
	return nil
}

func (r *Repository) BillingContext(ctx context.Context, actorID, brandID int64) (BillingContext, error) {
	var out BillingContext
	err := r.Pool.QueryRow(ctx, `SELECT u.email::text,(SELECT count(*) FROM sucursales s WHERE s.marca_id=$2 AND s.activo AND s.deleted_at IS NULL),p.tipo FROM membresias_marca mm JOIN usuarios u ON u.id=mm.usuario_id JOIN marcas m ON m.id=mm.marca_id AND m.activo JOIN programas_fidelidad p ON p.marca_id=m.id AND p.activo WHERE mm.usuario_id=$1 AND mm.marca_id=$2 AND mm.activo AND mm.rol='PROPIETARIO'`, actorID, brandID).Scan(&out.PayerEmail, &out.ActiveBranches, &out.ProgramType)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrForbidden
	}
	if err != nil {
		return out, err
	}
	if out.ActiveBranches < 1 {
		return out, ErrInvalidRequest
	}
	return out, nil
}

func (r *Repository) GetSubscriptionRecord(ctx context.Context, brandID int64) (SubscriptionRecord, error) {
	var out SubscriptionRecord
	s := &out.Subscription
	err := r.Pool.QueryRow(ctx, `SELECT marca_id,proveedor,estado,moneda,precio_sucursal_minor,cantidad_sucursales,importe_mensual_minor,COALESCE(checkout_url,''),proximo_cobro_at,updated_at,COALESCE(proveedor_suscripcion_id,''),referencia_externa,trial_months FROM suscripciones_marca WHERE marca_id=$1`, brandID).Scan(&s.BrandID, &s.Provider, &s.Status, &s.Currency, &s.UnitAmountCents, &s.ActiveBranches, &s.MonthlyAmountCents, &s.CheckoutURL, &s.NextPaymentDate, &s.UpdatedAt, &out.ProviderID, &out.ExternalReference, &out.TrialMonths)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	s.TrialAvailable = s.Status == "CREATING" && out.TrialMonths > 0
	return out, err
}

// ReserveSubscriptionCheckout commits the idempotency key before the external API call.
// A CREATING row keeps its reference for reconciliation after an uncertain provider call.
func (r *Repository) ReserveSubscriptionCheckout(ctx context.Context, actorID, brandID int64, external string, stampPrice, pointsPrice int64) (BillingContext, SubscriptionRecord, error) {
	var billing BillingContext
	var record SubscriptionRecord
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return billing, record, err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `SELECT u.email::text,p.tipo,(SELECT count(*) FROM sucursales s WHERE s.marca_id=$2 AND s.activo AND s.deleted_at IS NULL) FROM marcas m JOIN membresias_marca mm ON mm.marca_id=m.id AND mm.usuario_id=$1 AND mm.activo AND mm.rol='PROPIETARIO' JOIN usuarios u ON u.id=mm.usuario_id JOIN programas_fidelidad p ON p.marca_id=m.id AND p.activo WHERE m.id=$2 AND m.activo FOR UPDATE OF m`, actorID, brandID).Scan(&billing.PayerEmail, &billing.ProgramType, &billing.ActiveBranches)
	if errors.Is(err, pgx.ErrNoRows) {
		return billing, record, ErrForbidden
	}
	if err != nil {
		return billing, record, err
	}
	if billing.ActiveBranches < 1 {
		return billing, record, ErrInvalidRequest
	}
	unitPrice := stampPrice
	if billing.ProgramType == "PUNTOS" {
		unitPrice = pointsPrice
	} else if billing.ProgramType != "SELLOS" {
		return billing, record, ErrInvalidRequest
	}
	if unitPrice < 1 {
		return billing, record, ErrInvalidRequest
	}
	err = tx.QueryRow(ctx, `SELECT estado,COALESCE(proveedor_suscripcion_id,''),referencia_externa,trial_months,marca_id,proveedor,moneda,precio_sucursal_minor,cantidad_sucursales,importe_mensual_minor,COALESCE(checkout_url,''),proximo_cobro_at,updated_at FROM suscripciones_marca WHERE marca_id=$1 FOR UPDATE`, brandID).Scan(&record.Subscription.Status, &record.ProviderID, &record.ExternalReference, &record.TrialMonths, &record.Subscription.BrandID, &record.Subscription.Provider, &record.Subscription.Currency, &record.Subscription.UnitAmountCents, &record.Subscription.ActiveBranches, &record.Subscription.MonthlyAmountCents, &record.Subscription.CheckoutURL, &record.Subscription.NextPaymentDate, &record.Subscription.UpdatedAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return billing, record, err
	}
	if err == nil {
		if record.Subscription.Status == "CREATING" {
			record.Subscription.TrialAvailable = record.TrialMonths > 0
			return billing, record, tx.Commit(ctx)
		}
		if record.ExternalReference == external {
			return billing, record, tx.Commit(ctx)
		}
		if record.Subscription.Status != "CANCELLED" {
			return billing, record, ErrConflict
		}
	}
	trialMonths := 0
	if errors.Is(err, pgx.ErrNoRows) {
		trialMonths = 1
	}
	query := `INSERT INTO suscripciones_marca(marca_id,proveedor,proveedor_suscripcion_id,referencia_externa,estado,moneda,precio_sucursal_minor,cantidad_sucursales,importe_mensual_minor,trial_months) VALUES($1,'MERCADO_PAGO',NULL,$2,'CREATING','ARS',$3,$4,$3::bigint*$4::bigint,$5) ON CONFLICT(marca_id) DO UPDATE SET proveedor_suscripcion_id=NULL,referencia_externa=EXCLUDED.referencia_externa,estado='CREATING',precio_sucursal_minor=EXCLUDED.precio_sucursal_minor,cantidad_sucursales=EXCLUDED.cantidad_sucursales,importe_mensual_minor=EXCLUDED.importe_mensual_minor,trial_months=0,provider_call_started_at=NULL,checkout_url=NULL,proximo_cobro_at=NULL,updated_at=now() RETURNING marca_id,proveedor,estado,moneda,precio_sucursal_minor,cantidad_sucursales,importe_mensual_minor,COALESCE(checkout_url,''),proximo_cobro_at,updated_at,referencia_externa,trial_months`
	err = tx.QueryRow(ctx, query, brandID, external, unitPrice, billing.ActiveBranches, trialMonths).Scan(&record.Subscription.BrandID, &record.Subscription.Provider, &record.Subscription.Status, &record.Subscription.Currency, &record.Subscription.UnitAmountCents, &record.Subscription.ActiveBranches, &record.Subscription.MonthlyAmountCents, &record.Subscription.CheckoutURL, &record.Subscription.NextPaymentDate, &record.Subscription.UpdatedAt, &record.ExternalReference, &record.TrialMonths)
	if err != nil {
		return billing, record, err
	}
	record.Subscription.TrialAvailable = record.TrialMonths > 0
	return billing, record, tx.Commit(ctx)
}

// ClaimSubscriptionProviderCall allows at most one POST to /preapproval. The
// provider does not document idempotency for that endpoint, so an uncertain
// result must be reconciled via webhook or operator review before another POST.
func (r *Repository) ClaimSubscriptionProviderCall(ctx context.Context, brandID int64, external string) (bool, error) {
	tag, err := r.Pool.Exec(ctx, `UPDATE suscripciones_marca SET provider_call_started_at=now(),updated_at=now() WHERE marca_id=$1 AND referencia_externa=$2 AND estado='CREATING' AND provider_call_started_at IS NULL`, brandID, external)
	return tag.RowsAffected() == 1, err
}

func (r *Repository) SaveSubscriptionCheckout(ctx context.Context, brandID int64, provider model.BillingSubscriptionResult) (model.Subscription, error) {
	status, ok := providerStatus(provider.Status)
	if !ok {
		return model.Subscription{}, ErrInvalidRequest
	}
	var out model.Subscription
	err := r.Pool.QueryRow(ctx, `UPDATE suscripciones_marca SET proveedor_suscripcion_id=$2,estado=$3,checkout_url=NULLIF($4,''),proximo_cobro_at=$5,updated_at=now() WHERE marca_id=$1 AND referencia_externa=$6 AND estado='CREATING' AND provider_call_started_at IS NOT NULL RETURNING marca_id,proveedor,estado,moneda,precio_sucursal_minor,cantidad_sucursales,importe_mensual_minor,COALESCE(checkout_url,''),proximo_cobro_at,updated_at`, brandID, provider.ID, status, provider.CheckoutURL, provider.NextPaymentDate, provider.ExternalReference).Scan(&out.BrandID, &out.Provider, &out.Status, &out.Currency, &out.UnitAmountCents, &out.ActiveBranches, &out.MonthlyAmountCents, &out.CheckoutURL, &out.NextPaymentDate, &out.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrConflict
	}
	return out, normalize(err)
}

func (r *Repository) RecordSubscriptionWebhook(ctx context.Context, notificationID, topic string, provider model.BillingSubscriptionResult) error {
	status, ok := providerStatus(provider.Status)
	if !ok {
		return ErrInvalidRequest
	}
	tx, err := r.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `INSERT INTO eventos_mercado_pago(notification_id,resource_id,topic) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, notificationID, provider.ID, topic)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return tx.Commit(ctx)
	}
	tag, err = tx.Exec(ctx, `UPDATE suscripciones_marca SET proveedor_suscripcion_id=COALESCE(proveedor_suscripcion_id,$1),estado=$2,checkout_url=COALESCE(NULLIF($3,''),checkout_url),proximo_cobro_at=$4,updated_at=now() WHERE referencia_externa=$5 AND (proveedor_suscripcion_id=$1 OR (estado='CREATING' AND proveedor_suscripcion_id IS NULL AND provider_call_started_at IS NOT NULL))`, provider.ID, status, provider.CheckoutURL, provider.NextPaymentDate, provider.ExternalReference)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return tx.Commit(ctx)
}

func (r *Repository) UpdateSubscriptionFromProvider(ctx context.Context, provider model.BillingSubscriptionResult) (model.Subscription, error) {
	status, ok := providerStatus(provider.Status)
	if !ok {
		return model.Subscription{}, ErrInvalidRequest
	}
	var out model.Subscription
	err := r.Pool.QueryRow(ctx, `UPDATE suscripciones_marca SET estado=$2,checkout_url=COALESCE(NULLIF($3,''),checkout_url),proximo_cobro_at=$4,updated_at=now() WHERE proveedor_suscripcion_id=$1 AND referencia_externa=$5 RETURNING marca_id,proveedor,estado,moneda,precio_sucursal_minor,cantidad_sucursales,importe_mensual_minor,COALESCE(checkout_url,''),proximo_cobro_at,updated_at`, provider.ID, status, provider.CheckoutURL, provider.NextPaymentDate, provider.ExternalReference).Scan(&out.BrandID, &out.Provider, &out.Status, &out.Currency, &out.UnitAmountCents, &out.ActiveBranches, &out.MonthlyAmountCents, &out.CheckoutURL, &out.NextPaymentDate, &out.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	return out, err
}

func providerStatus(value string) (string, bool) {
	switch value {
	case "pending":
		return "PENDING", true
	case "authorized":
		return "AUTHORIZED", true
	case "paused":
		return "PAUSED", true
	case "cancelled", "canceled":
		return "CANCELLED", true
	default:
		return "", false
	}
}
