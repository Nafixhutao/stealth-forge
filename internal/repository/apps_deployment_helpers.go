package repository

import (
	"context"
	"encoding/hex"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) auditAppDeploymentTx(ctx context.Context, tx pgx.Tx, projectID uuid.UUID, actor *AppActor, action string, deploymentID uuid.UUID, metadata map[string]any) error {
	organizationID, err := projectOrganizationIDValue(ctx, tx, projectID)
	if err != nil {
		return err
	}
	actorID := uuid.Nil
	if actor != nil {
		if actor.Kind == AppConsoleActor {
			actorID = actor.AccountID
		} else if actor.Kind == AppAPIKeyActor {
			metadata["actor"] = "api_key"
			metadata["api_key_id"] = actor.APIKeyID.String()
		}
	}
	metadata["project_id"] = projectID.String()
	if err := writeAuditMetadata(ctx, tx, organizationID, actorID, action, "app_deployment", deploymentID, metadata); err != nil {
		return err
	}
	return r.enqueueWebhookEventTx(ctx, tx, projectID, action, "app_deployment", deploymentID, metadata)
}

func appDeploymentAuditMetadata(item domain.AppDeployment) map[string]any {
	return map[string]any{
		"app_id": item.AppID, "version": item.Version, "source_checksum_sha256": item.SourceChecksumSHA256,
		"workload_spec_sha256": item.WorkloadSpecSHA256, "image_digest": item.ImageDigest,
		"status": item.Status, "build_status": item.BuildStatus,
	}
}

func queueAppDeploymentArtifactsForDeletionTx(ctx context.Context, tx pgx.Tx, projectID, appID uuid.UUID) error {
	rows, err := tx.Query(ctx, `SELECT source_path,image_path FROM app_deployments WHERE project_id=$1 AND app_id=$2`, projectID, appID)
	if err != nil {
		return err
	}
	type paths struct {
		source string
		image  *string
	}
	items := make([]paths, 0)
	for rows.Next() {
		var item paths
		if err := rows.Scan(&item.source, &item.image); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, item := range items {
		if err := queueArtifactCleanupTx(ctx, tx, ArtifactCleanupInput{ProjectID: projectID, StoreKind: ArtifactCleanupAppSources, Operation: ArtifactCleanupRelative, RelativePath: item.source}); err != nil {
			return err
		}
		if item.image != nil {
			if err := queueArtifactCleanupTx(ctx, tx, ArtifactCleanupInput{ProjectID: projectID, StoreKind: ArtifactCleanupAppImages, Operation: ArtifactCleanupRelative, RelativePath: *item.image}); err != nil {
				return err
			}
		}
	}
	return nil
}

func validAppArtifactPath(value string) bool {
	parts := strings.Split(value, "/")
	if len(parts) != 3 || strings.ContainsAny(value, "\\\x00\r\n") {
		return false
	}
	for _, part := range parts {
		id, err := uuid.Parse(part)
		if err != nil || id == uuid.Nil || id.Version() != uuid.Version(7) || part != id.String() {
			return false
		}
	}
	return true
}

func validAppArtifactPathForApp(value string, projectID, appID uuid.UUID) bool {
	return validAppArtifactPath(value) && strings.HasPrefix(value, projectID.String()+"/"+appID.String()+"/")
}

func validAppSHA256(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func normalizeAppBuildLogMessage(message string) string {
	message = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || unicode.IsControl(r) {
			return ' '
		}
		return r
	}, message)
	message = strings.TrimSpace(message)
	if len(message) <= AppBuildLogMessageMaxBytes {
		return message
	}
	message = message[:AppBuildLogMessageMaxBytes]
	for !utf8.ValidString(message) {
		message = message[:len(message)-1]
	}
	return strings.TrimSpace(message)
}

func valueInt64(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
