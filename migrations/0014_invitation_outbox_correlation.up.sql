ALTER TABLE email_outbox ADD COLUMN invitation_id UUID REFERENCES invitaciones_marca(id) ON DELETE SET NULL;
CREATE INDEX email_outbox_invitation_pending_idx ON email_outbox(invitation_id) WHERE tipo='BRAND_INVITATION' AND estado IN ('PENDING','SENDING');
