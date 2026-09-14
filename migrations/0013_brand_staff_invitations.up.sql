ALTER TABLE membresias_marca
  ADD COLUMN version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
  ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE TABLE invitaciones_marca (
  id UUID PRIMARY KEY,
  marca_id BIGINT NOT NULL REFERENCES marcas(id) ON DELETE RESTRICT,
  invitado_por BIGINT NOT NULL REFERENCES usuarios(id) ON DELETE RESTRICT,
  email CITEXT NOT NULL,
  rol TEXT NOT NULL CHECK (rol IN ('ADMINISTRADOR','OPERADOR')),
  token_hash BYTEA NOT NULL UNIQUE CHECK (octet_length(token_hash)=32),
  estado TEXT NOT NULL DEFAULT 'PENDIENTE' CHECK (estado IN ('PENDIENTE','ACEPTADA','REVOCADA','EXPIRADA')),
  expires_at TIMESTAMPTZ NOT NULL,
  accepted_by BIGINT REFERENCES usuarios(id) ON DELETE RESTRICT,
  accepted_at TIMESTAMPTZ,
  version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (expires_at > created_at),
  CHECK ((estado='ACEPTADA' AND accepted_by IS NOT NULL AND accepted_at IS NOT NULL) OR
         (estado<>'ACEPTADA' AND accepted_by IS NULL AND accepted_at IS NULL))
);
CREATE UNIQUE INDEX invitaciones_marca_email_pendiente_uk
  ON invitaciones_marca(marca_id,email) WHERE estado='PENDIENTE';
CREATE INDEX invitaciones_marca_listado_idx ON invitaciones_marca(marca_id,created_at DESC);

CREATE TABLE invitaciones_sucursales (
  invitacion_id UUID NOT NULL REFERENCES invitaciones_marca(id) ON DELETE CASCADE,
  sucursal_id BIGINT NOT NULL,
  marca_id BIGINT NOT NULL,
  PRIMARY KEY (invitacion_id,sucursal_id),
  FOREIGN KEY (sucursal_id,marca_id) REFERENCES sucursales(id,marca_id) ON DELETE RESTRICT
);

ALTER TABLE email_outbox DROP CONSTRAINT email_outbox_tipo_check;
ALTER TABLE email_outbox ADD CONSTRAINT email_outbox_tipo_check
  CHECK (tipo IN ('VERIFY_EMAIL','RESET_PASSWORD','BRAND_INVITATION'));

