ALTER TABLE tokens_identidad_email
  ADD COLUMN pending_email CITEXT,
  ADD COLUMN account_version INTEGER;
ALTER TABLE tokens_identidad_email DROP CONSTRAINT tokens_identidad_email_proposito_check;
ALTER TABLE tokens_identidad_email
  ADD CONSTRAINT tokens_identidad_email_proposito_check CHECK (proposito IN ('VERIFY_EMAIL','RESET_PASSWORD','CHANGE_EMAIL'));
ALTER TABLE tokens_identidad_email
  ADD CONSTRAINT tokens_identidad_email_pending_check CHECK ((proposito = 'CHANGE_EMAIL') = (pending_email IS NOT NULL) AND (proposito = 'CHANGE_EMAIL') = (account_version IS NOT NULL));

ALTER TABLE email_outbox DROP CONSTRAINT email_outbox_tipo_check;
ALTER TABLE email_outbox
  ADD CONSTRAINT email_outbox_tipo_check CHECK (tipo IN ('VERIFY_EMAIL','RESET_PASSWORD','BRAND_INVITATION','CHANGE_EMAIL'));
