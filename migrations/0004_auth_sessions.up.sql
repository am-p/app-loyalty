CREATE TABLE sesiones_auth (
  id UUID PRIMARY KEY,
  usuario_id BIGINT NOT NULL REFERENCES usuarios(id) ON DELETE CASCADE,
  refresh_hash BYTEA NOT NULL UNIQUE,
  expires_at TIMESTAMPTZ NOT NULL,
  revoked_at TIMESTAMPTZ,
  replaced_by UUID REFERENCES sesiones_auth(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_used_at TIMESTAMPTZ,
  CHECK (expires_at > created_at)
);

CREATE INDEX sesiones_auth_usuario_activa_idx
  ON sesiones_auth(usuario_id,expires_at) WHERE revoked_at IS NULL;
