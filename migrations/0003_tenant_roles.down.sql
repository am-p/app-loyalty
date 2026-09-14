DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM membresias_marca WHERE rol<>'PROPIETARIO') THEN
    RAISE EXCEPTION 'cannot remove role support while non-owner memberships exist';
  END IF;
END;
$$;

DROP INDEX IF EXISTS membresias_sucursales_sucursal_activa_idx;
ALTER TABLE membresias_sucursales
  DROP CONSTRAINT membresias_sucursales_sucursal_marca_fk,
  DROP CONSTRAINT membresias_sucursales_membresia_marca_fk,
  DROP COLUMN marca_id;
ALTER TABLE sucursales DROP CONSTRAINT sucursales_id_marca_uk;
ALTER TABLE membresias_marca
  DROP CONSTRAINT membresias_marca_id_marca_uk,
  DROP CONSTRAINT membresias_marca_rol_check;
ALTER TABLE membresias_marca
  ADD CONSTRAINT membresias_marca_rol_check CHECK (rol='PROPIETARIO');
