CREATE TABLE web_push_keys (
  id BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (id),
  public_key TEXT NOT NULL,
  private_ciphertext BYTEA NOT NULL,
  nonce BYTEA NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE web_push_subscriptions (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  usuario_id BIGINT NOT NULL REFERENCES usuarios(id) ON DELETE CASCADE,
  endpoint_hash TEXT NOT NULL UNIQUE,
  ciphertext BYTEA NOT NULL,
  nonce BYTEA NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX web_push_subscriptions_customer_idx ON web_push_subscriptions(usuario_id);
CREATE TABLE web_push_notifications (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  subscription_id BIGINT NOT NULL REFERENCES web_push_subscriptions(id) ON DELETE CASCADE,
  operation_id UUID NOT NULL,
  card_id BIGINT NOT NULL REFERENCES tarjetas(id) ON DELETE CASCADE,
  estado TEXT NOT NULL DEFAULT 'PENDING' CHECK (estado IN ('PENDING','SENDING','SENT','FAILED')),
  intentos INTEGER NOT NULL DEFAULT 0,
  disponible_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  lease_until TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(subscription_id,operation_id)
);
CREATE INDEX web_push_notifications_claim_idx ON web_push_notifications(disponible_at,id) WHERE estado IN ('PENDING','SENDING');
