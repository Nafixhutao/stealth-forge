-- External sign-in identities are no longer GitHub-only. Google OAuth was
-- added for browser login, but account_identities still rejected every
-- provider except GitHub, so a Google identity could authenticate yet never be
-- linked. Widen the constraint to the providers the instance supports.
ALTER TABLE account_identities
  DROP CONSTRAINT account_identities_provider_valid;

ALTER TABLE account_identities
  ADD CONSTRAINT account_identities_provider_valid CHECK (provider IN ('github', 'google'));
