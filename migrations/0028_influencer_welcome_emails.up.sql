-- Influencers need not have an app account. Keep identity mail ownership intact.
ALTER TABLE email_outbox ALTER COLUMN usuario_id DROP NOT NULL;
ALTER TABLE email_outbox
  ADD COLUMN influencer_id BIGINT REFERENCES referral_influencers(id),
  ADD COLUMN referral_code_id BIGINT REFERENCES referral_codes(id);
ALTER TABLE email_outbox DROP CONSTRAINT email_outbox_tipo_check;
ALTER TABLE email_outbox ADD CONSTRAINT email_outbox_tipo_check
  CHECK (tipo IN ('VERIFY_EMAIL','RESET_PASSWORD','BRAND_INVITATION','INFLUENCER_WELCOME'));
ALTER TABLE email_outbox ADD CONSTRAINT email_outbox_owner_check CHECK (
  (tipo='INFLUENCER_WELCOME' AND usuario_id IS NULL AND invitation_id IS NULL
    AND influencer_id IS NOT NULL AND referral_code_id IS NOT NULL)
  OR (tipo<>'INFLUENCER_WELCOME' AND usuario_id IS NOT NULL
    AND influencer_id IS NULL AND referral_code_id IS NULL)
);
CREATE UNIQUE INDEX email_outbox_influencer_welcome_uk ON email_outbox(referral_code_id)
  WHERE tipo='INFLUENCER_WELCOME';
