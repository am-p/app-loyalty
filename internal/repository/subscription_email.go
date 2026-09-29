package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"clientesFrecuentes/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) SubscriptionBrandForOwner(ctx context.Context, actorID int64, providerID string) (int64, error) {
	var brandID int64
	err := r.Pool.QueryRow(ctx, `SELECT s.marca_id FROM suscripciones_marca s
 JOIN marcas m ON m.id=s.marca_id JOIN membresias_marca mm ON mm.marca_id=m.id
 JOIN usuarios u ON u.id=mm.usuario_id
 WHERE s.proveedor_suscripcion_id=$1 AND mm.usuario_id=$2 AND mm.rol='PROPIETARIO' AND mm.activo
 AND m.activo AND m.deleted_at IS NULL AND u.activo AND u.deleted_at IS NULL`, providerID, actorID).Scan(&brandID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return brandID, err
}

// The subscription row is locked by the webhook update, or by the explicit
// confirmation request. The unique provider ID covers distinct retry events too.
func (r *Repository) enqueueSubscriptionConfirmation(ctx context.Context, tx pgx.Tx, brandID int64) error {
	var d model.SubscriptionConfirmationDetails
	var ownerID int64
	var to string
	err := tx.QueryRow(ctx, `SELECT s.marca_id,s.proveedor_suscripcion_id,m.nombre,u.id,u.email::text,u.nombre,
 p.tipo,s.cantidad_sucursales,s.importe_mensual_minor,s.moneda,s.trial_months,s.updated_at,s.proximo_cobro_at
 FROM suscripciones_marca s JOIN marcas m ON m.id=s.marca_id
 JOIN membresias_marca mm ON mm.marca_id=m.id AND mm.rol='PROPIETARIO' AND mm.activo
 JOIN usuarios u ON u.id=mm.usuario_id JOIN programas_fidelidad p ON p.marca_id=m.id AND p.activo
 WHERE s.marca_id=$1 AND s.estado='AUTHORIZED' AND m.activo AND m.deleted_at IS NULL
 AND u.activo AND u.deleted_at IS NULL AND u.email_verified_at IS NOT NULL
 ORDER BY mm.id LIMIT 1`, brandID).Scan(&d.BrandID, &d.ProviderID, &d.BrandName, &ownerID, &to, &d.OwnerName, &d.ProgramType, &d.ActiveBranches, &d.MonthlyAmountMinor, &d.Currency, &d.TrialMonths, &d.ConfirmedAt, &d.NextPaymentDate)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(r.OutboxCipherKey) != 32 {
		return ErrEmailUnavailable
	}
	payload, err := json.Marshal(d)
	if err != nil {
		return err
	}
	id := uuid.NewString()
	encrypted, nonce, err := encryptOutboxToken(string(payload), id, r.OutboxCipherKey)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO email_outbox(id,usuario_id,tipo,destinatario,asunto,subscription_brand_id,subscription_provider_id,token_ciphertext,token_nonce,token_expires_at)
 VALUES($1,$2,'SUBSCRIPTION_CONFIRMATION',$3,'Tu suscripción de Puntazo está autorizada',$4,$5,$6,$7,$8)
 ON CONFLICT (subscription_provider_id) WHERE tipo='SUBSCRIPTION_CONFIRMATION' DO NOTHING`, id, ownerID, to, d.BrandID, d.ProviderID, encrypted, nonce, r.Now().Add(7*24*time.Hour))
	return err
}

func (r *Repository) RequestSubscriptionConfirmation(ctx context.Context, actorID, brandID int64) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var status string
	err = tx.QueryRow(ctx, `SELECT s.estado FROM suscripciones_marca s JOIN marcas m ON m.id=s.marca_id
 JOIN membresias_marca mm ON mm.marca_id=m.id JOIN usuarios u ON u.id=mm.usuario_id
 WHERE s.marca_id=$1 AND mm.usuario_id=$2 AND mm.rol='PROPIETARIO' AND mm.activo
 AND m.activo AND m.deleted_at IS NULL AND u.activo AND u.deleted_at IS NULL AND u.email_verified_at IS NOT NULL
 FOR UPDATE OF s`, brandID, actorID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if status != "AUTHORIZED" {
		return ErrConflict
	}
	if err = r.enqueueSubscriptionConfirmation(ctx, tx, brandID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) validateSubscriptionConfirmation(ctx context.Context, item model.OutboxEmail, payload string) error {
	var d model.SubscriptionConfirmationDetails
	if err := json.Unmarshal([]byte(payload), &d); err != nil {
		return errors.New("invalid subscription email payload")
	}
	var valid bool
	err := r.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM email_outbox e
 JOIN suscripciones_marca s ON s.marca_id=e.subscription_brand_id AND s.proveedor_suscripcion_id=e.subscription_provider_id
 JOIN marcas m ON m.id=s.marca_id JOIN usuarios u ON u.id=e.usuario_id
 JOIN membresias_marca mm ON mm.usuario_id=u.id AND mm.marca_id=m.id AND mm.rol='PROPIETARIO' AND mm.activo
 WHERE e.id=$1 AND e.tipo='SUBSCRIPTION_CONFIRMATION' AND e.estado='SENDING' AND e.lease_owner=$2
 AND e.destinatario=$3 AND u.email=$3 AND u.activo AND u.deleted_at IS NULL AND u.email_verified_at IS NOT NULL
 AND m.activo AND m.deleted_at IS NULL AND s.estado='AUTHORIZED'
 AND e.subscription_brand_id=$4 AND e.subscription_provider_id=$5)`, item.ID, item.LeaseOwner, item.To, d.BrandID, d.ProviderID).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return errors.New("subscription email correlation is invalid or subscription is no longer authorized")
	}
	return nil
}
