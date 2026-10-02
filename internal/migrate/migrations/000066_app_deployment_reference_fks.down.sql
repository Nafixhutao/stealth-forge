ALTER TABLE app_runtime_state
  DROP CONSTRAINT IF EXISTS app_runtime_state_applied_deployment_fk;

ALTER TABLE app_deployments
  DROP CONSTRAINT IF EXISTS app_deployments_selection_base_fk,
  DROP CONSTRAINT IF EXISTS app_deployments_created_by_account_fk;

-- Restore the original build-input immutability definition, which compared
-- created_by_account_id and selection_base_deployment_id inline.
CREATE OR REPLACE FUNCTION prevent_app_deployment_input_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF (NEW.id, NEW.app_id, NEW.project_id, NEW.version, NEW.source, NEW.source_name,
      NEW.source_size_bytes, NEW.source_checksum_sha256, NEW.source_path,
      NEW.dockerfile_path, NEW.context_directory, NEW.target, NEW.platform,
      NEW.workload_spec, NEW.workload_spec_sha256, NEW.select_requested, NEW.selection_base_deployment_id,
      NEW.created_by_account_id)
     IS DISTINCT FROM
     (OLD.id, OLD.app_id, OLD.project_id, OLD.version, OLD.source, OLD.source_name,
      OLD.source_size_bytes, OLD.source_checksum_sha256, OLD.source_path,
      OLD.dockerfile_path, OLD.context_directory, OLD.target, OLD.platform,
      OLD.workload_spec, OLD.workload_spec_sha256, OLD.select_requested, OLD.selection_base_deployment_id,
      OLD.created_by_account_id) THEN
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
