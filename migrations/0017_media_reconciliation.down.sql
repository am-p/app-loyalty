DROP INDEX idx_archivos_reconciliation;
ALTER TABLE archivos_marca DROP CONSTRAINT ck_archivos_lease, DROP COLUMN lease_until, DROP COLUMN lease_owner;
