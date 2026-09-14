ALTER TABLE email_outbox
  ALTER COLUMN cuerpo_texto DROP NOT NULL,
  ALTER COLUMN cuerpo_html DROP NOT NULL,
  ADD COLUMN token_ciphertext BYTEA,
  ADD COLUMN token_nonce BYTEA,
  ADD COLUMN token_expires_at TIMESTAMPTZ,
  ADD COLUMN lease_owner UUID;

UPDATE email_outbox
SET estado='FAILED', cuerpo_texto=NULL, cuerpo_html=NULL,
    ultimo_error='legacy plaintext payload purged', lease_until=NULL
WHERE estado IN ('PENDING','SENDING');

UPDATE email_outbox SET cuerpo_texto=NULL,cuerpo_html=NULL WHERE cuerpo_texto IS NOT NULL OR cuerpo_html IS NOT NULL;

ALTER TABLE email_outbox
  ADD CONSTRAINT email_outbox_secure_payload_check CHECK (
    (estado IN ('PENDING','SENDING') AND token_ciphertext IS NOT NULL AND token_nonce IS NOT NULL AND token_expires_at IS NOT NULL) OR
    (estado IN ('SENT','FAILED') AND token_ciphertext IS NULL AND token_nonce IS NULL)
  ),
  ADD CONSTRAINT email_outbox_lease_owner_check CHECK (
    (estado='SENDING' AND lease_until IS NOT NULL AND lease_owner IS NOT NULL) OR
    (estado<>'SENDING' AND lease_until IS NULL AND lease_owner IS NULL)
  );
