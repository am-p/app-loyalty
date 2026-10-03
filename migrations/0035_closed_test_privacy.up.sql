ALTER TABLE usuarios ADD COLUMN adult_confirmed_at TIMESTAMPTZ;
ALTER TABLE usuarios ADD COLUMN adult_policy_version TEXT;
ALTER TABLE historial_movimientos ALTER COLUMN usuario_operador_id DROP NOT NULL;
CREATE TABLE profile_media_deletions (
 object_key TEXT PRIMARY KEY CHECK(object_key LIKE 'profiles/%' AND strpos(object_key,'..')=0),
 requested_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- This journal must also be exported outside database backups before restoring.
CREATE TABLE account_deletion_journal (
 user_id BIGINT PRIMARY KEY,
 deleted_at TIMESTAMPTZ NOT NULL,
 policy_version TEXT NOT NULL DEFAULT 'closed-test-2026-10'
);
