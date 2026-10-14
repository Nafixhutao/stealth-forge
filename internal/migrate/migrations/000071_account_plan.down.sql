ALTER TABLE accounts DROP COLUMN IF EXISTS plan_updated_at;
ALTER TABLE accounts DROP CONSTRAINT IF EXISTS accounts_plan_valid;
ALTER TABLE accounts DROP COLUMN IF EXISTS plan;
