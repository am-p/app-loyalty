WITH legacy_password_users AS (
  SELECT u.id
  FROM usuarios u
  WHERE u.google_id IS NULL
    AND u.password_hash IS NOT NULL
    AND u.created_at < (SELECT applied_at FROM schema_migrations WHERE version='0008')
)
UPDATE usuarios u
SET email_verified_at=NULL,auth_version=auth_version+1
FROM legacy_password_users legacy
WHERE u.id=legacy.id;

UPDATE sesiones_auth s
SET revoked_at=COALESCE(s.revoked_at,now())
FROM usuarios u
WHERE u.id=s.usuario_id AND u.email_verified_at IS NULL;

