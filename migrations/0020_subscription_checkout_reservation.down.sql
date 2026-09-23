DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM suscripciones_marca WHERE estado = 'CREATING') THEN
    RAISE EXCEPTION 'Cannot downgrade while a checkout needs reconciliation';
  END IF;
END $$;

ALTER TABLE suscripciones_marca DROP CONSTRAINT suscripciones_marca_provider_id_check;
ALTER TABLE suscripciones_marca DROP CONSTRAINT suscripciones_marca_estado_check;
ALTER TABLE suscripciones_marca
  ADD CONSTRAINT suscripciones_marca_estado_check
  CHECK (estado IN ('PENDING','AUTHORIZED','PAUSED','CANCELLED'));
ALTER TABLE suscripciones_marca DROP COLUMN trial_months;
ALTER TABLE suscripciones_marca DROP COLUMN provider_call_started_at;
ALTER TABLE suscripciones_marca ALTER COLUMN proveedor_suscripcion_id SET NOT NULL;
