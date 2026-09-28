package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

var ErrCampaignOverlap = fmt.Errorf("campaign overlap: %w", ErrConflict)
var ErrCampaignChanged = fmt.Errorf("campaign changed: %w", ErrConflict)

func normalizeReferralCampaign(x ReferralCampaign) (ReferralCampaign, error) {
	x.Name = strings.TrimSpace(x.Name)
	if x.ProgramTypes == nil {
		x.ProgramTypes = []string{x.ProgramType}
	}
	selected := map[string]bool{}
	for _, program := range x.ProgramTypes {
		if (program != "SELLOS" && program != "PUNTOS") || selected[program] {
			return x, ErrInvalidRequest
		}
		selected[program] = true
	}
	if len(selected) == 0 {
		return x, ErrInvalidRequest
	}
	x.ProgramTypes = []string{}
	for _, program := range []string{"SELLOS", "PUNTOS"} {
		if selected[program] {
			x.ProgramTypes = append(x.ProgramTypes, program)
		}
	}
	if x.ProgramType != "" && x.ProgramType != x.ProgramTypes[0] {
		return x, ErrInvalidRequest
	}
	x.ProgramType = x.ProgramTypes[0]
	if utf8.RuneCountInString(x.Name) < 3 || utf8.RuneCountInString(x.Name) > 120 || x.DiscountBPS < 0 || x.DiscountBPS > 9999 || x.RewardBPS < 0 || x.RewardBPS > 10000 || x.DiscountCharges < 0 || x.DiscountCharges > 36 || x.RewardCharges < 0 || x.RewardCharges > 36 || x.StartsAt.IsZero() || x.EndsAt.IsZero() || !x.EndsAt.After(x.StartsAt) {
		return x, ErrInvalidRequest
	}
	return x, nil
}

// Every campaign mutation uses the same lock before acquiring a row lock, so
// expanding to both programs cannot race a creation or activation for either.
func lockReferralCampaigns(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(42022,0)`)
	return err
}

func checkReferralCampaignOverlap(ctx context.Context, tx pgx.Tx, x ReferralCampaign) error {
	var overlaps bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM referral_campaigns
    WHERE id<>$1 AND active AND COALESCE(program_types,ARRAY[program_type]) && $2::text[]
    AND tstzrange(starts_at,ends_at,'[)') && tstzrange($3::timestamptz,$4::timestamptz,'[)'))`, x.ID, x.ProgramTypes, x.StartsAt, x.EndsAt).Scan(&overlaps)
	if err != nil {
		return err
	}
	if overlaps {
		return ErrCampaignOverlap
	}
	return nil
}

func generateReferralMerchantCodes(ctx context.Context, tx pgx.Tx, x ReferralCampaign) error {
	_, err := tx.Exec(ctx, `INSERT INTO referral_codes(code,campaign_id,source_kind,source_brand_id)
    SELECT DISTINCT 'PZ-M-'||m.id||'-'||($1::bigint)::text,$1::bigint,'MERCHANT',m.id
    FROM marcas m JOIN programas_fidelidad p ON p.marca_id=m.id
    WHERE m.activo AND m.deleted_at IS NULL AND p.activo AND p.tipo=ANY($2::text[])
    ON CONFLICT DO NOTHING`, x.ID, x.ProgramTypes)
	return err
}

