ALTER TABLE suscripciones_marca
  ADD COLUMN full_unit_price_minor BIGINT CHECK (full_unit_price_minor > 0),
  ADD COLUMN price_transitioned_at TIMESTAMPTZ;

ALTER TABLE referral_rewards DROP CONSTRAINT referral_rewards_status_check;
ALTER TABLE referral_rewards ADD CONSTRAINT referral_rewards_status_check
  CHECK (status IN ('PENDING','SETTLED','VOID','RECOVERY_DUE'));
ALTER TABLE referral_rewards ADD COLUMN recovery_due_at TIMESTAMPTZ;
ALTER TABLE referral_charges ADD COLUMN full_amount_minor BIGINT CHECK (full_amount_minor >= amount_minor);
UPDATE referral_charges SET full_amount_minor=amount_minor;
ALTER TABLE referral_charges ALTER COLUMN full_amount_minor SET NOT NULL;

-- A reward remains a liability until finance settles it. A merchant credit can be
-- consumed only through an explicit billing offset, never by mutating this ledger.
CREATE VIEW referral_merchant_credit_balances AS
SELECT source_brand_id AS brand_id, COALESCE(sum(amount_minor),0)::bigint AS balance_minor
FROM referral_rewards WHERE source_kind='MERCHANT' AND status='PENDING'
GROUP BY source_brand_id;

CREATE VIEW referral_influencer_payables AS
SELECT influencer_id, COALESCE(sum(amount_minor),0)::bigint AS balance_minor
FROM referral_rewards
WHERE source_kind='INFLUENCER' AND status='PENDING'
GROUP BY influencer_id;

CREATE TABLE referral_merchant_credit_allocations (
  id BIGSERIAL PRIMARY KEY,
  source_brand_id BIGINT NOT NULL REFERENCES marcas(id),
  provider_invoice_id TEXT NOT NULL,
  provider_payment_id TEXT NOT NULL,
  amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
  currency CHAR(3) NOT NULL DEFAULT 'ARS' CHECK (currency='ARS'),
  external_settlement_reference TEXT NOT NULL UNIQUE CHECK (length(trim(external_settlement_reference)) BETWEEN 6 AND 120),
  idempotency_key UUID NOT NULL UNIQUE,
  finance_user_id BIGINT NOT NULL REFERENCES backoffice_users(id),
  status TEXT NOT NULL DEFAULT 'RECORDED' CHECK (status IN ('RECORDED','RECOVERY_DUE')),
  recovery_due_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX referral_merchant_credit_allocations_brand_idx ON referral_merchant_credit_allocations(source_brand_id,created_at DESC);
CREATE INDEX referral_merchant_credit_allocations_payment_idx ON referral_merchant_credit_allocations(provider_payment_id);

CREATE OR REPLACE VIEW referral_merchant_credit_balances AS
SELECT b.brand_id,
  COALESCE((SELECT sum(r.amount_minor) FROM referral_rewards r WHERE r.source_kind='MERCHANT' AND r.status='PENDING' AND r.source_brand_id=b.brand_id),0)::bigint
  - COALESCE((SELECT sum(a.amount_minor) FROM referral_merchant_credit_allocations a WHERE a.source_brand_id=b.brand_id),0)::bigint AS balance_minor
FROM (SELECT source_brand_id AS brand_id FROM referral_rewards WHERE source_kind='MERCHANT'
      UNION SELECT source_brand_id FROM referral_merchant_credit_allocations) b;
