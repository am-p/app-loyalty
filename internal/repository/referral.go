package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ReferralValidation is the only referral information exposed publicly.
type ReferralValidation struct {
	Code        string `json:"code"`
	ProgramType string `json:"program_type"`
}

var referralCodePattern = regexp.MustCompile(`^[A-Z0-9-]{4,40}$`)

func NormalizeReferralCode(raw string) (string, error) {
	code := strings.ToUpper(strings.TrimSpace(raw))
	if !referralCodePattern.MatchString(code) {
		return "", ErrReferralCodeInvalid
	}
	return code, nil
}

// ValidateReferralCode only reads eligibility. Registration must still call
// AttributeReferral in its own transaction, since campaigns can change later.
func (r *Repository) ValidateReferralCode(ctx context.Context, rawCode, programType string) (ReferralValidation, error) {
	code, err := NormalizeReferralCode(rawCode)
	if err != nil {
		return ReferralValidation{}, err
	}
	var out ReferralValidation
	err = r.Pool.QueryRow(ctx, `SELECT rc.code,CASE WHEN $2='' THEN cp.program_type ELSE $2 END
 FROM referral_codes rc JOIN referral_campaigns cp ON cp.id=rc.campaign_id
 WHERE rc.code=$1 AND rc.active AND cp.active
 AND ($2='' OR $2=ANY(COALESCE(cp.program_types,ARRAY[cp.program_type]))) AND now()>=cp.starts_at AND now()<cp.ends_at`, code, programType).Scan(&out.Code, &out.ProgramType)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReferralValidation{}, ErrReferralCodeInvalid
	}
	return out, err
}

type ReferralCampaign struct {
	ID              int64     `json:"id"`
	Name            string    `json:"name"`
	ProgramType     string    `json:"program_type"`
	ProgramTypes    []string  `json:"program_types"`
	Version         int64     `json:"version"`
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
	 (SELECT count(DISTINCT c.brand_id) FROM referral_charges c JOIN referral_attributions a ON a.brand_id=c.brand_id WHERE c.status='APPROVED'),
	 (SELECT count(*) FROM (SELECT c.brand_id FROM referral_charges c JOIN referral_attributions a ON a.brand_id=c.brand_id WHERE c.status='APPROVED' GROUP BY c.brand_id HAVING count(*)>=2) q),
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
		if _, err := NormalizeReferralCode(code); err != nil {
			return err
		}
		var c ReferralCode
		var discountBPS, discountCharges, rewardBPS, rewardCharges int
		err := tx.QueryRow(ctx, `SELECT rc.id,rc.campaign_id,rc.source_kind,rc.influencer_id,rc.source_brand_id,
   cp.discount_bps,cp.discount_charges,cp.reward_bps,cp.reward_charges
   FROM referral_codes rc JOIN referral_campaigns cp ON cp.id=rc.campaign_id
   WHERE rc.code=$1 AND rc.active AND cp.active AND $2=ANY(COALESCE(cp.program_types,ARRAY[cp.program_type])) AND
   now()>=cp.starts_at AND now()<cp.ends_at FOR SHARE OF rc,cp`, code, programType).
			Scan(&c.ID, &c.CampaignID, &c.SourceKind, &c.InfluencerID, &c.SourceBrandID, &discountBPS, &discountCharges, &rewardBPS, &rewardCharges)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrReferralCodeInvalid
		}
		if err != nil {
			return err
		}
		if c.SourceBrandID != nil && *c.SourceBrandID == brandID {
			return ErrReferralCodeInvalid
		}
		_, err = tx.Exec(ctx, `INSERT INTO referral_attributions(brand_id,code_id,campaign_id,source_kind,influencer_id,source_brand_id,discount_bps,discount_charges,reward_bps,reward_charges)
   VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, brandID, c.ID, c.CampaignID, c.SourceKind, c.InfluencerID, c.SourceBrandID, discountBPS, discountCharges, rewardBPS, rewardCharges)
		if err != nil {
			return normalize(err)
		}
	}
	// Every new brand receives a shareable merchant code for each open campaign.
	rows, err := tx.Query(ctx, `SELECT id FROM referral_campaigns WHERE $1=ANY(COALESCE(program_types,ARRAY[program_type])) AND active AND now()>=starts_at AND now()<ends_at`, programType)
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
	rows, err := r.Pool.Query(ctx, `SELECT id,name,program_type,COALESCE(program_types,ARRAY[program_type]),version,discount_bps,discount_charges,reward_bps,reward_charges,starts_at,ends_at,active FROM referral_campaigns ORDER BY id DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ReferralCampaign{}
	for rows.Next() {
		var x ReferralCampaign
		if err = rows.Scan(&x.ID, &x.Name, &x.ProgramType, &x.ProgramTypes, &x.Version, &x.DiscountBPS, &x.DiscountCharges, &x.RewardBPS, &x.RewardCharges, &x.StartsAt, &x.EndsAt, &x.Active); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
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
		var email any
		if value, err := normalizeInfluencerEmail(contact); err == nil {
			email = value
		}
		out = append(out, map[string]any{"id": id, "name": name, "contact": contact, "email": email})
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
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(code)), "PZ-M-") {
		return ReferralCode{}, ErrInvalidRequest
	}
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return ReferralCode{}, err
	}
	defer tx.Rollback(ctx)
	out, err := createReferralInfluencerCode(ctx, tx, code, campaignID, influencerID, adminID)
	if err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
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
