DO $$ BEGIN RAISE EXCEPTION '0011 is forward-only: legacy password accounts must not be silently re-verified'; END $$;
