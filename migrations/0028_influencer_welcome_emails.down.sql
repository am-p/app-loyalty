-- Never discard mail or its delivery evidence to downgrade the schema.
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM email_outbox WHERE tipo='INFLUENCER_WELCOME') THEN
    RAISE EXCEPTION 'Cannot downgrade: influencer welcome emails exist; use a forward fix';
  END IF;
END $$;
DROP INDEX email_outbox_influencer_welcome_uk;
ALTER TABLE email_outbox DROP CONSTRAINT email_outbox_owner_check;
ALTER TABLE email_outbox DROP CONSTRAINT email_outbox_tipo_check;
ALTER TABLE email_outbox ADD CONSTRAINT email_outbox_tipo_check
  CHECK (tipo IN ('VERIFY_EMAIL','RESET_PASSWORD','BRAND_INVITATION'));
ALTER TABLE email_outbox DROP COLUMN influencer_id, DROP COLUMN referral_code_id;
ALTER TABLE email_outbox ALTER COLUMN usuario_id SET NOT NULL;
