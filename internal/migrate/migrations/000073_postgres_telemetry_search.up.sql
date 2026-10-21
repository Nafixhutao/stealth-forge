-- Admin log search is a bounded substring search. A trigram index keeps it
-- usable without turning the request path into a full-table scan.
CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;
CREATE INDEX IF NOT EXISTS http_traces_route_trgm_idx
  ON http_traces USING gin (route public.gin_trgm_ops);
