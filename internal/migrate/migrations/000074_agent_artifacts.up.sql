-- Agent artifacts are the files a run produces (Word, PDF). The blob lives in
-- the configured Storage backend; this table owns ownership, version, and the
-- opaque relative path.
CREATE TABLE agent_artifacts (
  id UUID PRIMARY KEY,
  project_id UUID NOT NULL,
  agent_id UUID NOT NULL,
  run_id UUID NOT NULL,
  kind TEXT NOT NULL DEFAULT 'document',
  title TEXT NOT NULL,
  version INT NOT NULL DEFAULT 1,
  mime_type TEXT NOT NULL,
  storage_path TEXT NOT NULL,
  size_bytes BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT agent_artifacts_id_uuidv7 CHECK (substring(id::text FROM 15 FOR 1) = '7'),
  CONSTRAINT agent_artifacts_project_run_fk FOREIGN KEY (project_id, run_id)
    REFERENCES agent_runs(project_id, id) ON DELETE CASCADE,
  CONSTRAINT agent_artifacts_project_agent_fk FOREIGN KEY (project_id, agent_id)
    REFERENCES project_agents(project_id, id) ON DELETE CASCADE,
  CONSTRAINT agent_artifacts_title_valid CHECK (
    char_length(title) BETWEEN 1 AND 200 AND title = btrim(title)
  ),
  CONSTRAINT agent_artifacts_kind_valid CHECK (kind IN ('document')),
  CONSTRAINT agent_artifacts_version_valid CHECK (version >= 1),
  CONSTRAINT agent_artifacts_mime_valid CHECK (
    char_length(mime_type) BETWEEN 1 AND 200 AND mime_type !~ '[[:cntrl:]]'
  ),
  CONSTRAINT agent_artifacts_path_valid CHECK (
    char_length(storage_path) BETWEEN 1 AND 1000 AND storage_path !~ '[[:cntrl:]]'
  ),
  CONSTRAINT agent_artifacts_size_valid CHECK (size_bytes >= 0),
  CONSTRAINT agent_artifacts_unique_version UNIQUE (agent_id, kind, mime_type, version)
);

CREATE INDEX agent_artifacts_project_agent_created_idx
  ON agent_artifacts (project_id, agent_id, created_at DESC, id DESC);
CREATE INDEX agent_artifacts_project_agent_id_idx
  ON agent_artifacts (project_id, agent_id, id DESC);
CREATE INDEX agent_artifacts_project_run_idx
  ON agent_artifacts (project_id, run_id, created_at DESC);
