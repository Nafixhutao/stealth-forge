-- Production-readiness indexes.
--
-- The admin operations view unions six durable job tables and filters/orders on
-- created_at. Their existing indexes lead with project_id, so a global
-- created_at window cannot use them and the view degrades as history grows.
CREATE INDEX IF NOT EXISTS function_deployments_created_at_idx
    ON function_deployments (created_at DESC);
CREATE INDEX IF NOT EXISTS site_deployments_created_at_idx
    ON site_deployments (created_at DESC);
CREATE INDEX IF NOT EXISTS function_executions_created_at_idx
    ON function_executions (created_at DESC);
CREATE INDEX IF NOT EXISTS agent_runs_created_at_idx
    ON agent_runs (created_at DESC);
CREATE INDEX IF NOT EXISTS artifact_cleanup_jobs_created_at_idx
    ON artifact_cleanup_jobs (created_at DESC);
CREATE INDEX IF NOT EXISTS database_backups_created_at_idx
    ON database_backups (created_at DESC);

-- Foreign-key lookups for notification deliveries. Deleting a channel or an
-- alert event otherwise scans the whole deliveries table to enforce the FK.
CREATE INDEX IF NOT EXISTS admin_notification_deliveries_channel_idx
    ON admin_notification_deliveries (channel_id);
CREATE INDEX IF NOT EXISTS admin_notification_deliveries_alert_event_idx
    ON admin_notification_deliveries (alert_event_id);
