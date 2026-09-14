ALTER TABLE usuarios
  ADD COLUMN email_verified_at TIMESTAMPTZ,
  ADD COLUMN auth_version INTEGER NOT NULL DEFAULT 1 CHECK (auth_version > 0);

UPDATE usuarios SET email_verified_at=created_at WHERE email_verified_at IS NULL;

CREATE TABLE tokens_identidad_email (
  id UUID PRIMARY KEY,
  usuario_id BIGINT NOT NULL REFERENCES usuarios(id) ON DELETE CASCADE,
  proposito TEXT NOT NULL CHECK (proposito IN ('VERIFY_EMAIL','RESET_PASSWORD')),
  token_hash BYTEA NOT NULL UNIQUE,
  expires_at TIMESTAMPTZ NOT NULL,
  consumed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (expires_at > created_at)
);
CREATE INDEX tokens_identidad_email_usuario_proposito_idx ON tokens_identidad_email(usuario_id,proposito,created_at DESC);
CREATE INDEX tokens_identidad_email_expiracion_idx ON tokens_identidad_email(expires_at) WHERE consumed_at IS NULL;

CREATE TABLE email_outbox (
  id UUID PRIMARY KEY,
  usuario_id BIGINT NOT NULL REFERENCES usuarios(id) ON DELETE CASCADE,
  tipo TEXT NOT NULL CHECK (tipo IN ('VERIFY_EMAIL','RESET_PASSWORD')),
  destinatario CITEXT NOT NULL,
  asunto VARCHAR(200) NOT NULL,
  cuerpo_texto TEXT NOT NULL,
  cuerpo_html TEXT NOT NULL,
  estado TEXT NOT NULL DEFAULT 'PENDING' CHECK (estado IN ('PENDING','SENDING','SENT','FAILED')),
  intentos INTEGER NOT NULL DEFAULT 0 CHECK (intentos >= 0),
  disponible_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  lease_until TIMESTAMPTZ,
  ultimo_error TEXT,
  sent_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK ((estado='SENT' AND sent_at IS NOT NULL) OR (estado<>'SENT' AND sent_at IS NULL))
);
CREATE INDEX email_outbox_pendientes_idx ON email_outbox(disponible_at,created_at) WHERE estado IN ('PENDING','SENDING');
CREATE INDEX email_outbox_usuario_tipo_idx ON email_outbox(usuario_id,tipo,created_at DESC);
