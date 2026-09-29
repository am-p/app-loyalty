-- Freeze ledger inserts while backfill and trigger installation share one migration transaction.
LOCK TABLE historial_movimientos IN SHARE ROW EXCLUSIVE MODE;
-- Historical confirmed operations count once, including PUNTOS; no historical invitation.
CREATE TABLE configuracion_resenas (
 sucursal_id BIGINT PRIMARY KEY REFERENCES sucursales(id),
 enabled BOOLEAN NOT NULL DEFAULT false,
 purchase_threshold INTEGER NOT NULL DEFAULT 1 CHECK(purchase_threshold BETWEEN 1 AND 1000000),
 message TEXT NOT NULL DEFAULT '¿Cómo fue tu experiencia? Compartí tu opinión en Google.' CHECK(char_length(message) BETWEEN 1 AND 500),
 destination_type TEXT NOT NULL DEFAULT 'MANUAL_LINK' CHECK(destination_type IN ('MANUAL_LINK','GOOGLE_PLACE')),
 google_place_id TEXT,
 manual_review_url TEXT,
 version INTEGER NOT NULL DEFAULT 1 CHECK(version>0),
 CHECK((destination_type='MANUAL_LINK' AND google_place_id IS NULL) OR (destination_type='GOOGLE_PLACE' AND manual_review_url IS NULL)),
 CHECK(NOT enabled OR (destination_type='MANUAL_LINK' AND manual_review_url IS NOT NULL) OR (destination_type='GOOGLE_PLACE' AND google_place_id IS NOT NULL))
);
CREATE TABLE progreso_resenas (
 usuario_id BIGINT NOT NULL REFERENCES usuarios(id),
 sucursal_id BIGINT NOT NULL REFERENCES sucursales(id),
 purchases BIGINT NOT NULL CHECK(purchases>0),
 PRIMARY KEY(usuario_id,sucursal_id)
);
INSERT INTO progreso_resenas SELECT t.usuario_id,h.sucursal_id,count(*) FROM historial_movimientos h JOIN tarjetas t ON t.id=h.tarjeta_id JOIN usuarios u ON u.id=t.usuario_id AND u.activo AND u.deleted_at IS NULL WHERE h.operacion='ACUMULACION' AND h.sentido='CREDITO' GROUP BY t.usuario_id,h.sucursal_id;
CREATE TABLE invitaciones_resenas (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 usuario_id BIGINT REFERENCES usuarios(id),
 tarjeta_id BIGINT REFERENCES tarjetas(id),
 marca_id BIGINT NOT NULL REFERENCES marcas(id),
 sucursal_id BIGINT NOT NULL REFERENCES sucursales(id),
 operation_id UUID REFERENCES historial_movimientos(operation_id),
 occurred_at TIMESTAMPTZ NOT NULL,
 cancelled_at TIMESTAMPTZ,
 reservation_token UUID,
 lease_until TIMESTAMPTZ,
 shown_at TIMESTAMPTZ,
 skipped_at TIMESTAMPTZ,
 clicked_at TIMESTAMPTZ,
 UNIQUE(usuario_id,sucursal_id),
 CHECK((reservation_token IS NULL)=(lease_until IS NULL)),
 CHECK((skipped_at IS NULL AND clicked_at IS NULL) OR shown_at IS NOT NULL)
);
CREATE INDEX invitaciones_resenas_branch_idx ON invitaciones_resenas(sucursal_id);
CREATE INDEX invitaciones_resenas_pending_idx ON invitaciones_resenas(usuario_id,occurred_at,id) WHERE shown_at IS NULL AND cancelled_at IS NULL;
CREATE FUNCTION registrar_compra_resena() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE customer BIGINT; total BIGINT;
BEGIN
 IF NEW.operacion<>'ACUMULACION' OR NEW.sentido<>'CREDITO' THEN RETURN NEW; END IF;
 SELECT t.usuario_id INTO customer FROM tarjetas t JOIN usuarios u ON u.id=t.usuario_id AND u.activo AND u.deleted_at IS NULL WHERE t.id=NEW.tarjeta_id;
 IF customer IS NULL THEN RETURN NEW; END IF;
 -- Serialize creation with settings writes and branch deactivation, including absent settings.
 PERFORM id FROM sucursales WHERE id=NEW.sucursal_id FOR SHARE;
 INSERT INTO progreso_resenas VALUES(customer,NEW.sucursal_id,1) ON CONFLICT(usuario_id,sucursal_id) DO UPDATE SET purchases=progreso_resenas.purchases+1 RETURNING purchases INTO total;
 INSERT INTO invitaciones_resenas(usuario_id,tarjeta_id,marca_id,sucursal_id,operation_id,occurred_at)
 SELECT customer,NEW.tarjeta_id,NEW.marca_id,NEW.sucursal_id,NEW.operation_id,NEW.occurred_at FROM configuracion_resenas c JOIN sucursales s ON s.id=c.sucursal_id AND s.activo AND s.deleted_at IS NULL JOIN marcas m ON m.id=s.marca_id AND m.activo AND m.deleted_at IS NULL WHERE c.sucursal_id=NEW.sucursal_id AND c.enabled AND c.purchase_threshold=total ON CONFLICT(usuario_id,sucursal_id) DO NOTHING;
 RETURN NEW;
END $$;
CREATE TRIGGER historial_resenas AFTER INSERT ON historial_movimientos FOR EACH ROW EXECUTE FUNCTION registrar_compra_resena();
CREATE FUNCTION cancelar_resenas() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (TG_TABLE_NAME='configuracion_resenas' AND NOT (to_jsonb(NEW)->>'enabled')::boolean) OR (TG_TABLE_NAME='sucursales' AND NOT (to_jsonb(NEW)->>'activo')::boolean) THEN
 UPDATE invitaciones_resenas SET cancelled_at=now(),reservation_token=NULL,lease_until=NULL WHERE sucursal_id=CASE WHEN TG_TABLE_NAME='sucursales' THEN (to_jsonb(NEW)->>'id')::bigint ELSE (to_jsonb(NEW)->>'sucursal_id')::bigint END AND shown_at IS NULL AND cancelled_at IS NULL;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER configuracion_cancelar_resenas AFTER UPDATE OF enabled ON configuracion_resenas FOR EACH ROW EXECUTE FUNCTION cancelar_resenas();
CREATE TRIGGER sucursal_cancelar_resenas AFTER UPDATE OF activo ON sucursales FOR EACH ROW EXECUTE FUNCTION cancelar_resenas();
