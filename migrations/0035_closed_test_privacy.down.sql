-- Privacy deletions cannot be undone by a schema rollback.
DO $$ BEGIN
 RAISE EXCEPTION '0035 is forward-only; use a forward migration and the external deletion journal for restore';
END $$;
