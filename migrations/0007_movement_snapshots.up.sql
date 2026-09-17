ALTER TABLE historial_movimientos
  ADD COLUMN marca_nombre_snapshot VARCHAR(120),
  ADD COLUMN sucursal_nombre_snapshot VARCHAR(120),
  ADD COLUMN programa_id_snapshot BIGINT;

UPDATE historial_movimientos h
SET marca_nombre_snapshot=m.nombre,
    sucursal_nombre_snapshot=s.nombre,
    programa_id_snapshot=p.id
FROM marcas m,sucursales s,programas_fidelidad p
WHERE m.id=h.marca_id
  AND s.id=h.sucursal_id
  AND p.marca_id=h.marca_id
  AND p.tipo=h.programa_tipo;

ALTER TABLE historial_movimientos
  ALTER COLUMN marca_nombre_snapshot SET NOT NULL,
  ALTER COLUMN sucursal_nombre_snapshot SET NOT NULL,
  ALTER COLUMN programa_id_snapshot SET NOT NULL,
  ADD CONSTRAINT historial_movimientos_marca_snapshot_check CHECK (btrim(marca_nombre_snapshot)<>''),
  ADD CONSTRAINT historial_movimientos_sucursal_snapshot_check CHECK (btrim(sucursal_nombre_snapshot)<>'');
