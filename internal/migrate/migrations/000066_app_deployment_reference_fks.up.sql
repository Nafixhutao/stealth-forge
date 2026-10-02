-- App deployment references were declared without foreign keys, so deleting an
-- account or a deployment could leave dangling UUIDs in
-- app_deployments.created_by_account_id, app_deployments.selection_base_deployment_id,
-- and app_runtime_state.applied_deployment_id. Add the missing referential
-- integrity with ON DELETE SET NULL so the deployment row (and its history)
-- survives while the optional reference is cleared.

-- app_deployments_immutable_inputs compares created_by_account_id and
-- selection_base_deployment_id, so the ON DELETE SET NULL actions below would
-- raise "AppDeployment build inputs are immutable" before the reference could
-- be cleared. Permit referential cleanup to clear those two references to NULL
-- while still rejecting any attempt to set or repoint them.
CREATE OR REPLACE FUNCTION prevent_app_deployment_input_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF (NEW.id, NEW.app_id, NEW.project_id, NEW.version, NEW.source, NEW.source_name,
      NEW.source_size_bytes, NEW.source_checksum_sha256, NEW.source_path,
      NEW.dockerfile_path, NEW.context_directory, NEW.target, NEW.platform,
      NEW.workload_spec, NEW.workload_spec_sha256, NEW.select_requested)
     IS DISTINCT FROM
     (OLD.id, OLD.app_id, OLD.project_id, OLD.version, OLD.source, OLD.source_name,
      OLD.source_size_bytes, OLD.source_checksum_sha256, OLD.source_path,
      OLD.dockerfile_path, OLD.context_directory, OLD.target, OLD.platform,
      OLD.workload_spec, OLD.workload_spec_sha256, OLD.select_requested) THEN
    RAISE EXCEPTION 'AppDeployment build inputs are immutable';
  END IF;
  IF NEW.selection_base_deployment_id IS DISTINCT FROM OLD.selection_base_deployment_id
     AND NEW.selection_base_deployment_id IS NOT NULL THEN
    RAISE EXCEPTION 'AppDeployment build inputs are immutable';
  END IF;
  IF NEW.created_by_account_id IS DISTINCT FROM OLD.created_by_account_id
     AND NEW.created_by_account_id IS NOT NULL THEN
    RAISE EXCEPTION 'AppDeployment build inputs are immutable';
  END IF;
  IF OLD.image_digest IS NOT NULL AND
     (NEW.image_digest, NEW.image_archive_sha256, NEW.image_size_bytes, NEW.image_path)
     IS DISTINCT FROM
     (OLD.image_digest, OLD.image_archive_sha256, OLD.image_size_bytes, OLD.image_path) THEN
    RAISE EXCEPTION 'AppDeployment build output is immutable';
  END IF;
  RETURN NEW;
END;
$$;

-- Clear dangling references left behind by the missing constraints before we
-- validate them. The trigger above now permits these NULL writes, so the
-- migration succeeds even on databases that already lost the referenced rows.
UPDATE app_deployments AS d
   SET selection_base_deployment_id = NULL
 WHERE d.selection_base_deployment_id IS NOT NULL
   AND NOT EXISTS (SELECT 1 FROM app_deployments AS p WHERE p.id = d.selection_base_deployment_id);

UPDATE app_deployments AS d
   SET created_by_account_id = NULL
 WHERE d.created_by_account_id IS NOT NULL
   AND NOT EXISTS (SELECT 1 FROM accounts AS a WHERE a.id = d.created_by_account_id);

UPDATE app_runtime_state AS s
   SET applied_deployment_id = NULL
 WHERE s.applied_deployment_id IS NOT NULL
   AND NOT EXISTS (SELECT 1 FROM app_deployments AS d WHERE d.id = s.applied_deployment_id);

ALTER TABLE app_deployments
  ADD CONSTRAINT app_deployments_created_by_account_fk
    FOREIGN KEY (created_by_account_id) REFERENCES accounts(id) ON DELETE SET NULL,
  ADD CONSTRAINT app_deployments_selection_base_fk
    FOREIGN KEY (selection_base_deployment_id) REFERENCES app_deployments(id) ON DELETE SET NULL;

ALTER TABLE app_runtime_state
  ADD CONSTRAINT app_runtime_state_applied_deployment_fk
    FOREIGN KEY (applied_deployment_id) REFERENCES app_deployments(id) ON DELETE SET NULL;
