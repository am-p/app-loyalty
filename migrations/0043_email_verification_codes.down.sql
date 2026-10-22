-- Cancel verification mail associated with code challenges; preserve other accounts.
UPDATE email_outbox SET estado='FAILED',ultimo_error='verification code migration rolled back',token_ciphertext=NULL,token_nonce=NULL,token_expires_at=NULL,lease_until=NULL,lease_owner=NULL WHERE tipo='VERIFY_EMAIL' AND estado IN ('PENDING','SENDING') AND usuario_id IN (SELECT usuario_id FROM email_verification_codes);
DROP TABLE email_verification_codes;
