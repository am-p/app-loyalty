DROP TRIGGER IF EXISTS sucursales_retire_pdf_menus ON sucursales;
DROP TRIGGER IF EXISTS marcas_retire_pdf_menus ON marcas;
DROP FUNCTION IF EXISTS retire_branch_pdf_menus();
DROP TABLE IF EXISTS branch_menus;
DROP TABLE IF EXISTS branch_menu_documents;
