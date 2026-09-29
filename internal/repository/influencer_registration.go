package repository

import (
	"clientesFrecuentes/internal/model"
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"github.com/jackc/pgx/v5"
)

type ReferralInfluencerWithCode struct {
	ID          int64        `json:"id"`
	Name        string       `json:"name"`
	Contact     string       `json:"contact"`
	Email       string       `json:"email"`
	Code        ReferralCode `json:"code"`
	EmailQueued bool         `json:"email_queued"`
}

func normalizeInfluencerEmail(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	parsed, err := mail.ParseAddress(value)
	if err != nil || parsed.Address != value || len(value) > 200 {
		return "", ErrInvalidRequest
	}
	return value, nil
}

func createReferralInfluencerCode(ctx context.Context, tx pgx.Tx, code string, campaignID, influencerID, adminID int64) (ReferralCode, error) {
	code, err := NormalizeReferralCode(code)
	if err != nil || strings.HasPrefix(code, "PZ-M-") || campaignID < 1 || influencerID < 1 {
		return ReferralCode{}, ErrInvalidRequest
	}
	out := ReferralCode{Code: code, CampaignID: campaignID, SourceKind: "INFLUENCER", InfluencerID: &influencerID}
	if err = tx.QueryRow(ctx, `INSERT INTO referral_codes(code,campaign_id,source_kind,influencer_id) VALUES($1,$2,'INFLUENCER',$3) RETURNING id,active`, code, campaignID, influencerID).Scan(&out.ID, &out.Active); err != nil {
		if IsUniqueViolation(err) {
			return out, ErrConflict
		}
		return out, normalize(err)
	}
	if err = auditReferral(ctx, tx, adminID, "code.create", "code", fmt.Sprint(out.ID), out); err != nil {
		return out, err
	}
	return out, nil
}

// Register the influencer and first code together. Failed code creation leaves
// neither a profile nor audit entries behind. Contact retains the email for
// compatibility with the existing schema and contact-only API clients.
func (r *Repository) CreateReferralInfluencerWithCode(ctx context.Context, name, email, code string, campaignID, adminID int64) (ReferralInfluencerWithCode, error) {
	var out ReferralInfluencerWithCode
	name = strings.TrimSpace(name)
	email, err := normalizeInfluencerEmail(email)
	code, codeErr := NormalizeReferralCode(code)
	if err != nil || codeErr != nil || strings.HasPrefix(code, "PZ-M-") || len([]rune(name)) < 2 || len([]rune(name)) > 120 || campaignID < 1 {
		return out, ErrInvalidRequest
	}
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	var welcome model.InfluencerWelcomeDetails
	err = tx.QueryRow(ctx, `SELECT id,name,COALESCE(program_types,ARRAY[program_type]),discount_bps,discount_charges,reward_bps,reward_charges,starts_at,ends_at,active FROM referral_campaigns WHERE id=$1 FOR SHARE`, campaignID).
		Scan(&welcome.CampaignID, &welcome.CampaignName, &welcome.ProgramTypes, &welcome.DiscountBPS, &welcome.DiscountCharges, &welcome.RewardBPS, &welcome.RewardCharges, &welcome.StartsAt, &welcome.EndsAt, &welcome.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrInvalidRequest
	}
	if err != nil {
		return out, err
	}
	out.Name = name
	out.Email = email
	out.Contact = email
	if err = tx.QueryRow(ctx, `INSERT INTO referral_influencers(name,contact) VALUES($1,$2) RETURNING id`, name, email).Scan(&out.ID); err != nil {
		return out, err
	}
	out.Code, err = createReferralInfluencerCode(ctx, tx, code, campaignID, out.ID, adminID)
	if err != nil {
		return ReferralInfluencerWithCode{}, err
	}
	if err = auditReferral(ctx, tx, adminID, "influencer.create", "influencer", fmt.Sprint(out.ID)); err != nil {
		return ReferralInfluencerWithCode{}, err
	}
	welcome.InfluencerID, welcome.CodeID, welcome.Name, welcome.Code = out.ID, out.Code.ID, name, code
	if err = r.enqueueInfluencerWelcome(ctx, tx, email, welcome); err != nil {
		return ReferralInfluencerWithCode{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ReferralInfluencerWithCode{}, err
	}
	out.EmailQueued = true
	return out, nil
}
