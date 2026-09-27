package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type ReferralCampaign struct {
	ID              int64     `json:"id"`
	Name            string    `json:"name"`
	ProgramType     string    `json:"program_type"`
	DiscountBPS     int       `json:"discount_bps"`
	DiscountCharges int       `json:"discount_charges"`
	RewardBPS       int       `json:"reward_bps"`
	RewardCharges   int       `json:"reward_charges"`
	StartsAt        time.Time `json:"starts_at"`
	EndsAt          time.Time `json:"ends_at"`
	Active          bool      `json:"active"`
}

type ReferralCode struct {
	ID            int64  `json:"id"`
	Code          string `json:"code"`
	CampaignID    int64  `json:"campaign_id"`
	SourceKind    string `json:"source_kind"`
	InfluencerID  *int64 `json:"influencer_id"`
	SourceBrandID *int64 `json:"source_brand_id"`
	Active        bool   `json:"active"`
}

type ReferralMetrics struct {
	AttributedBrands       int64 `json:"attributed_brands"`
	PayingBrands           int64 `json:"paying_brands"`
	ContinuingBrands       int64 `json:"continuing_brands"`
	FirstFullPriceBrands   int64 `json:"first_full_price_brands"`
	MerchantReferralBrands int64 `json:"merchant_referral_brands"`
	DiscountCostMinor      int64 `json:"discount_cost_minor"`
	RewardCostMinor        int64 `json:"reward_cost_minor"`
}

func (r *Repository) ReferralMetrics(ctx context.Context) (ReferralMetrics, error) {
	var x ReferralMetrics
	err := r.Pool.QueryRow(ctx, `SELECT
	 (SELECT count(*) FROM referral_attributions),
	 (SELECT count(DISTINCT brand_id) FROM referral_charges WHERE status='APPROVED'),
	 (SELECT count(*) FROM (SELECT brand_id FROM referral_charges WHERE status='APPROVED' GROUP BY brand_id HAVING count(*)>=2) q),
	 (SELECT count(DISTINCT c.brand_id) FROM referral_charges c JOIN referral_attributions a ON a.brand_id=c.brand_id WHERE c.status='APPROVED' AND c.paid_index>a.discount_charges),
	 (SELECT count(*) FROM referral_attributions WHERE source_kind='MERCHANT'),
	 (SELECT COALESCE(sum(greatest(full_amount_minor-amount_minor,0)),0) FROM referral_charges WHERE status='APPROVED'),
	 (SELECT COALESCE(sum(amount_minor),0) FROM referral_rewards WHERE status IN ('PENDING','SETTLED'))`).Scan(&x.AttributedBrands, &x.PayingBrands, &x.ContinuingBrands, &x.FirstFullPriceBrands, &x.MerchantReferralBrands, &x.DiscountCostMinor, &x.RewardCostMinor)
	return x, err
}

