ALTER TABLE beneficios DROP CONSTRAINT beneficios_descripcion_check,DROP COLUMN updated_at,DROP COLUMN descripcion;
ALTER TABLE programas_fidelidad DROP CONSTRAINT programas_nombre_unidad_check,DROP COLUMN updated_at,DROP COLUMN version,DROP COLUMN nombre_unidad;
DROP INDEX sucursales_marca_principal_activa_idx;
ALTER TABLE sucursales DROP CONSTRAINT sucursales_longitud_check,DROP CONSTRAINT sucursales_latitud_check,DROP COLUMN updated_at,DROP COLUMN version,DROP COLUMN principal,DROP COLUMN longitud,DROP COLUMN latitud,DROP COLUMN codigo_postal,DROP COLUMN provincia,DROP COLUMN localidad;
ALTER TABLE marcas DROP CONSTRAINT marcas_color_secundario_check,DROP CONSTRAINT marcas_color_primario_check,DROP CONSTRAINT marcas_descripcion_check,DROP COLUMN updated_at,DROP COLUMN version,DROP COLUMN zona_horaria,DROP COLUMN color_secundario,DROP COLUMN color_primario,DROP COLUMN descripcion;

