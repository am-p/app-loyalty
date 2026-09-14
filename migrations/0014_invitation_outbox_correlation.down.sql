DROP INDEX email_outbox_invitation_pending_idx;
ALTER TABLE email_outbox DROP COLUMN invitation_id;
