-- A destructive rollback could reassign public identifiers. Use a forward fix.
DO $$ BEGIN RAISE EXCEPTION '0033 preserves issued user codes; use a forward migration'; END $$;
