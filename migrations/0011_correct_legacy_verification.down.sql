UPDATE usuarios
SET email_verified_at=created_at
WHERE google_id IS NULL AND password_hash IS NOT NULL AND email_verified_at IS NULL
  AND created_at < (SELECT applied_at FROM schema_migrations WHERE version='0008');

