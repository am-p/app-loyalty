ALTER TABLE subscription_price_changes ADD COLUMN previous_unit_price_minor BIGINT
  CHECK (previous_unit_price_minor BETWEEN 1 AND 9007199254740991);

-- Recover only recorded catalog versions. A legacy first change used an
-- environment fallback which was not snapshotted; leave that value unknown.
UPDATE subscription_price_changes current SET previous_unit_price_minor=previous.unit_price_minor
FROM subscription_price_changes previous
WHERE previous.program_type=current.program_type AND previous.price_version=current.previous_version;

CREATE INDEX subscription_price_changes_created_idx ON subscription_price_changes(created_at DESC,id DESC);
