-- Expand the existing PDF metadata bound without changing document bytes or pointers.
ALTER TABLE branch_menu_documents
 DROP CONSTRAINT branch_menu_documents_byte_size_check,
 ADD CONSTRAINT branch_menu_documents_byte_size_check
 CHECK(byte_size BETWEEN 1 AND 31457280);
