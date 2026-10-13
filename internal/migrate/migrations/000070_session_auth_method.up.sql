-- Record how a Console session was opened so the Admin Console can show each
-- account's most recent sign-in method. Historical rows predate the column and
-- are backfilled as password sessions, which is the only method that existed
-- before external providers were configurable in the Console.
ALTER TABLE sessions
  ADD COLUMN auth_method TEXT NOT NULL DEFAULT 'password';

ALTER TABLE sessions
  ADD CONSTRAINT sessions_auth_method_valid CHECK (auth_method IN ('password', 'github', 'google'));

CREATE INDEX sessions_account_created_at_idx ON sessions (account_id, created_at DESC);
