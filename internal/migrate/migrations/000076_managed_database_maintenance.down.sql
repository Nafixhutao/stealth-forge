DROP TABLE IF EXISTS managed_database_backups;
DROP TABLE IF EXISTS managed_database_restores;
ALTER TABLE managed_databases DROP COLUMN IF EXISTS write_locked;