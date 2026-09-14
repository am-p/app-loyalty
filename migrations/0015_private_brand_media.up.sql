ALTER TABLE programas_fidelidad
    ADD CONSTRAINT uq_programas_id_marca UNIQUE (id, marca_id);

ALTER TABLE beneficios
    ADD CONSTRAINT uq_beneficios_id_programa UNIQUE (id, programa_id);

CREATE TABLE archivos_marca (
    id UUID PRIMARY KEY,
    marca_id BIGINT NOT NULL REFERENCES marcas(id),
    tipo VARCHAR(16) NOT NULL CHECK (tipo IN ('LOGO', 'ICONO', 'BENEFICIO')),
    beneficio_id BIGINT,
    programa_id BIGINT,
    object_key TEXT NOT NULL UNIQUE,
    mime_type VARCHAR(32) NOT NULL CHECK (mime_type IN ('image/jpeg', 'image/png')),
    byte_size BIGINT NOT NULL CHECK (byte_size BETWEEN 1 AND 5242880),
    sha256 BYTEA NOT NULL CHECK (octet_length(sha256) = 32),
    width INTEGER NOT NULL CHECK (width BETWEEN 1 AND 4096),
    height INTEGER NOT NULL CHECK (height BETWEEN 1 AND 4096),
    estado VARCHAR(24) NOT NULL CHECK (estado IN ('UPLOAD_PENDING', 'ACTIVA', 'UPLOAD_FAILED', 'DELETE_PENDING', 'DELETED')),
    replaces_id UUID REFERENCES archivos_marca(id),
    version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
    delete_after TIMESTAMPTZ,
    last_error VARCHAR(1000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    uploaded_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    CONSTRAINT ck_archivos_beneficio_scope CHECK (
        (tipo = 'BENEFICIO' AND beneficio_id IS NOT NULL AND programa_id IS NOT NULL)
        OR (tipo <> 'BENEFICIO' AND beneficio_id IS NULL AND programa_id IS NULL)
    ),
    CONSTRAINT ck_archivos_pixels CHECK ((width::BIGINT * height::BIGINT) <= 16000000),
    CONSTRAINT fk_archivos_programa_marca FOREIGN KEY (programa_id, marca_id)
        REFERENCES programas_fidelidad(id, marca_id),
    CONSTRAINT fk_archivos_beneficio_programa FOREIGN KEY (beneficio_id, programa_id)
        REFERENCES beneficios(id, programa_id)
);

CREATE UNIQUE INDEX uq_archivos_slot_marca_pending
    ON archivos_marca (marca_id, tipo)
    WHERE tipo IN ('LOGO', 'ICONO') AND estado = 'UPLOAD_PENDING';
CREATE UNIQUE INDEX uq_archivos_slot_beneficio_pending
    ON archivos_marca (marca_id, beneficio_id)
    WHERE tipo = 'BENEFICIO' AND estado = 'UPLOAD_PENDING';
CREATE UNIQUE INDEX uq_archivos_slot_marca_activa
    ON archivos_marca (marca_id, tipo)
    WHERE tipo IN ('LOGO', 'ICONO') AND estado = 'ACTIVA';
CREATE UNIQUE INDEX uq_archivos_slot_beneficio_activa
    ON archivos_marca (marca_id, beneficio_id)
    WHERE tipo = 'BENEFICIO' AND estado = 'ACTIVA';
CREATE INDEX idx_archivos_cleanup ON archivos_marca (delete_after, id)
    WHERE estado = 'DELETE_PENDING';

INSERT INTO schema_migrations(version) VALUES ('0015');
