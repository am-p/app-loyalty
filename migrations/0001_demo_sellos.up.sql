CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE usuarios (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  email CITEXT NOT NULL UNIQUE,
  password_hash TEXT,
  google_id TEXT UNIQUE,
  nombre VARCHAR(120) NOT NULL CHECK (btrim(nombre) <> ''),
  tipo_cuenta TEXT NOT NULL CHECK (tipo_cuenta IN ('CLIENTE_FINAL','PERSONAL_MARCA')),
  qr_hash BYTEA UNIQUE,
  activo BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ,
  CHECK ((tipo_cuenta = 'CLIENTE_FINAL' AND qr_hash IS NOT NULL) OR (tipo_cuenta = 'PERSONAL_MARCA' AND qr_hash IS NULL))
);

CREATE TABLE marcas (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  nombre VARCHAR(120) NOT NULL CHECK (btrim(nombre) <> ''),
  activo BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ
);

CREATE TABLE membresias_marca (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  usuario_id BIGINT NOT NULL REFERENCES usuarios(id) ON DELETE RESTRICT,
  marca_id BIGINT NOT NULL REFERENCES marcas(id) ON DELETE RESTRICT,
  rol TEXT NOT NULL CHECK (rol = 'PROPIETARIO'),
  activo BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (usuario_id, marca_id)
);

CREATE INDEX membresias_marca_marca_rol_activo_idx ON membresias_marca(marca_id, rol) WHERE activo;

CREATE TABLE sucursales (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  marca_id BIGINT NOT NULL REFERENCES marcas(id) ON DELETE RESTRICT,
  nombre VARCHAR(120) NOT NULL CHECK (btrim(nombre) <> ''),
  direccion VARCHAR(240),
  activo BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ
);
CREATE INDEX sucursales_marca_activo_idx ON sucursales(marca_id) WHERE activo;

CREATE TABLE membresias_sucursales (
  membresia_id BIGINT NOT NULL REFERENCES membresias_marca(id) ON DELETE CASCADE,
  sucursal_id BIGINT NOT NULL REFERENCES sucursales(id) ON DELETE CASCADE,
  activo BOOLEAN NOT NULL DEFAULT TRUE,
  PRIMARY KEY (membresia_id, sucursal_id)
);

CREATE TABLE programas_fidelidad (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  marca_id BIGINT NOT NULL REFERENCES marcas(id) ON DELETE RESTRICT,
  tipo TEXT NOT NULL CHECK (tipo = 'SELLOS'),
  sellos_por_acumulacion BIGINT NOT NULL CHECK (sellos_por_acumulacion = 1),
  activo BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (marca_id, tipo)
);
CREATE UNIQUE INDEX programas_fidelidad_marca_activo_idx ON programas_fidelidad(marca_id) WHERE activo;

CREATE TABLE beneficios (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  programa_id BIGINT NOT NULL REFERENCES programas_fidelidad(id) ON DELETE RESTRICT,
  nombre VARCHAR(120) NOT NULL CHECK (btrim(nombre) <> ''),
  requisito_sellos BIGINT NOT NULL CHECK (requisito_sellos > 0),
  activo BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ
);

