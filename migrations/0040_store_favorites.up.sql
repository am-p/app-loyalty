CREATE TABLE sucursales_favoritas (
  usuario_id BIGINT NOT NULL REFERENCES usuarios(id) ON DELETE CASCADE,
  sucursal_id BIGINT NOT NULL REFERENCES sucursales(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (usuario_id, sucursal_id)
);

CREATE INDEX sucursales_favoritas_sucursal_idx ON sucursales_favoritas(sucursal_id);
