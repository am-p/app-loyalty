CREATE TABLE referral_campaigns (
  id BIGSERIAL PRIMARY KEY,
  name TEXT NOT NULL CHECK (length(trim(name)) BETWEEN 3 AND 120),
  program_type TEXT NOT NULL CHECK (program_type IN ('SELLOS','PUNTOS')),
  discount_bps INTEGER NOT NULL CHECK (discount_bps BETWEEN 0 AND 10000),
  discount_charges INTEGER NOT NULL CHECK (discount_charges BETWEEN 0 AND 36),
  reward_bps INTEGER NOT NULL CHECK (reward_bps BETWEEN 0 AND 10000),
  reward_charges INTEGER NOT NULL CHECK (reward_charges BETWEEN 0 AND 36),
  starts_at TIMESTAMPTZ NOT NULL,
  ends_at TIMESTAMPTZ NOT NULL CHECK (ends_at > starts_at),
  active BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX referral_campaigns_window_idx ON referral_campaigns(program_type, starts_at, ends_at) WHERE active;

CREATE TABLE referral_influencers (
  id BIGSERIAL PRIMARY KEY,
  name TEXT NOT NULL CHECK (length(trim(name)) BETWEEN 2 AND 120),
  contact TEXT NOT NULL CHECK (length(trim(contact)) BETWEEN 3 AND 200),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE referral_codes (
  id BIGSERIAL PRIMARY KEY,
  code TEXT NOT NULL UNIQUE CHECK (code ~ '^[A-Z0-9-]{4,40}$'),
  campaign_id BIGINT NOT NULL REFERENCES referral_campaigns(id),
  source_kind TEXT NOT NULL CHECK (source_kind IN ('INFLUENCER','MERCHANT')),
  influencer_id BIGINT REFERENCES referral_influencers(id),
  source_brand_id BIGINT REFERENCES marcas(id),
  active BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK ((source_kind='INFLUENCER' AND influencer_id IS NOT NULL AND source_brand_id IS NULL) OR
         (source_kind='MERCHANT' AND source_brand_id IS NOT NULL AND influencer_id IS NULL))
);
CREATE UNIQUE INDEX referral_merchant_campaign_uk ON referral_codes(campaign_id,source_brand_id) WHERE source_kind='MERCHANT';

CREATE TABLE referral_attributions (
  brand_id BIGINT PRIMARY KEY REFERENCES marcas(id),
  code_id BIGINT NOT NULL REFERENCES referral_codes(id),
  campaign_id BIGINT NOT NULL REFERENCES referral_campaigns(id),
  source_kind TEXT NOT NULL CHECK (source_kind IN ('INFLUENCER','MERCHANT')),
  influencer_id BIGINT REFERENCES referral_influencers(id),
  source_brand_id BIGINT REFERENCES marcas(id),
  discount_bps INTEGER NOT NULL CHECK (discount_bps BETWEEN 0 AND 10000),
  discount_charges INTEGER NOT NULL CHECK (discount_charges BETWEEN 0 AND 36),
  reward_bps INTEGER NOT NULL CHECK (reward_bps BETWEEN 0 AND 10000),
  reward_charges INTEGER NOT NULL CHECK (reward_charges BETWEEN 0 AND 36),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (brand_id IS DISTINCT FROM source_brand_id)
);

CREATE TABLE referral_charges (
  provider_invoice_id TEXT PRIMARY KEY,
  brand_id BIGINT NOT NULL REFERENCES marcas(id),
  provider_payment_id TEXT,
  amount_minor BIGINT NOT NULL CHECK (amount_minor >= 0),
  currency CHAR(3) NOT NULL CHECK (currency='ARS'),
  status TEXT NOT NULL CHECK (status IN ('APPROVED','REFUNDED')),
  paid_index INTEGER,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (brand_id,paid_index)
);
CREATE UNIQUE INDEX referral_charges_payment_uk ON referral_charges(provider_payment_id) WHERE provider_payment_id IS NOT NULL;

CREATE TABLE referral_rewards (
  provider_invoice_id TEXT PRIMARY KEY REFERENCES referral_charges(provider_invoice_id),
  brand_id BIGINT NOT NULL REFERENCES marcas(id),
  source_kind TEXT NOT NULL CHECK (source_kind IN ('INFLUENCER','MERCHANT')),
  influencer_id BIGINT REFERENCES referral_influencers(id),
  source_brand_id BIGINT REFERENCES marcas(id),
  amount_minor BIGINT NOT NULL CHECK (amount_minor >= 0),
  status TEXT NOT NULL CHECK (status IN ('PENDING','SETTLED','VOID')),
  settled_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE backoffice_users (
  id BIGSERIAL PRIMARY KEY,
  email CITEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  totp_secret TEXT NOT NULL,
  last_totp_step BIGINT NOT NULL DEFAULT 0,
  role TEXT NOT NULL CHECK (role IN ('ADMIN_SISTEMA','FINANZAS')),
  active BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE backoffice_sessions (
  token_hash BYTEA PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES backoffice_users(id),
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX backoffice_sessions_expires_idx ON backoffice_sessions(expires_at);
CREATE TABLE backoffice_audit (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES backoffice_users(id),
  action TEXT NOT NULL,
  object_type TEXT NOT NULL,
  object_id TEXT NOT NULL,
  detail JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
