ALTER TABLE historial_movimientos
  DROP CONSTRAINT historial_movimientos_sucursal_snapshot_check,
  DROP CONSTRAINT historial_movimientos_marca_snapshot_check,
  DROP COLUMN programa_id_snapshot,
  DROP COLUMN sucursal_nombre_snapshot,
  DROP COLUMN marca_nombre_snapshot;
