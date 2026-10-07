-- External sign-in providers are instance-level settings with a per-provider
-- encrypted client secret. Storing them in the database lets an operator
-- configure sign-in from the Console instead of editing deployment files and
-- restarting the API.
CREATE TABLE instance_oauth_providers (
  provider TEXT PRIMARY KEY,
  client_id TEXT NOT NULL,
  client_secret_ciphertext BYTEA NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT instance_oauth_providers_provider_valid CHECK (
    provider IN ('github', 'google')
  ),
  CONSTRAINT instance_oauth_providers_client_id_valid CHECK (
    char_length(client_id) BETWEEN 1 AND 256 AND
    client_id = btrim(client_id)
  ),
  CONSTRAINT instance_oauth_providers_secret_present CHECK (
    octet_length(client_secret_ciphertext) > 0
  )
);