CREATE TABLE accesos_demo (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  marca_id BIGINT NOT NULL UNIQUE REFERENCES marcas(id) ON DELETE RESTRICT,
  tipo TEXT NOT NULL CHECK (tipo = 'SELLOS_FREE_TRIAL'),
  precio_minor BIGINT NOT NULL CHECK (precio_minor = 0),
  moneda CHAR(3) NOT NULL CHECK (moneda = 'ARS'),
  cobro_automatico BOOLEAN NOT NULL CHECK (cobro_automatico = FALSE),
  activo BOOLEAN NOT NULL DEFAULT TRUE,
  started_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE tarjetas (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  usuario_id BIGINT NOT NULL REFERENCES usuarios(id) ON DELETE RESTRICT,
  marca_id BIGINT NOT NULL REFERENCES marcas(id) ON DELETE RESTRICT,
  saldo_sellos BIGINT NOT NULL DEFAULT 0 CHECK (saldo_sellos >= 0),
  version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
  activo BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ,
  UNIQUE (usuario_id, marca_id)
);

CREATE TABLE previews_movimiento (
  id UUID PRIMARY KEY,
  actor_id BIGINT NOT NULL REFERENCES usuarios(id) ON DELETE RESTRICT,
  sucursal_id BIGINT NOT NULL REFERENCES sucursales(id) ON DELETE RESTRICT,
  marca_id BIGINT NOT NULL REFERENCES marcas(id) ON DELETE RESTRICT,
  cliente_id BIGINT NOT NULL REFERENCES usuarios(id) ON DELETE RESTRICT,
  tarjeta_id BIGINT REFERENCES tarjetas(id) ON DELETE RESTRICT,
  operacion TEXT NOT NULL CHECK (operacion IN ('ACUMULACION','CANJE')),
  beneficio_id BIGINT REFERENCES beneficios(id) ON DELETE RESTRICT,
  qr_hash BYTEA NOT NULL,
  saldo_anterior BIGINT NOT NULL CHECK (saldo_anterior >= 0),
  cantidad BIGINT NOT NULL CHECK (cantidad > 0),
  saldo_posterior BIGINT NOT NULL CHECK (saldo_posterior >= 0),
  request_fingerprint BYTEA NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  consumed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK ((operacion = 'ACUMULACION' AND beneficio_id IS NULL AND saldo_posterior = saldo_anterior + cantidad) OR
         (operacion = 'CANJE' AND beneficio_id IS NOT NULL AND saldo_posterior = saldo_anterior - cantidad))
);
CREATE INDEX previews_movimiento_actor_expiry_idx ON previews_movimiento(actor_id, expires_at);

CREATE TABLE historial_movimientos (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  operation_id UUID NOT NULL UNIQUE,
  tarjeta_id BIGINT NOT NULL REFERENCES tarjetas(id) ON DELETE RESTRICT,
  marca_id BIGINT NOT NULL REFERENCES marcas(id) ON DELETE RESTRICT,
  sucursal_id BIGINT NOT NULL REFERENCES sucursales(id) ON DELETE RESTRICT,
  usuario_operador_id BIGINT NOT NULL REFERENCES usuarios(id) ON DELETE RESTRICT,
  beneficio_id BIGINT REFERENCES beneficios(id) ON DELETE RESTRICT,
  operacion TEXT NOT NULL CHECK (operacion IN ('ACUMULACION','CANJE')),
  sentido TEXT NOT NULL CHECK (sentido IN ('CREDITO','DEBITO')),
  cantidad BIGINT NOT NULL CHECK (cantidad > 0),
  saldo_anterior BIGINT NOT NULL CHECK (saldo_anterior >= 0),
  saldo_posterior BIGINT NOT NULL CHECK (saldo_posterior >= 0),
  beneficio_nombre_snapshot VARCHAR(120),
  beneficio_requisito_snapshot BIGINT,
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK ((operacion='ACUMULACION' AND sentido='CREDITO' AND saldo_posterior=saldo_anterior+cantidad) OR
         (operacion='CANJE' AND sentido='DEBITO' AND saldo_posterior=saldo_anterior-cantidad)),
  CHECK ((beneficio_nombre_snapshot IS NULL AND beneficio_requisito_snapshot IS NULL) OR
         (beneficio_nombre_snapshot IS NOT NULL AND beneficio_requisito_snapshot > 0))
);
CREATE INDEX historial_movimientos_tarjeta_fecha_idx ON historial_movimientos(tarjeta_id, occurred_at DESC, id DESC);
CREATE INDEX historial_movimientos_marca_fecha_idx ON historial_movimientos(marca_id, occurred_at DESC, id DESC);
CREATE INDEX historial_movimientos_sucursal_fecha_idx ON historial_movimientos(sucursal_id, occurred_at DESC, id DESC);

CREATE TABLE solicitudes_idempotentes (
  idempotency_key UUID PRIMARY KEY,
  actor_scope TEXT NOT NULL,
  operacion TEXT NOT NULL,
  fingerprint BYTEA NOT NULL,
  estado TEXT NOT NULL CHECK (estado IN ('PENDING','COMPLETED')),
  response_status INTEGER,
  response_body BYTEA,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ,
  CHECK ((estado='PENDING' AND response_status IS NULL AND response_body IS NULL) OR
         (estado='COMPLETED' AND response_status BETWEEN 200 AND 299 AND response_body IS NOT NULL))
);
CREATE INDEX solicitudes_idempotentes_actor_operacion_idx ON solicitudes_idempotentes(actor_scope, operacion, created_at DESC);
