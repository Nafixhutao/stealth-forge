ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_auth_method_valid;
DROP INDEX IF EXISTS sessions_account_created_at_idx;
ALTER TABLE sessions DROP COLUMN IF EXISTS auth_method;
