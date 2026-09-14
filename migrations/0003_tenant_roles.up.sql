ALTER TABLE membresias_marca DROP CONSTRAINT IF EXISTS membresias_marca_rol_check;
ALTER TABLE membresias_marca
  ADD CONSTRAINT membresias_marca_rol_check CHECK (rol IN ('PROPIETARIO','ADMINISTRADOR','OPERADOR')),
  ADD CONSTRAINT membresias_marca_id_marca_uk UNIQUE (id, marca_id);

ALTER TABLE sucursales
  ADD CONSTRAINT sucursales_id_marca_uk UNIQUE (id, marca_id);

ALTER TABLE membresias_sucursales ADD COLUMN marca_id BIGINT;
UPDATE membresias_sucursales ms
SET marca_id=s.marca_id
FROM sucursales s
WHERE s.id=ms.sucursal_id;
ALTER TABLE membresias_sucursales ALTER COLUMN marca_id SET NOT NULL;
ALTER TABLE membresias_sucursales
  ADD CONSTRAINT membresias_sucursales_membresia_marca_fk
    FOREIGN KEY (membresia_id,marca_id) REFERENCES membresias_marca(id,marca_id) ON DELETE CASCADE,
  ADD CONSTRAINT membresias_sucursales_sucursal_marca_fk
    FOREIGN KEY (sucursal_id,marca_id) REFERENCES sucursales(id,marca_id) ON DELETE CASCADE;

CREATE INDEX membresias_sucursales_sucursal_activa_idx
  ON membresias_sucursales(sucursal_id,marca_id) WHERE activo;
