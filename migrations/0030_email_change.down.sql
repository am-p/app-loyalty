DELETE FROM email_outbox WHERE tipo = 'CHANGE_EMAIL';
DELETE FROM tokens_identidad_email WHERE proposito = 'CHANGE_EMAIL';
ALTER TABLE email_outbox DROP CONSTRAINT email_outbox_tipo_check;
ALTER TABLE email_outbox
  ADD CONSTRAINT email_outbox_tipo_check CHECK (tipo IN ('VERIFY_EMAIL','RESET_PASSWORD','BRAND_INVITATION','INFLUENCER_WELCOME','SUBSCRIPTION_CONFIRMATION'));
ALTER TABLE tokens_identidad_email DROP CONSTRAINT tokens_identidad_email_pending_check;
ALTER TABLE tokens_identidad_email DROP CONSTRAINT tokens_identidad_email_proposito_check;
ALTER TABLE tokens_identidad_email
  ADD CONSTRAINT tokens_identidad_email_proposito_check CHECK (proposito IN ('VERIFY_EMAIL','RESET_PASSWORD'));
ALTER TABLE tokens_identidad_email DROP COLUMN pending_email, DROP COLUMN account_version;
