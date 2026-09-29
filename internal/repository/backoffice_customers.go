package repository

import (
	"context"
	"time"
)

// BackofficeCustomerBranch includes inactive branches for administrative history.
type BackofficeCustomerBranch struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	Address    *string `json:"address"`
	Locality   *string `json:"locality"`
	Province   *string `json:"province"`
	PostalCode *string `json:"postal_code"`
	Active     bool    `json:"active"`
	Primary    bool    `json:"primary"`
}

// BackofficeCustomer represents a commercial customer, including closed brands.
// Payment totals include verified, positive charges that have not been reversed.
type BackofficeCustomer struct {
	BrandID                  int64                      `json:"brand_id"`
	BrandName                string                     `json:"brand_name"`
	Active                   bool                       `json:"active"`
	CreatedAt                time.Time                  `json:"created_at"`
	ProgramType              *string                    `json:"program_type"`
	SubscriptionStatus       *string                    `json:"subscription_status"`
	UnitPriceMinor           *int64                     `json:"unit_price_minor"`
	FullUnitPriceMinor       *int64                     `json:"full_unit_price_minor"`
	MonthlyAmountMinor       *int64                     `json:"monthly_amount_minor"`
	SubscribedBranches       *int64                     `json:"subscribed_branches"`
	NextPaymentAt            *time.Time                 `json:"next_payment_at"`
	PaidCharges              int64                      `json:"paid_charges"`
	PaidAmountMinor          int64                      `json:"paid_amount_minor"`
	RefundedCharges          int64                      `json:"refunded_charges"`
	LastPaymentAt            *time.Time                 `json:"last_payment_at"`
	ReferralCode             *string                    `json:"referral_code"`
	SourceKind               *string                    `json:"source_kind"`
	SourceName               *string                    `json:"source_name"`
	CampaignName             *string                    `json:"campaign_name"`
	DiscountBPS              int                        `json:"discount_bps"`
	DiscountCharges          int64                      `json:"discount_charges"`
	DiscountRemainingCharges int64                      `json:"discount_remaining_charges"`
	Branches                 []BackofficeCustomerBranch `json:"branches"`
}

func (r *Repository) ListBackofficeCustomers(ctx context.Context) ([]BackofficeCustomer, error) {
	rows, err := r.Pool.Query(ctx, `SELECT m.id,m.nombre,m.activo AND m.deleted_at IS NULL,m.created_at,
 COALESCE(s.program_type,p.tipo),s.estado,s.precio_sucursal_minor,COALESCE(s.full_unit_price_minor,s.precio_sucursal_minor),s.importe_mensual_minor,s.cantidad_sucursales,s.proximo_cobro_at,
 COALESCE(ch.paid_charges,0),COALESCE(ch.paid_amount_minor,0),COALESCE(ch.refunded_charges,0),ch.last_payment_at,
 rc.code,a.source_kind,CASE WHEN a.source_kind='INFLUENCER' THEN i.name ELSE source.nombre END,cp.name,
 COALESCE(a.discount_bps,0),COALESCE(a.discount_charges,0),
 greatest(COALESCE(a.discount_charges,0)-COALESCE(ch.total_charges,0),0),branches.items
 FROM marcas m
 LEFT JOIN LATERAL (SELECT tipo FROM programas_fidelidad WHERE marca_id=m.id ORDER BY activo DESC,created_at DESC,id DESC LIMIT 1) p ON true
 LEFT JOIN suscripciones_marca s ON s.marca_id=m.id
 LEFT JOIN referral_attributions a ON a.brand_id=m.id
 LEFT JOIN referral_codes rc ON rc.id=a.code_id
 LEFT JOIN referral_campaigns cp ON cp.id=a.campaign_id
 LEFT JOIN referral_influencers i ON i.id=a.influencer_id
 LEFT JOIN marcas source ON source.id=a.source_brand_id
 LEFT JOIN LATERAL (
   SELECT COALESCE(jsonb_agg(jsonb_build_object(
     'id',b.id,'name',b.nombre,'address',b.direccion,'locality',b.localidad,
     'province',b.provincia,'postal_code',b.codigo_postal,
     'active',b.activo AND b.deleted_at IS NULL,'primary',b.principal
   ) ORDER BY (b.activo AND b.deleted_at IS NULL) DESC,b.principal DESC,b.id),'[]'::jsonb) AS items
   FROM sucursales b WHERE b.marca_id=m.id
 ) branches ON true
 LEFT JOIN (
   SELECT brand_id,count(*) AS total_charges,
     count(*) FILTER (WHERE status='APPROVED' AND amount_minor>0) AS paid_charges,
     COALESCE(sum(amount_minor) FILTER (WHERE status='APPROVED' AND amount_minor>0),0)::bigint AS paid_amount_minor,
     count(*) FILTER (WHERE status='REFUNDED') AS refunded_charges,
     max(created_at) FILTER (WHERE status='APPROVED' AND amount_minor>0) AS last_payment_at
   FROM referral_charges GROUP BY brand_id
 ) ch ON ch.brand_id=m.id
 ORDER BY m.created_at DESC,m.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]BackofficeCustomer, 0)
	for rows.Next() {
		var x BackofficeCustomer
		if err := rows.Scan(&x.BrandID, &x.BrandName, &x.Active, &x.CreatedAt, &x.ProgramType,
			&x.SubscriptionStatus, &x.UnitPriceMinor, &x.FullUnitPriceMinor, &x.MonthlyAmountMinor, &x.SubscribedBranches, &x.NextPaymentAt, &x.PaidCharges, &x.PaidAmountMinor, &x.RefundedCharges, &x.LastPaymentAt,
			&x.ReferralCode, &x.SourceKind, &x.SourceName, &x.CampaignName,
			&x.DiscountBPS, &x.DiscountCharges, &x.DiscountRemainingCharges, &x.Branches); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
