DROP INDEX sesiones_auth_family_idx;
ALTER TABLE sesiones_auth
  DROP CONSTRAINT sesiones_auth_family_fk,
  DROP COLUMN auth_time,
  DROP COLUMN family_id;
