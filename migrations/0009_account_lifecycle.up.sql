ALTER TABLE usuarios
  ADD COLUMN apellido VARCHAR(120),
  ADD COLUMN alias VARCHAR(80),
  ADD COLUMN foto_url TEXT,
  ADD COLUMN version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0);

ALTER TABLE usuarios DROP CONSTRAINT usuarios_check;
ALTER TABLE usuarios
  ADD CONSTRAINT usuarios_qr_lifecycle_check CHECK (
    deleted_at IS NOT NULL OR
    (tipo_cuenta = 'CLIENTE_FINAL' AND qr_hash IS NOT NULL) OR
    (tipo_cuenta = 'PERSONAL_MARCA' AND qr_hash IS NULL)
  ),
  ADD CONSTRAINT usuarios_foto_url_length_check CHECK (foto_url IS NULL OR length(foto_url) <= 2048);
