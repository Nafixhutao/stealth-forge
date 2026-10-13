-- Reverting would fail while any Google identity is linked, which is the
-- correct behavior: unlink those identities before rolling the schema back.
ALTER TABLE account_identities
  DROP CONSTRAINT account_identities_provider_valid;

ALTER TABLE account_identities
  ADD CONSTRAINT account_identities_provider_valid CHECK (provider IN ('github'));
