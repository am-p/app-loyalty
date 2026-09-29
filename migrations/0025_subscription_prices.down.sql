-- Application rollback must disable price changes first and keep provider prices
-- aligned. Prefer retaining this additive schema and rolling the app forward.
DROP TABLE subscription_price_history;
DROP TABLE subscription_price_change_items;
DROP TABLE subscription_price_changes;
DROP TABLE subscription_prices;
ALTER TABLE suscripciones_marca DROP COLUMN price_valid_from,DROP COLUMN program_type;
