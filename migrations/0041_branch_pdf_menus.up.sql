CREATE TABLE branch_menu_documents (
 id UUID PRIMARY KEY,
 marca_id BIGINT NOT NULL REFERENCES marcas(id),
 sucursal_id BIGINT NOT NULL,
 file_name VARCHAR(160) NOT NULL CHECK(length(file_name)>0),
 mime_type TEXT NOT NULL DEFAULT 'application/pdf' CHECK(mime_type='application/pdf'),
 byte_size BIGINT NOT NULL CHECK(byte_size BETWEEN 1 AND 10485760),
 page_count INTEGER NOT NULL CHECK(page_count BETWEEN 1 AND 30),
 sha256 BYTEA NOT NULL CHECK(octet_length(sha256)=32),
 object_key TEXT NOT NULL UNIQUE,
 estado TEXT NOT NULL DEFAULT 'UPLOAD_PENDING' CHECK(estado IN ('UPLOAD_PENDING','READY','DELETE_PENDING','DELETED')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 delete_after TIMESTAMPTZ,
 lease_owner TEXT,
 lease_until TIMESTAMPTZ,
 UNIQUE(id,sucursal_id,marca_id),
 FOREIGN KEY(sucursal_id,marca_id) REFERENCES sucursales(id,marca_id)
);
CREATE INDEX branch_menu_documents_cleanup_idx ON branch_menu_documents(estado,delete_after,created_at);
CREATE TABLE branch_menus (
 sucursal_id BIGINT PRIMARY KEY,
 marca_id BIGINT NOT NULL REFERENCES marcas(id),
 version INTEGER NOT NULL DEFAULT 1 CHECK(version>0),
 draft_document_id UUID,
 published_document_id UUID,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(sucursal_id,marca_id) REFERENCES sucursales(id,marca_id),
 FOREIGN KEY(draft_document_id,sucursal_id,marca_id) REFERENCES branch_menu_documents(id,sucursal_id,marca_id),
 FOREIGN KEY(published_document_id,sucursal_id,marca_id) REFERENCES branch_menu_documents(id,sucursal_id,marca_id),
 CHECK(draft_document_id IS NULL OR published_document_id IS NULL OR draft_document_id<>published_document_id)
);
-- All lifecycle paths, including account anonymization that closes brands, retire
-- publications and queued uploads. A six-minute grace covers the maximum
-- five-minute signed URL plus clock/transaction margin.
CREATE FUNCTION retire_branch_pdf_menus() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.activo AND NEW.deleted_at IS NULL THEN RETURN NEW; END IF;
 IF TG_TABLE_NAME='marcas' THEN
  UPDATE branch_menus SET draft_document_id=NULL,published_document_id=NULL,version=version+1,updated_at=now()
   WHERE marca_id=NEW.id AND (draft_document_id IS NOT NULL OR published_document_id IS NOT NULL);
  UPDATE branch_menu_documents SET estado='DELETE_PENDING',delete_after=clock_timestamp()+interval '6 minutes',lease_owner=NULL,lease_until=NULL
   WHERE marca_id=NEW.id AND estado IN ('UPLOAD_PENDING','READY');
 ELSE
  UPDATE branch_menus SET draft_document_id=NULL,published_document_id=NULL,version=version+1,updated_at=now()
   WHERE sucursal_id=NEW.id AND (draft_document_id IS NOT NULL OR published_document_id IS NOT NULL);
  UPDATE branch_menu_documents SET estado='DELETE_PENDING',delete_after=clock_timestamp()+interval '6 minutes',lease_owner=NULL,lease_until=NULL
   WHERE sucursal_id=NEW.id AND estado IN ('UPLOAD_PENDING','READY');
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER marcas_retire_pdf_menus AFTER UPDATE OF activo,deleted_at ON marcas
 FOR EACH ROW WHEN (OLD.activo IS DISTINCT FROM NEW.activo OR OLD.deleted_at IS DISTINCT FROM NEW.deleted_at) EXECUTE FUNCTION retire_branch_pdf_menus();
CREATE TRIGGER sucursales_retire_pdf_menus AFTER UPDATE OF activo,deleted_at ON sucursales
 FOR EACH ROW WHEN (OLD.activo IS DISTINCT FROM NEW.activo OR OLD.deleted_at IS DISTINCT FROM NEW.deleted_at) EXECUTE FUNCTION retire_branch_pdf_menus();
