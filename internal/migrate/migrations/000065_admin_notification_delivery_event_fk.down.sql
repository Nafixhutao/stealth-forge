ALTER TABLE admin_notification_deliveries
  DROP CONSTRAINT IF EXISTS admin_notification_deliveries_alert_event_id_fkey;

ALTER TABLE admin_notification_deliveries
  ADD CONSTRAINT admin_notification_deliveries_alert_event_id_fkey
    FOREIGN KEY (alert_event_id) REFERENCES admin_alert_events(id) ON DELETE SET NULL;
