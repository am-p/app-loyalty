ALTER TABLE usuarios ADD COLUMN first_login_at TIMESTAMPTZ, ADD COLUMN first_login_estimated BOOLEAN NOT NULL DEFAULT false;
-- Historical sessions are pruned. Account creation is a conservative bound,
-- never a fabricated exact first-login time or a fresh trial at migration time.
UPDATE usuarios SET first_login_at=created_at,first_login_estimated=true;
ALTER TABLE marcas ADD COLUMN trial_started_at TIMESTAMPTZ, ADD COLUMN trial_start_estimated BOOLEAN NOT NULL DEFAULT false;
UPDATE marcas m SET trial_started_at=x.started,trial_start_estimated=true FROM
 (SELECT mm.marca_id,min(u.first_login_at) AS started FROM membresias_marca mm JOIN usuarios u ON u.id=mm.usuario_id WHERE mm.rol='PROPIETARIO' GROUP BY mm.marca_id) x WHERE x.marca_id=m.id;
ALTER TABLE suscripciones_marca ADD COLUMN trial_ends_at TIMESTAMPTZ;
ALTER TABLE suscripciones_marca ADD COLUMN checkout_rejected BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE suscripciones_marca DROP CONSTRAINT suscripciones_marca_provider_id_check;
ALTER TABLE suscripciones_marca ADD CONSTRAINT suscripciones_marca_provider_id_check CHECK (
 (estado='CREATING' AND proveedor_suscripcion_id IS NULL AND NOT checkout_rejected)
 OR (estado='CANCELLED' AND proveedor_suscripcion_id IS NULL AND checkout_rejected)
 OR (estado<>'CREATING' AND proveedor_suscripcion_id IS NOT NULL AND NOT checkout_rejected));
