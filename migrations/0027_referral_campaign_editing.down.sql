DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM referral_campaigns WHERE cardinality(program_types) > 1) THEN
    RAISE EXCEPTION 'Dual-program campaigns must be narrowed explicitly before reverting campaign editing';
  END IF;
END $$;

ALTER TABLE referral_campaigns DROP CONSTRAINT referral_campaign_program_types_check,
  DROP COLUMN program_types, DROP COLUMN version;