func (r *Repository) CreateReferralCampaign(ctx context.Context, x ReferralCampaign, adminID int64) (ReferralCampaign, error) {
	var err error
	x, err = normalizeReferralCampaign(x)
	if err != nil {
		return x, err
	}
	x.ID = 0
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return x, err
	}
	defer tx.Rollback(ctx)
	if err = lockReferralCampaigns(ctx, tx); err != nil {
		return x, err
	}
	if err = checkReferralCampaignOverlap(ctx, tx, x); err != nil {
		return x, err
	}
	err = tx.QueryRow(ctx, `INSERT INTO referral_campaigns(name,program_type,program_types,discount_bps,discount_charges,reward_bps,reward_charges,starts_at,ends_at)
    VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id,active,version`, x.Name, x.ProgramType, x.ProgramTypes, x.DiscountBPS, x.DiscountCharges, x.RewardBPS, x.RewardCharges, x.StartsAt, x.EndsAt).Scan(&x.ID, &x.Active, &x.Version)
	if err != nil {
		return x, err
	}
	if err = generateReferralMerchantCodes(ctx, tx, x); err != nil {
		return x, err
	}
	if err = auditReferral(ctx, tx, adminID, "campaign.create", "campaign", fmt.Sprint(x.ID), x); err != nil {
		return x, err
	}
	return x, tx.Commit(ctx)
}

func referralCampaignForUpdate(ctx context.Context, tx pgx.Tx, id int64) (ReferralCampaign, error) {
	var x ReferralCampaign
	err := tx.QueryRow(ctx, `SELECT id,name,program_type,COALESCE(program_types,ARRAY[program_type]),discount_bps,discount_charges,reward_bps,reward_charges,starts_at,ends_at,active,version FROM referral_campaigns WHERE id=$1 FOR UPDATE`, id).
		Scan(&x.ID, &x.Name, &x.ProgramType, &x.ProgramTypes, &x.DiscountBPS, &x.DiscountCharges, &x.RewardBPS, &x.RewardCharges, &x.StartsAt, &x.EndsAt, &x.Active, &x.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return x, ErrNotFound
	}
	return x, err
}

func (r *Repository) UpdateReferralCampaign(ctx context.Context, id, adminID int64, x ReferralCampaign) (ReferralCampaign, error) {
	var err error
	x, err = normalizeReferralCampaign(x)
	if err != nil || x.Version < 1 {
		return x, ErrInvalidRequest
	}
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return x, err
	}
	defer tx.Rollback(ctx)
	if err = lockReferralCampaigns(ctx, tx); err != nil {
		return x, err
	}
	before, err := referralCampaignForUpdate(ctx, tx, id)
	if err != nil {
		return x, err
	}
	if x.Version != before.Version {
		return x, ErrCampaignChanged
	}
	x.ID = id
	x.Active = before.Active
	if x.Active {
		if err = checkReferralCampaignOverlap(ctx, tx, x); err != nil {
			return x, err
		}
	}
	err = tx.QueryRow(ctx, `UPDATE referral_campaigns SET name=$2,program_type=$3,program_types=$4,discount_bps=$5,discount_charges=$6,reward_bps=$7,reward_charges=$8,starts_at=$9,ends_at=$10,version=version+1 WHERE id=$1 RETURNING version`, id, x.Name, x.ProgramType, x.ProgramTypes, x.DiscountBPS, x.DiscountCharges, x.RewardBPS, x.RewardCharges, x.StartsAt, x.EndsAt).Scan(&x.Version)
	if err != nil {
		return x, err
	}
	if x.Active {
		if err = generateReferralMerchantCodes(ctx, tx, x); err != nil {
			return x, err
		}
	}
	if err = auditReferral(ctx, tx, adminID, "campaign.update", "campaign", fmt.Sprint(id), map[string]any{"before": before, "after": x}); err != nil {
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
	if err = lockReferralCampaigns(ctx, tx); err != nil {
		return err
	}
	x, err := referralCampaignForUpdate(ctx, tx, id)
	if err != nil {
		return err
	}
	if active {
		if err = checkReferralCampaignOverlap(ctx, tx, x); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE referral_campaigns SET active=$2,version=version+1 WHERE id=$1`, id, active); err != nil {
		return err
	}
	if active {
		if err = generateReferralMerchantCodes(ctx, tx, x); err != nil {
			return err
		}
	}
	if err = auditReferral(ctx, tx, adminID, "campaign.active", "campaign", fmt.Sprint(id), map[string]any{"active": active}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
