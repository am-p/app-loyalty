ALTER TABLE email_outbox
  DROP CONSTRAINT email_outbox_lease_owner_check,
  DROP CONSTRAINT email_outbox_secure_payload_check,
  DROP COLUMN lease_owner,
  DROP COLUMN token_expires_at,
  DROP COLUMN token_nonce,
  DROP COLUMN token_ciphertext;

UPDATE email_outbox SET cuerpo_texto='',cuerpo_html='' WHERE cuerpo_texto IS NULL OR cuerpo_html IS NULL;
ALTER TABLE email_outbox ALTER COLUMN cuerpo_texto SET NOT NULL,ALTER COLUMN cuerpo_html SET NOT NULL;

