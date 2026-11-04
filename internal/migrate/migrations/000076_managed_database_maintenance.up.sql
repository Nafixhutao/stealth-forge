-- Managed database maintenance state. write_locked records the fair-use
-- enforcement applied by the worker when a free-tier database outgrows its
-- 256 MB budget: write privileges are revoked in MySQL while reads keep
-- working, and the flag is cleared once usage drops back under the limit.
ALTER TABLE managed_databases
  ADD COLUMN write_locked BOOLEAN NOT NULL DEFAULT false;

-- Backup ledger. BackupPath is a validated relative path inside the shared
-- backups volume; credentials are never stored here.
CREATE TABLE managed_database_backups (
  id UUID PRIMARY KEY,
  database_id UUID NOT NULL REFERENCES managed_databases(id) ON DELETE CASCADE,
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  backup_path TEXT NOT NULL,
  size_bytes BIGINT NOT NULL DEFAULT 0,
  source TEXT NOT NULL DEFAULT 'daily',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT managed_database_backups_id_uuidv7 CHECK (substring(id::text FROM 15 FOR 1) = '7'),
  CONSTRAINT managed_database_backups_path_length CHECK (char_length(backup_path) BETWEEN 1 AND 512),
  CONSTRAINT managed_database_backups_source_valid CHECK (source IN ('daily','manual','pre_restore'))
);

CREATE INDEX managed_database_backups_database_idx
  ON managed_database_backups (database_id, created_at DESC);

-- Restore requests are queued by the Console API and executed by the worker,
-- which owns all Docker runtime side effects.
CREATE TABLE managed_database_restores (
  id UUID PRIMARY KEY,
  database_id UUID NOT NULL REFERENCES managed_databases(id) ON DELETE CASCADE,
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  backup_path TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'queued',
  error TEXT NOT NULL DEFAULT '',
  requested_by UUID NOT NULL REFERENCES accounts(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT managed_database_restores_id_uuidv7 CHECK (substring(id::text FROM 15 FOR 1) = '7'),
  CONSTRAINT managed_database_restores_status_valid CHECK (status IN ('queued','running','succeeded','failed')),
  CONSTRAINT managed_database_restores_error_length CHECK (char_length(error) <= 2000)
);

CREATE INDEX managed_database_restores_database_idx
  ON managed_database_restores (database_id, created_at DESC);
CREATE INDEX managed_database_restores_queued_idx
  ON managed_database_restores (created_at) WHERE status = 'queued';