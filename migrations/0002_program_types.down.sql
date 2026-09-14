DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM programas_fidelidad WHERE tipo='PUNTOS') THEN
    RAISE EXCEPTION 'cannot remove PUNTOS schema while PUNTOS programs exist';
  END IF;
END;
$$;

DROP TRIGGER IF EXISTS programas_fidelidad_tipo_inmutable ON programas_fidelidad;
DROP FUNCTION IF EXISTS prevent_program_type_change_after_movement();
DROP INDEX IF EXISTS beneficios_programa_activo_idx;
DROP INDEX IF EXISTS tarjetas_marca_activo_idx;
ALTER TABLE historial_movimientos DROP COLUMN beneficio_requisito_puntos_snapshot, DROP COLUMN programa_tipo;
ALTER TABLE previews_movimiento DROP COLUMN programa_tipo;
ALTER TABLE tarjetas DROP COLUMN saldo_puntos;
ALTER TABLE accesos_demo DROP CONSTRAINT accesos_demo_tipo_check;
ALTER TABLE accesos_demo ADD CONSTRAINT accesos_demo_tipo_check CHECK (tipo='SELLOS_FREE_TRIAL');
ALTER TABLE beneficios DROP CONSTRAINT beneficios_requisito_check;
ALTER TABLE beneficios DROP COLUMN requisito_puntos;
ALTER TABLE beneficios ALTER COLUMN requisito_sellos SET NOT NULL;
ALTER TABLE beneficios ADD CONSTRAINT beneficios_requisito_sellos_check CHECK (requisito_sellos > 0);
ALTER TABLE programas_fidelidad DROP CONSTRAINT programas_fidelidad_acumulacion_check;
ALTER TABLE programas_fidelidad DROP CONSTRAINT programas_fidelidad_tipo_check;
ALTER TABLE programas_fidelidad ALTER COLUMN sellos_por_acumulacion SET NOT NULL;
ALTER TABLE programas_fidelidad ADD CONSTRAINT programas_fidelidad_tipo_check CHECK (tipo='SELLOS');
ALTER TABLE programas_fidelidad ADD CONSTRAINT programas_fidelidad_sellos_por_acumulacion_check CHECK (sellos_por_acumulacion=1);
