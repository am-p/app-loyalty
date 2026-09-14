ALTER TABLE archivos_marca
    ADD COLUMN lease_owner UUID,
    ADD COLUMN lease_until TIMESTAMPTZ,
    ADD CONSTRAINT ck_archivos_lease CHECK ((lease_owner IS NULL) = (lease_until IS NULL));

CREATE INDEX idx_archivos_reconciliation ON archivos_marca(estado,delete_after,created_at,id)
    WHERE estado IN ('UPLOAD_PENDING','UPLOAD_FAILED','DELETE_PENDING');
