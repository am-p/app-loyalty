DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM archivos_marca) THEN
        RAISE EXCEPTION '0015 is forward-only while brand media rows exist';
    END IF;
END $$;

DROP TABLE archivos_marca;
ALTER TABLE beneficios DROP CONSTRAINT uq_beneficios_id_programa;
ALTER TABLE programas_fidelidad DROP CONSTRAINT uq_programas_id_marca;
DELETE FROM schema_migrations WHERE version = '0015';
