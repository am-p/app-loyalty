-- 0036-0042 are reserved by testing migrations; do not renumber applied history.
CREATE TABLE email_verification_codes (
 usuario_id BIGINT PRIMARY KEY REFERENCES usuarios(id) ON DELETE CASCADE,
 code_hash BYTEA NOT NULL CHECK(octet_length(code_hash)=32),
 expires_at TIMESTAMPTZ NOT NULL,
 attempts INTEGER NOT NULL DEFAULT 0 CHECK(attempts BETWEEN 0 AND 5),
 sent_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 consumed_at TIMESTAMPTZ,
 registration_hash BYTEA CHECK(registration_hash IS NULL OR octet_length(registration_hash)=32),
 registration_expires_at TIMESTAMPTZ
);
