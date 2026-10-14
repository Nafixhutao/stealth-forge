-- The Admin Console directory shows each account's subscription plan. Plans
-- are admin-assigned metadata for now (no payment provider); the column is a
-- narrow CHECK so later billing integration cannot introduce unknown values.
ALTER TABLE accounts
  ADD COLUMN plan TEXT NOT NULL DEFAULT 'free';

ALTER TABLE accounts
  ADD CONSTRAINT accounts_plan_valid CHECK (plan IN ('free', 'go', 'plus', 'pro'));

ALTER TABLE accounts
  ADD COLUMN plan_updated_at TIMESTAMPTZ;
