CREATE TABLE subscription_prices (
  program_type TEXT PRIMARY KEY CHECK (program_type IN ('SELLOS','PUNTOS')),
  unit_price_minor BIGINT NOT NULL CHECK (unit_price_minor BETWEEN 1 AND 9007199254740991),
  version BIGINT NOT NULL CHECK (version > 0),
  updated_by BIGINT NOT NULL REFERENCES backoffice_users(id),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE suscripciones_marca
  ADD COLUMN program_type TEXT CHECK (program_type IN ('SELLOS','PUNTOS')),
  ADD COLUMN price_valid_from TIMESTAMPTZ;
UPDATE suscripciones_marca s SET program_type=p.tipo,price_valid_from=s.created_at
FROM (SELECT DISTINCT ON (marca_id) marca_id,tipo FROM programas_fidelidad ORDER BY marca_id,activo DESC,created_at DESC,id DESC) p
WHERE p.marca_id=s.marca_id;

CREATE TABLE subscription_price_changes (
  id UUID PRIMARY KEY,
  program_type TEXT NOT NULL CHECK (program_type IN ('SELLOS','PUNTOS')),
  unit_price_minor BIGINT NOT NULL CHECK (unit_price_minor > 0),
  previous_version BIGINT NOT NULL,
  price_version BIGINT NOT NULL,
  include_existing BOOLEAN NOT NULL,
  admin_id BIGINT NOT NULL REFERENCES backoffice_users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE subscription_price_change_items (
  change_id UUID NOT NULL REFERENCES subscription_price_changes(id),
  brand_id BIGINT NOT NULL REFERENCES marcas(id),
  provider_subscription_id TEXT NOT NULL,
  external_reference TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','APPLIED','SKIPPED','FAILED')),
  amount_minor BIGINT,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY(change_id,brand_id)
);
CREATE INDEX subscription_price_change_pending_idx ON subscription_price_change_items(updated_at) WHERE status='PENDING';
CREATE TABLE subscription_price_history (
  id BIGSERIAL PRIMARY KEY,
  provider_subscription_id TEXT NOT NULL,
  unit_price_minor BIGINT NOT NULL,
  full_unit_price_minor BIGINT NOT NULL,
  branches BIGINT NOT NULL,
  valid_from TIMESTAMPTZ NOT NULL,
  valid_until TIMESTAMPTZ NOT NULL,
  change_id UUID NOT NULL REFERENCES subscription_price_changes(id)
);
CREATE INDEX subscription_price_history_provider_idx ON subscription_price_history(provider_subscription_id,valid_until);
