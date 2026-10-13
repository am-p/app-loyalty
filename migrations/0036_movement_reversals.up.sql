CREATE TABLE reversiones_movimiento (
  id BIGSERIAL PRIMARY KEY,
  operation_id UUID NOT NULL UNIQUE,
  movimiento_id BIGINT NOT NULL UNIQUE REFERENCES historial_movimientos(id) ON DELETE CASCADE,
  usuario_operador_id BIGINT NOT NULL REFERENCES usuarios(id),
  saldo_anterior BIGINT NOT NULL CHECK (saldo_anterior >= 0),
  saldo_posterior BIGINT NOT NULL CHECK (saldo_posterior >= 0),
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX reversiones_movimiento_operador_idx
  ON reversiones_movimiento(usuario_operador_id, occurred_at DESC);
