ALTER TABLE email_outbox
  ADD COLUMN subscription_brand_id BIGINT REFERENCES marcas(id),
  ADD COLUMN subscription_provider_id TEXT;
ALTER TABLE email_outbox DROP CONSTRAINT email_outbox_tipo_check;
ALTER TABLE email_outbox ADD CONSTRAINT email_outbox_tipo_check CHECK
  (tipo IN ('VERIFY_EMAIL','RESET_PASSWORD','BRAND_INVITATION','INFLUENCER_WELCOME','SUBSCRIPTION_CONFIRMATION'));
ALTER TABLE email_outbox ADD CONSTRAINT email_outbox_subscription_check CHECK (
  (tipo='SUBSCRIPTION_CONFIRMATION' AND usuario_id IS NOT NULL AND invitation_id IS NULL
    AND subscription_brand_id IS NOT NULL AND subscription_provider_id IS NOT NULL)
  OR (tipo<>'SUBSCRIPTION_CONFIRMATION' AND subscription_brand_id IS NULL AND subscription_provider_id IS NULL)
);
CREATE UNIQUE INDEX email_outbox_subscription_confirmation_uk ON email_outbox(subscription_provider_id)
  WHERE tipo='SUBSCRIPTION_CONFIRMATION';
