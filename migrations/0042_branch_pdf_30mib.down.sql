-- Never narrow the bound while preserving a document larger than the old limit.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM branch_menu_documents WHERE byte_size > 10485760) THEN
  RAISE EXCEPTION 'cannot narrow PDF size limit while documents exceed 10 MiB';
 END IF;
END $$;
ALTER TABLE branch_menu_documents
 DROP CONSTRAINT branch_menu_documents_byte_size_check,
 ADD CONSTRAINT branch_menu_documents_byte_size_check
 CHECK(byte_size BETWEEN 1 AND 10485760);
