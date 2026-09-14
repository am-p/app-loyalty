DO $$ BEGIN RAISE EXCEPTION '0009 is forward-only: restoring deleted account identifiers or removing versions is unsafe'; END $$;
