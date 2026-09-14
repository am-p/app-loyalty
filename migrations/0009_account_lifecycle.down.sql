ALTER TABLE usuarios
  DROP CONSTRAINT usuarios_foto_url_length_check,
  DROP CONSTRAINT usuarios_qr_lifecycle_check,
  ADD CONSTRAINT usuarios_check CHECK (
    (tipo_cuenta = 'CLIENTE_FINAL' AND qr_hash IS NOT NULL) OR
    (tipo_cuenta = 'PERSONAL_MARCA' AND qr_hash IS NULL)
  ),
  DROP COLUMN version,
  DROP COLUMN foto_url,
  DROP COLUMN alias,
  DROP COLUMN apellido;

