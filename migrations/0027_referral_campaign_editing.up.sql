ALTER TABLE referral_campaigns
  ADD COLUMN program_types TEXT[],
  ADD COLUMN version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
  ADD CONSTRAINT referral_campaign_program_types_check CHECK (
    program_types IS NULL OR (
      program_types IN (ARRAY['SELLOS'], ARRAY['PUNTOS'], ARRAY['SELLOS','PUNTOS'])
      AND program_type = program_types[1]
    )
  );

UPDATE referral_campaigns SET program_types=ARRAY[program_type];
