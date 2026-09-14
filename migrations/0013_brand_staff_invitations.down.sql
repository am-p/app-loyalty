DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM invitaciones_marca) THEN
    RAISE EXCEPTION '0013 cannot be reverted while brand invitations exist';
  END IF;
END $$;
ALTER TABLE email_outbox DROP CONSTRAINT email_outbox_tipo_check;
ALTER TABLE email_outbox ADD CONSTRAINT email_outbox_tipo_check
  CHECK (tipo IN ('VERIFY_EMAIL','RESET_PASSWORD'));
DROP TABLE invitaciones_sucursales;
DROP TABLE invitaciones_marca;
ALTER TABLE membresias_marca DROP COLUMN updated_at, DROP COLUMN version;