// AttributeReferral runs inside the brand creation transaction. The brand row
// and attribution either both commit or both roll back.
func AttributeReferral(ctx context.Context, tx pgx.Tx, brandID int64, programType, rawCode string) error {
	code := strings.ToUpper(strings.TrimSpace(rawCode))
	if code != "" {
		var c ReferralCode
		var discountBPS, discountCharges, rewardBPS, rewardCharges int
		err := tx.QueryRow(ctx, `SELECT rc.id,rc.campaign_id,rc.source_kind,rc.influencer_id,rc.source_brand_id,
   cp.discount_bps,cp.discount_charges,cp.reward_bps,cp.reward_charges
   FROM referral_codes rc JOIN referral_campaigns cp ON cp.id=rc.campaign_id
   WHERE rc.code=$1 AND rc.active AND cp.active AND cp.program_type=$2 AND
   now()>=cp.starts_at AND now()<cp.ends_at FOR SHARE OF rc,cp`, code, programType).
			Scan(&c.ID, &c.CampaignID, &c.SourceKind, &c.InfluencerID, &c.SourceBrandID, &discountBPS, &discountCharges, &rewardBPS, &rewardCharges)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidRequest
		}
		if err != nil {
			return err
		}
		if c.SourceBrandID != nil && *c.SourceBrandID == brandID {
			return ErrInvalidRequest
		}
		_, err = tx.Exec(ctx, `INSERT INTO referral_attributions(brand_id,code_id,campaign_id,source_kind,influencer_id,source_brand_id,discount_bps,discount_charges,reward_bps,reward_charges)
   VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, brandID, c.ID, c.CampaignID, c.SourceKind, c.InfluencerID, c.SourceBrandID, discountBPS, discountCharges, rewardBPS, rewardCharges)
		if err != nil {
			return normalize(err)
		}
	}
	// Every new brand receives a shareable merchant code for each open campaign.
	rows, err := tx.Query(ctx, `SELECT id FROM referral_campaigns WHERE program_type=$1 AND active AND now()>=starts_at AND now()<ends_at`, programType)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		code := fmt.Sprintf("PZ-M-%d-%d", brandID, id)
		if _, err = tx.Exec(ctx, `INSERT INTO referral_codes(code,campaign_id,source_kind,source_brand_id) VALUES($1,$2,'MERCHANT',$3) ON CONFLICT DO NOTHING`, code, id, brandID); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) ListReferralCampaigns(ctx context.Context) ([]ReferralCampaign, error) {
	rows, err := r.Pool.Query(ctx, `SELECT id,name,program_type,discount_bps,discount_charges,reward_bps,reward_charges,starts_at,ends_at,active FROM referral_campaigns ORDER BY id DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ReferralCampaign{}
	for rows.Next() {
		var x ReferralCampaign
		if err = rows.Scan(&x.ID, &x.Name, &x.ProgramType, &x.DiscountBPS, &x.DiscountCharges, &x.RewardBPS, &x.RewardCharges, &x.StartsAt, &x.EndsAt, &x.Active); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (r *Repository) CreateReferralCampaign(ctx context.Context, x ReferralCampaign, adminID int64) (ReferralCampaign, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return x, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(42022,hashtext($1))`, x.ProgramType); err != nil {
		return x, err
	}
	var overlaps bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM referral_campaigns WHERE program_type=$1 AND active AND tstzrange(starts_at,ends_at,'[)') && tstzrange($2::timestamptz,$3::timestamptz,'[)'))`, x.ProgramType, x.StartsAt, x.EndsAt).Scan(&overlaps); err != nil {
		return x, err
	}
	if overlaps {
		return x, ErrConflict
	}
	err = tx.QueryRow(ctx, `INSERT INTO referral_campaigns(name,program_type,discount_bps,discount_charges,reward_bps,reward_charges,starts_at,ends_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id,active`, x.Name, x.ProgramType, x.DiscountBPS, x.DiscountCharges, x.RewardBPS, x.RewardCharges, x.StartsAt, x.EndsAt).Scan(&x.ID, &x.Active)
	if err != nil {
		return x, err
	}
	// Existing brands can refer merchants as soon as the campaign opens.
	_, err = tx.Exec(ctx, `INSERT INTO referral_codes(code,campaign_id,source_kind,source_brand_id)
 SELECT 'PZ-M-'||m.id||'-'||($1::bigint)::text,$1::bigint,'MERCHANT',m.id FROM marcas m JOIN programas_fidelidad p ON p.marca_id=m.id
 WHERE m.activo AND p.activo AND p.tipo=$2 ON CONFLICT DO NOTHING`, x.ID, x.ProgramType)
	if err != nil {
		return x, err
	}
	if err = auditReferral(ctx, tx, adminID, "campaign.create", "campaign", fmt.Sprint(x.ID), x); err != nil {
		return x, err
	}
	return x, tx.Commit(ctx)
}

func (r *Repository) SetReferralCampaignActive(ctx context.Context, id, adminID int64, active bool) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if active {
		var program string
		var starts, ends time.Time
		err = tx.QueryRow(ctx, `SELECT program_type,starts_at,ends_at FROM referral_campaigns WHERE id=$1`, id).Scan(&program, &starts, &ends)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(42022,hashtext($1))`, program); err != nil {
			return err
		}
		var overlaps bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM referral_campaigns WHERE id<>$1 AND program_type=$2 AND active AND tstzrange(starts_at,ends_at,'[)') && tstzrange($3::timestamptz,$4::timestamptz,'[)'))`, id, program, starts, ends).Scan(&overlaps); err != nil {
			return err
		}
		if overlaps {
			return ErrConflict
		}
	}
	tag, err := tx.Exec(ctx, `UPDATE referral_campaigns SET active=$2 WHERE id=$1`, id, active)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if active {
		// Brands created while this campaign was paused need a code when it reopens.
		_, err = tx.Exec(ctx, `INSERT INTO referral_codes(code,campaign_id,source_kind,source_brand_id)
 SELECT 'PZ-M-'||m.id||'-'||c.id,c.id,'MERCHANT',m.id
 FROM referral_campaigns c JOIN programas_fidelidad p ON p.tipo=c.program_type
 JOIN marcas m ON m.id=p.marca_id
 WHERE c.id=$1 AND m.activo AND p.activo ON CONFLICT DO NOTHING`, id)
		if err != nil {
			return err
		}
	}
	if err = auditReferral(ctx, tx, adminID, "campaign.active", "campaign", fmt.Sprint(id), map[string]any{"active": active}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) CreateReferralInfluencer(ctx context.Context, name, contact string, adminID int64) (int64, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	var id int64
	err = tx.QueryRow(ctx, `INSERT INTO referral_influencers(name,contact) VALUES($1,$2) RETURNING id`, name, contact).Scan(&id)
	if err != nil {
		return 0, err
	}
	if err = auditReferral(ctx, tx, adminID, "influencer.create", "influencer", fmt.Sprint(id)); err != nil {
		return 0, err
	}
	return id, tx.Commit(ctx)
}

func (r *Repository) ListReferralInfluencers(ctx context.Context) ([]map[string]any, error) {
	rows, err := r.Pool.Query(ctx, `SELECT id,name,contact FROM referral_influencers ORDER BY id DESC LIMIT 500`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var name, contact string
		if err = rows.Scan(&id, &name, &contact); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": id, "name": name, "contact": contact})
	}
	return out, rows.Err()
}

func (r *Repository) ListReferralCodes(ctx context.Context) ([]ReferralCode, error) {
	rows, err := r.Pool.Query(ctx, `SELECT id,code,campaign_id,source_kind,influencer_id,source_brand_id,active FROM referral_codes ORDER BY id DESC LIMIT 1000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ReferralCode{}
	for rows.Next() {
		var x ReferralCode
		if err = rows.Scan(&x.ID, &x.Code, &x.CampaignID, &x.SourceKind, &x.InfluencerID, &x.SourceBrandID, &x.Active); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (r *Repository) MerchantReferralCodes(ctx context.Context, actorID, brandID int64) ([]ReferralCode, error) {
	var allowed bool
	if err := r.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM membresias_marca mm JOIN marcas m ON m.id=mm.marca_id WHERE mm.usuario_id=$1 AND mm.marca_id=$2 AND mm.activo AND mm.rol IN ('PROPIETARIO','ADMINISTRADOR') AND m.activo)`, actorID, brandID).Scan(&allowed); err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrForbidden
	}
	rows, err := r.Pool.Query(ctx, `SELECT rc.id,rc.code,rc.campaign_id,rc.source_kind,rc.influencer_id,rc.source_brand_id,rc.active FROM referral_codes rc JOIN referral_campaigns cp ON cp.id=rc.campaign_id WHERE rc.source_brand_id=$1 AND rc.source_kind='MERCHANT' AND rc.active AND cp.active AND now()>=cp.starts_at AND now()<cp.ends_at ORDER BY cp.ends_at LIMIT 10`, brandID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ReferralCode{}
	for rows.Next() {
		var x ReferralCode
		if err = rows.Scan(&x.ID, &x.Code, &x.CampaignID, &x.SourceKind, &x.InfluencerID, &x.SourceBrandID, &x.Active); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (r *Repository) CreateReferralInfluencerCode(ctx context.Context, code string, campaignID, influencerID, adminID int64) (ReferralCode, error) {
	if strings.HasPrefix(code, "PZ-M-") {
		return ReferralCode{}, ErrInvalidRequest
	}
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return ReferralCode{}, err
	}
	defer tx.Rollback(ctx)
	x := ReferralCode{Code: code, CampaignID: campaignID, SourceKind: "INFLUENCER", InfluencerID: &influencerID}
	err = tx.QueryRow(ctx, `INSERT INTO referral_codes(code,campaign_id,source_kind,influencer_id) VALUES($1,$2,'INFLUENCER',$3) RETURNING id,active`, code, campaignID, influencerID).Scan(&x.ID, &x.Active)
	if err != nil {
		return x, normalize(err)
	}
	if err = auditReferral(ctx, tx, adminID, "code.create", "code", fmt.Sprint(x.ID), x); err != nil {
		return x, err
	}
	return x, tx.Commit(ctx)
}

func (r *Repository) SetReferralCodeActive(ctx context.Context, id, adminID int64, active bool) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE referral_codes SET active=$2 WHERE id=$1`, id, active)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if err = auditReferral(ctx, tx, adminID, "code.active", "code", fmt.Sprint(id), map[string]any{"active": active}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) ListReferralAttributions(ctx context.Context) ([]map[string]any, error) {
	rows, err := r.Pool.Query(ctx, `SELECT a.brand_id,m.nombre,c.code,a.source_kind,a.discount_bps,a.discount_charges,a.reward_bps,a.reward_charges,a.created_at FROM referral_attributions a JOIN marcas m ON m.id=a.brand_id JOIN referral_codes c ON c.id=a.code_id ORDER BY a.created_at DESC LIMIT 1000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var brand, code, source string
		var db, dc, rb, rc int
		var created time.Time
		if err = rows.Scan(&id, &brand, &code, &source, &db, &dc, &rb, &rc, &created); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"brand_id": id, "brand_name": brand, "code": code, "source_kind": source, "discount_bps": db, "discount_charges": dc, "reward_bps": rb, "reward_charges": rc, "created_at": created})
	}
	return out, rows.Err()
}

func (r *Repository) ListReferralRewards(ctx context.Context) ([]map[string]any, error) {
	rows, err := r.Pool.Query(ctx, `SELECT provider_invoice_id,brand_id,source_kind,influencer_id,source_brand_id,amount_minor,status,created_at FROM referral_rewards ORDER BY created_at DESC LIMIT 1000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var invoice, source, status string
		var brand, amount int64
		var influencerID, sourceBrandID *int64
		var created time.Time
		if err = rows.Scan(&invoice, &brand, &source, &influencerID, &sourceBrandID, &amount, &status, &created); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"invoice_id": invoice, "brand_id": brand, "source_kind": source, "influencer_id": influencerID, "source_brand_id": sourceBrandID, "amount_minor": amount, "status": status, "created_at": created})
	}
	return out, rows.Err()
}

func auditReferral(ctx context.Context, tx pgx.Tx, adminID int64, action, kind, id string, detail ...any) error {
	value := any(map[string]any{})
	if len(detail) > 0 {
		value = detail[0]
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO backoffice_audit(user_id,action,object_type,object_id,detail) VALUES($1,$2,$3,$4,$5::jsonb)`, adminID, action, kind, id, string(encoded))
	return err
}
