ALTER TABLE suscripciones_marca
  ALTER COLUMN proveedor_suscripcion_id DROP NOT NULL,
  ADD COLUMN trial_months INTEGER NOT NULL DEFAULT 0 CHECK (trial_months IN (0, 1)),
  ADD COLUMN provider_call_started_at TIMESTAMPTZ;

ALTER TABLE suscripciones_marca DROP CONSTRAINT suscripciones_marca_estado_check;
ALTER TABLE suscripciones_marca
  ADD CONSTRAINT suscripciones_marca_estado_check
  CHECK (estado IN ('CREATING','PENDING','AUTHORIZED','PAUSED','CANCELLED'));
ALTER TABLE suscripciones_marca
  ADD CONSTRAINT suscripciones_marca_provider_id_check
  CHECK ((estado = 'CREATING' AND proveedor_suscripcion_id IS NULL)
    OR (estado <> 'CREATING' AND proveedor_suscripcion_id IS NOT NULL));
