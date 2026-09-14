DROP TABLE email_outbox;
DROP TABLE tokens_identidad_email;
ALTER TABLE usuarios DROP COLUMN auth_version,DROP COLUMN email_verified_at;
