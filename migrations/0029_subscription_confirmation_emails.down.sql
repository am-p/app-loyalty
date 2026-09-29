DELETE FROM email_outbox WHERE tipo='SUBSCRIPTION_CONFIRMATION';
ALTER TABLE email_outbox DROP CONSTRAINT email_outbox_subscription_check;
ALTER TABLE email_outbox DROP CONSTRAINT email_outbox_tipo_check;
ALTER TABLE email_outbox ADD CONSTRAINT email_outbox_tipo_check CHECK
  (tipo IN ('VERIFY_EMAIL','RESET_PASSWORD','BRAND_INVITATION','INFLUENCER_WELCOME'));
DROP INDEX email_outbox_subscription_confirmation_uk;
ALTER TABLE email_outbox DROP COLUMN subscription_brand_id, DROP COLUMN subscription_provider_id;
