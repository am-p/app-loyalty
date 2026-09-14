ALTER TABLE programas_fidelidad
  DROP CONSTRAINT IF EXISTS programas_fidelidad_tipo_check,
  DROP CONSTRAINT IF EXISTS programas_fidelidad_sellos_por_acumulacion_check;
ALTER TABLE programas_fidelidad ALTER COLUMN sellos_por_acumulacion DROP NOT NULL;
ALTER TABLE programas_fidelidad
  ADD CONSTRAINT programas_fidelidad_tipo_check CHECK (tipo IN ('SELLOS','PUNTOS')),
  ADD CONSTRAINT programas_fidelidad_acumulacion_check CHECK (
    (tipo='SELLOS' AND sellos_por_acumulacion=1) OR
    (tipo='PUNTOS' AND sellos_por_acumulacion IS NULL)
  );

ALTER TABLE beneficios DROP CONSTRAINT IF EXISTS beneficios_requisito_sellos_check;
ALTER TABLE beneficios
  ALTER COLUMN requisito_sellos DROP NOT NULL,
  ADD COLUMN requisito_puntos BIGINT;
ALTER TABLE beneficios
  ADD CONSTRAINT beneficios_requisito_check CHECK (
    (requisito_sellos BETWEEN 1 AND 10000000 AND requisito_puntos IS NULL) OR
    (requisito_sellos IS NULL AND requisito_puntos BETWEEN 1 AND 10000000)
  );

ALTER TABLE accesos_demo DROP CONSTRAINT IF EXISTS accesos_demo_tipo_check;
ALTER TABLE accesos_demo
  ADD CONSTRAINT accesos_demo_tipo_check CHECK (tipo IN ('SELLOS_FREE_TRIAL','PUNTOS_FREE_TRIAL'));

ALTER TABLE tarjetas ADD COLUMN saldo_puntos BIGINT NOT NULL DEFAULT 0 CHECK (saldo_puntos >= 0);
ALTER TABLE previews_movimiento
  ADD COLUMN programa_tipo TEXT NOT NULL DEFAULT 'SELLOS' CHECK (programa_tipo IN ('SELLOS','PUNTOS'));
ALTER TABLE historial_movimientos
  ADD COLUMN programa_tipo TEXT NOT NULL DEFAULT 'SELLOS' CHECK (programa_tipo IN ('SELLOS','PUNTOS')),
  ADD COLUMN beneficio_requisito_puntos_snapshot BIGINT;

CREATE OR REPLACE FUNCTION prevent_program_type_change_after_movement()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.tipo <> OLD.tipo AND EXISTS (
    SELECT 1 FROM historial_movimientos WHERE marca_id=OLD.marca_id
  ) THEN
    RAISE EXCEPTION 'program type is immutable after the first movement' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER programas_fidelidad_tipo_inmutable
BEFORE UPDATE OF tipo ON programas_fidelidad
FOR EACH ROW EXECUTE FUNCTION prevent_program_type_change_after_movement();

CREATE INDEX tarjetas_marca_activo_idx ON tarjetas(marca_id) WHERE activo;
CREATE INDEX beneficios_programa_activo_idx ON beneficios(programa_id) WHERE activo;
