-- Existing users retain their legacy code; only new registrations allocate one.
ALTER TABLE usuarios ADD COLUMN codigo_usuario TEXT UNIQUE;
CREATE TABLE contadores_codigo_usuario (
  prefijo TEXT PRIMARY KEY,
  ultimo_numero BIGINT NOT NULL CHECK (ultimo_numero > 0)
);

CREATE FUNCTION preservar_codigo_usuario() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.codigo_usuario IS DISTINCT FROM OLD.codigo_usuario THEN
    RAISE EXCEPTION 'codigo_usuario is immutable' USING ERRCODE = '23514';
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER usuarios_codigo_inmutable BEFORE UPDATE OF codigo_usuario ON usuarios
FOR EACH ROW EXECUTE FUNCTION preservar_codigo_usuario();
