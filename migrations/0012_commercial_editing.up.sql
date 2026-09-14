ALTER TABLE marcas ADD COLUMN descripcion TEXT,ADD COLUMN color_primario VARCHAR(7),ADD COLUMN color_secundario VARCHAR(7),ADD COLUMN zona_horaria VARCHAR(80) NOT NULL DEFAULT 'America/Argentina/Buenos_Aires',ADD COLUMN version INTEGER NOT NULL DEFAULT 1 CHECK(version>0),ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE marcas ADD CONSTRAINT marcas_descripcion_check CHECK(descripcion IS NULL OR length(descripcion)<=1000),ADD CONSTRAINT marcas_color_primario_check CHECK(color_primario IS NULL OR color_primario ~ '^#[0-9A-Fa-f]{6}$'),ADD CONSTRAINT marcas_color_secundario_check CHECK(color_secundario IS NULL OR color_secundario ~ '^#[0-9A-Fa-f]{6}$');

ALTER TABLE sucursales ADD COLUMN localidad VARCHAR(120),ADD COLUMN provincia VARCHAR(120),ADD COLUMN codigo_postal VARCHAR(20),ADD COLUMN latitud DOUBLE PRECISION,ADD COLUMN longitud DOUBLE PRECISION,ADD COLUMN principal BOOLEAN NOT NULL DEFAULT false,ADD COLUMN version INTEGER NOT NULL DEFAULT 1 CHECK(version>0),ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
UPDATE sucursales s SET principal=true WHERE s.id=(SELECT min(x.id) FROM sucursales x WHERE x.marca_id=s.marca_id AND x.activo AND x.deleted_at IS NULL);
ALTER TABLE sucursales ADD CONSTRAINT sucursales_latitud_check CHECK(latitud IS NULL OR latitud BETWEEN -90 AND 90),ADD CONSTRAINT sucursales_longitud_check CHECK(longitud IS NULL OR longitud BETWEEN -180 AND 180);
CREATE UNIQUE INDEX sucursales_marca_principal_activa_idx ON sucursales(marca_id) WHERE principal AND activo AND deleted_at IS NULL;

ALTER TABLE programas_fidelidad ADD COLUMN nombre_unidad VARCHAR(40),ADD COLUMN version INTEGER NOT NULL DEFAULT 1 CHECK(version>0),ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
UPDATE programas_fidelidad SET nombre_unidad=CASE tipo WHEN 'PUNTOS' THEN 'puntos' ELSE 'sellos' END;
ALTER TABLE programas_fidelidad ALTER COLUMN nombre_unidad SET NOT NULL,ADD CONSTRAINT programas_nombre_unidad_check CHECK(btrim(nombre_unidad)<>'');

ALTER TABLE beneficios ADD COLUMN descripcion TEXT NOT NULL DEFAULT '',ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE beneficios ADD CONSTRAINT beneficios_descripcion_check CHECK(length(descripcion)<=1000);

