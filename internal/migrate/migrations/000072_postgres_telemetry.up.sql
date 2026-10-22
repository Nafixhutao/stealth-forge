-- PostgreSQL is the default telemetry store when no external ClickHouse
-- address is configured. Root request metadata already has the shape needed
-- for bounded logs, traces, error groups, and HTTP metrics. Instance-level
-- requests have no tenant scope, so the old tenant-only constraint must not
-- reject them.
ALTER TABLE http_traces
  DROP CONSTRAINT IF EXISTS http_traces_scope_check;

CREATE INDEX IF NOT EXISTS http_traces_started_at_idx
  ON http_traces (started_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS http_traces_status_started_at_idx
  ON http_traces (status, started_at DESC);

CREATE INDEX IF NOT EXISTS http_traces_trace_id_started_at_idx
  ON http_traces (trace_id, started_at DESC);
