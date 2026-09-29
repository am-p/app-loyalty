DROP VIEW IF EXISTS referral_influencer_payables;
DROP VIEW IF EXISTS referral_merchant_credit_balances;
DROP TABLE IF EXISTS referral_merchant_credit_allocations;
ALTER TABLE referral_charges DROP COLUMN IF EXISTS full_amount_minor;
UPDATE referral_rewards SET status='VOID' WHERE status='RECOVERY_DUE';
ALTER TABLE referral_rewards DROP COLUMN IF EXISTS recovery_due_at;
ALTER TABLE referral_rewards DROP CONSTRAINT referral_rewards_status_check;
ALTER TABLE referral_rewards ADD CONSTRAINT referral_rewards_status_check CHECK (status IN ('PENDING','SETTLED','VOID'));
ALTER TABLE suscripciones_marca DROP COLUMN IF EXISTS price_transitioned_at, DROP COLUMN IF EXISTS full_unit_price_minor;
