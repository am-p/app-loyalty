DO $$ BEGIN RAISE EXCEPTION '0010 is forward-only: plaintext identity-token outbox payloads must not be restored'; END $$;
