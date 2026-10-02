-- Delivery rows are durable history. A normal delivery must retain the alert
-- event that produced it, which is exactly what the
-- admin_notification_deliveries_source_valid CHECK requires. The released
-- foreign key used ON DELETE SET NULL, so deleting an alert event that still
-- had a normal delivery would null alert_event_id and then violate that CHECK.
-- RESTRICT states the durable-history intent directly and turns the previous
-- CHECK failure into an explicit referential error.
ALTER TABLE admin_notification_deliveries
  DROP CONSTRAINT IF EXISTS admin_notification_deliveries_alert_event_id_fkey;

ALTER TABLE admin_notification_deliveries
  ADD CONSTRAINT admin_notification_deliveries_alert_event_id_fkey
    FOREIGN KEY (alert_event_id) REFERENCES admin_alert_events(id) ON DELETE RESTRICT;
