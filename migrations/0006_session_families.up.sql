ALTER TABLE sesiones_auth
  ADD COLUMN family_id UUID,
  ADD COLUMN auth_time TIMESTAMPTZ;
UPDATE sesiones_auth SET family_id=id,auth_time=created_at;
ALTER TABLE sesiones_auth
  ALTER COLUMN family_id SET NOT NULL,
  ALTER COLUMN auth_time SET NOT NULL,
  ADD CONSTRAINT sesiones_auth_family_fk FOREIGN KEY (family_id) REFERENCES sesiones_auth(id) ON DELETE CASCADE;

CREATE INDEX sesiones_auth_family_idx ON sesiones_auth(family_id);
