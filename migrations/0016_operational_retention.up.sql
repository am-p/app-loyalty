ALTER TABLE email_outbox ADD COLUMN redacted_at TIMESTAMPTZ;

CREATE INDEX previews_movimiento_retention_idx ON previews_movimiento(expires_at,id);
CREATE INDEX solicitudes_idempotentes_retention_idx ON solicitudes_idempotentes(completed_at,idempotency_key) WHERE estado='COMPLETED';
CREATE INDEX sesiones_auth_retention_idx ON sesiones_auth(expires_at,id);
CREATE INDEX tokens_identidad_email_retention_idx ON tokens_identidad_email(expires_at,id);
CREATE INDEX email_outbox_retention_idx ON email_outbox(created_at,id) WHERE estado IN ('SENT','FAILED');
