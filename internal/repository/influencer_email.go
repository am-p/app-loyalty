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

var ErrEmailUnavailable = errors.New("email queue encryption is unavailable")

func (r *Repository) enqueueInfluencerWelcome(ctx context.Context, tx pgx.Tx, to string, details model.InfluencerWelcomeDetails) error {
	if len(r.OutboxCipherKey) != 32 {
		return ErrEmailUnavailable
	}
	payload, err := json.Marshal(details)
	if err != nil {
		return err
	}
	id := uuid.NewString()
	encrypted, nonce, err := encryptOutboxToken(string(payload), id, r.OutboxCipherKey)
	if err != nil {
		return err
	}
	// Legacy token columns also carry this encrypted JSON snapshot. No identity
	// token or fake user is created. Seven days is the delivery deadline only.
	_, err = tx.Exec(ctx, `INSERT INTO email_outbox(id,tipo,destinatario,asunto,influencer_id,referral_code_id,token_ciphertext,token_nonce,token_expires_at)
 VALUES($1,'INFLUENCER_WELCOME',$2,'Tu código y beneficios en Puntazo',$3,$4,$5,$6,$7)`, id, to, details.InfluencerID, details.CodeID, encrypted, nonce, r.Now().Add(7*24*time.Hour))
	return err
}

func (r *Repository) validateInfluencerWelcome(ctx context.Context, item model.OutboxEmail, payload string) error {
	var details model.InfluencerWelcomeDetails
	if err := json.Unmarshal([]byte(payload), &details); err != nil {
		return errors.New("invalid influencer email payload")
	}
	var valid bool
	err := r.Pool.QueryRow(ctx, `SELECT EXISTS(
 SELECT 1 FROM email_outbox e JOIN referral_codes c ON c.id=e.referral_code_id
 JOIN referral_influencers i ON i.id=e.influencer_id
 WHERE e.id=$1 AND e.tipo='INFLUENCER_WELCOME' AND e.estado='SENDING' AND e.lease_owner=$2
 AND e.destinatario=$3 AND i.contact=$3 AND e.influencer_id=$4 AND e.referral_code_id=$5
 AND c.influencer_id=i.id AND c.source_kind='INFLUENCER' AND c.code=$6 AND c.campaign_id=$7 AND c.active
 )`, item.ID, item.LeaseOwner, item.To, details.InfluencerID, details.CodeID, details.Code, details.CampaignID).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return errors.New("influencer email correlation is invalid or code is disabled")
	}
	return nil
}
