-- Managed MySQL databases are the hosted-database product. Each row owns one
-- MySQL database and one MySQL user on the shared tenant cluster; the control
-- plane stores only metadata and the encrypted credential. The free tier allows
-- one database per project, which is why the project id is unique.
CREATE TABLE managed_databases (
  id UUID PRIMARY KEY,
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  database_name TEXT NOT NULL UNIQUE,
  username TEXT NOT NULL UNIQUE,
  password_ciphertext BYTEA NOT NULL,
  status TEXT NOT NULL DEFAULT 'provisioning',
  connection_limit INTEGER NOT NULL DEFAULT 10,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT managed_databases_id_uuidv7 CHECK (substring(id::text FROM 15 FOR 1) = '7'),
  CONSTRAINT managed_databases_project_unique UNIQUE (project_id),
  CONSTRAINT managed_databases_name_length CHECK (char_length(name) BETWEEN 2 AND 120),
  CONSTRAINT managed_databases_name_trimmed CHECK (name = btrim(name)),
  CONSTRAINT managed_databases_database_name_valid CHECK (database_name ~ '^mdb_[a-f0-9]{24}$'),
  CONSTRAINT managed_databases_username_valid CHECK (username ~ '^u_[a-f0-9]{24}$'),
  CONSTRAINT managed_databases_status_valid CHECK (status IN ('provisioning','ready','failed')),
  CONSTRAINT managed_databases_connection_limit_valid CHECK (connection_limit BETWEEN 1 AND 100)
);

CREATE INDEX managed_databases_project_id_idx ON managed_databases (project_id);
