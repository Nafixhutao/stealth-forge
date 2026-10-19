"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import {
  ArrowLeft,
  ExternalLink,
  GitBranch,
  RefreshCw,
  Rocket,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";
import type { components } from "@/api/generated/schema";
import {
  useActivateSiteDeployment,
  useDeleteSiteDeployment,
} from "@/api/mutations/sites";
import { useSite, useSiteDeployments, useSiteDeploymentLogs } from "@/api/queries/site-detail";
import { ErrorState } from "@/components/feedback/error-state";
import { Button } from "@/components/ui/button";
import { GitDeployDialog } from "@/features/project/git-deploy-dialog";
import { ResourceStatusChip } from "@/features/project/resource-list";
import { SiteSource } from "@/features/project/site-source";
import { PRIMARY_ACTION } from "@/features/user/action-styles";
import { formatBytes, formatDate, pluralize } from "@/lib/format";
import { cn } from "@/lib/utils";

type Deployment = components["schemas"]["SiteDeployment"];

/** Releases that have a published artifact behind them. */
function isPublished(deployment: Deployment) {
  return deployment.build_status === "succeeded" && deployment.status !== "failed";
}

function isBuilding(deployment: Deployment) {
  return deployment.build_status === "queued" || deployment.build_status === "running";
}

/**
 * One Site, as its own surface.
 *
 * Three panels answer the three questions an operator has about a site: what it
 * looks like right now (the preview, which is the live release), what has been
 * published (the release history, with activate and delete), and what is inside
 * a release (the source viewer, read-only because releases are immutable).
 *
 * The preview is framed with `sandbox` and without `allow-same-origin` on
 * purpose: the console serves the preview from its own origin, so an unsandboxed
 * frame would let published site code read the console's session.
 */
export function SiteDetail({
  projectId,
  siteId,
}: {
  projectId: string;
  siteId: string;
}) {
  const site = useSite(projectId, siteId);
  const deployments = useSiteDeployments(projectId, siteId);
  const activate = useActivateSiteDeployment(projectId);
  const remove = useDeleteSiteDeployment(projectId);
  const [gitOpen, setGitOpen] = useState(false);
  const [previewNonce, setPreviewNonce] = useState(0);
  const [selectedDeploymentId, setSelectedDeploymentId] = useState<string | null>(
    null,
  );

  const items = useMemo(
    () => deployments.data?.pages.flatMap((page) => page?.deployments ?? []) ?? [],
    [deployments.data],
  );
  const canManage = deployments.data?.pages[0]?.can_manage ?? false;
  const record = site.data?.site;
  const activeDeploymentId = record?.active_deployment_id ?? null;

  // The source panel follows the operator's choice, falling back to the live
  // release so the panel is useful before anything is clicked.
  const sourceDeploymentId =
    selectedDeploymentId ??
    activeDeploymentId ??
    items.find((item) => isPublished(item))?.id ??
    null;
  const sourceDeployment = items.find((item) => item.id === sourceDeploymentId);

  if (site.isError) {
    return (
      <ErrorState
        error={site.error}
        retry={() => void site.refetch()}
        title="Unable to load this site"
      />
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-start justify-between gap-x-4 gap-y-2 px-4 sm:px-0">
        <div className="flex min-w-0 flex-col gap-1">
          <Link
            href={`/projects/${projectId}/sites`}
            className="inline-flex w-fit items-center gap-1 text-[12.5px] text-[var(--projects-muted)] no-underline transition-colors hover:text-[var(--projects-text)]"
          >
            <ArrowLeft className="size-3.5" strokeWidth={2} aria-hidden="true" />
            All sites
          </Link>
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <h2 className="m-0 truncate text-[17px] font-semibold tracking-[-0.01em] text-[var(--projects-text)] sm:text-[19px]">
              {record?.name ?? "Site"}
            </h2>
            {record ? (
              <ResourceStatusChip
                label={
                  record.status !== "active"
                    ? "Disabled"
                    : activeDeploymentId
                      ? "Active"
                      : "Not deployed"
                }
                tone={
                  record.status !== "active"
                    ? "neutral"
                    : activeDeploymentId
                      ? "positive"
                      : "warning"
                }
              />
            ) : null}
          </div>
          {record?.platform_hostname ? (
            <a
              href={`https://${record.platform_hostname}`}
              target="_blank"
              rel="noreferrer"
              className="inline-flex w-fit items-center gap-1 truncate text-[12.5px] text-[var(--projects-text)] underline decoration-[var(--projects-border-hover)] underline-offset-2"
            >
              {record.platform_hostname}
              <ExternalLink className="size-3 shrink-0" strokeWidth={2} aria-hidden="true" />
            </a>
          ) : (
            <p className="m-0 text-[12.5px] text-[var(--projects-muted)]">
              No public address yet
            </p>
          )}
        </div>

        {canManage ? (
          <Button
            type="button"
            onClick={() => setGitOpen(true)}
            className={PRIMARY_ACTION}
          >
            <GitBranch className="size-4 shrink-0" strokeWidth={2.2} aria-hidden="true" />
            Deploy from Git
          </Button>
        ) : null}
      </div>

      {/* Preview ------------------------------------------------------------ */}
      <section className="flex flex-col gap-2" aria-label="Preview">
        <div className="flex flex-wrap items-center justify-between gap-2 px-4 sm:px-0">
          <h3 className="m-0 text-[13.5px] font-semibold text-[var(--projects-text)]">
            Preview
            <span className="ml-2 font-normal text-[var(--projects-muted)]">
              {activeDeploymentId ? "live release" : "nothing published yet"}
            </span>
          </h3>
          <div className="flex items-center gap-1.5">
            <button
              type="button"
              onClick={() => setPreviewNonce((value) => value + 1)}
              className="inline-flex h-8 items-center gap-1.5 rounded-md border border-[var(--projects-border)] px-2.5 text-[12px] font-medium text-[var(--projects-text)] transition-colors hover:border-[var(--projects-border-hover)] hover:bg-[var(--projects-control)]"
            >
              <RefreshCw className="size-3.5" strokeWidth={2} aria-hidden="true" />
              Reload
            </button>
            <a
              href={`/v1/sites/${siteId}`}
              target="_blank"
              rel="noreferrer"
              className="inline-flex h-8 items-center gap-1.5 rounded-md border border-[var(--projects-border)] px-2.5 text-[12px] font-medium text-[var(--projects-text)] no-underline transition-colors hover:border-[var(--projects-border-hover)] hover:bg-[var(--projects-control)]"
            >
              <ExternalLink className="size-3.5" strokeWidth={2} aria-hidden="true" />
              Open
            </a>
          </div>
        </div>
        <div className="mx-4 overflow-hidden rounded-xl border border-[var(--projects-border)] bg-[var(--projects-card-bg)] sm:mx-0">
          {activeDeploymentId ? (
            <iframe
              key={previewNonce}
              title={`Preview of ${record?.name ?? "site"}`}
              src={`/v1/sites/${siteId}?v=${previewNonce}`}
              sandbox="allow-scripts allow-forms allow-popups allow-modals"
              className="block h-[420px] w-full border-0 bg-white"
            />
          ) : (
            <p className="m-0 p-6 text-center text-[12.5px] text-[var(--projects-muted)]">
              Deploy a release to see the site here.
            </p>
          )}
        </div>
      </section>

      {/* Releases ----------------------------------------------------------- */}
      <section className="flex flex-col gap-2" aria-label="Releases">
        <h3 className="m-0 px-4 text-[13.5px] font-semibold text-[var(--projects-text)] sm:px-0">
          Releases
          <span className="ml-2 font-normal text-[var(--projects-muted)]">
            {pluralize(items.length, "release")}
          </span>
        </h3>

        {deployments.isPending ? (
          <p className="m-0 px-4 text-[12.5px] text-[var(--projects-muted)] sm:px-0">
            Loading releases…
          </p>
        ) : deployments.isError ? (
          <ErrorState
            error={deployments.error}
            retry={() => void deployments.refetch()}
            title="Unable to load releases"
          />
        ) : items.length === 0 ? (
          <p className="m-0 px-4 text-[12.5px] text-[var(--projects-muted)] sm:px-0">
            No releases yet. Upload an archive or deploy from Git to publish one.
          </p>
        ) : (
          <div className="mx-4 overflow-hidden rounded-xl border border-[var(--projects-border)] bg-[var(--projects-card-bg)] sm:mx-0">
            <ul className="m-0 flex list-none flex-col divide-y divide-[var(--projects-divider)] p-0">
              {items.map((item) => (
                <ReleaseRow
                  key={item.id}
                  projectId={projectId}
                  siteId={siteId}
                  deployment={item}
                  isActive={item.id === activeDeploymentId}
                  isSelected={item.id === sourceDeploymentId}
                  canManage={canManage}
                  onSelect={() => setSelectedDeploymentId(item.id)}
                  onActivate={() =>
                    activate.mutate(
                      { siteId, deploymentId: item.id },
                      {
                        onSuccess: () => toast.success("Release is live"),
                        onError: (cause) =>
                          toast.error(
                            cause instanceof Error
                              ? cause.message
                              : "Could not activate this release",
                          ),
                      },
                    )
                  }
                  onDelete={() =>
                    remove.mutate(
                      { siteId, deploymentId: item.id },
                      {
                        onSuccess: () => {
                          toast.success("Release deleted");
                          setSelectedDeploymentId(null);
                        },
                        onError: (cause) =>
                          toast.error(
                            cause instanceof Error
                              ? cause.message
                              : "Could not delete this release",
                          ),
                      },
                    )
                  }
                  busy={activate.isPending || remove.isPending}
                />
              ))}
            </ul>
          </div>
        )}
      </section>

      {/* Source ------------------------------------------------------------- */}
      <section className="flex flex-col gap-2" aria-label="Source">
        <h3 className="m-0 flex flex-wrap items-center gap-2 px-4 text-[13.5px] font-semibold text-[var(--projects-text)] sm:px-0">
          Source
          <span className="font-normal text-[var(--projects-muted)]">
            {sourceDeployment
              ? `release ${sourceDeployment.version}`
              : "no published release"}
          </span>
          <span className="rounded-full border border-[var(--projects-border)] px-2 py-0.5 text-[11px] font-normal text-[var(--projects-muted)]">
            read-only
          </span>
        </h3>
        {sourceDeploymentId && sourceDeployment && isPublished(sourceDeployment) ? (
          <div className="mx-4 sm:mx-0">
            <SiteSource
              key={sourceDeploymentId}
              projectId={projectId}
              siteId={siteId}
              deploymentId={sourceDeploymentId}
              canEdit={canManage}
              onPublished={(published) => {
                // Follow the release the save created, and reload the preview
                // because it is now the live one.
                setSelectedDeploymentId(published.id);
                setPreviewNonce((value) => value + 1);
                void deployments.refetch();
                void site.refetch();
              }}
            />
          </div>
        ) : (
          <p className="m-0 px-4 text-[12.5px] text-[var(--projects-muted)] sm:px-0">
            The source of a release appears here once it has finished building.
            A release is immutable, so changing it means deploying again.
          </p>
        )}
      </section>

      {canManage && record ? (
        <GitDeployDialog
          projectId={projectId}
          siteId={siteId}
          siteName={record.name}
          open={gitOpen}
          onOpenChange={setGitOpen}
        />
      ) : null}
    </div>
  );
}

function ReleaseRow({
  projectId,
  siteId,
  deployment,
  isActive,
  isSelected,
  canManage,
  busy,
  onSelect,
  onActivate,
  onDelete,
}: {
  projectId: string;
  siteId: string;
  deployment: Deployment;
  isActive: boolean;
  isSelected: boolean;
  canManage: boolean;
  busy: boolean;
  onSelect: () => void;
  onActivate: () => void;
  onDelete: () => void;
}) {
  const [logsOpen, setLogsOpen] = useState(false);
  const building = isBuilding(deployment);
  // Polls only while the build is still moving; a finished release is read once.
  const logs = useSiteDeploymentLogs(
    projectId,
    siteId,
    logsOpen ? deployment.id : undefined,
    building,
  );
  const lines = logs.data?.logs ?? [];

  const source =
    deployment.source === "upload"
      ? (deployment.source_name ?? "archive upload")
      : (deployment.git_repository ?? "git");

  return (
    <li
      className={cn(
        "flex flex-col gap-1 px-4 py-3 transition-colors",
        isSelected && "bg-[var(--projects-control)]",
      )}
    >
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-[13.5px] font-semibold text-[var(--projects-text)]">
          v{deployment.version}
        </span>
        {isActive ? <ResourceStatusChip label="Live" tone="positive" /> : null}
        {deployment.build_status !== "succeeded" ? (
          <ResourceStatusChip
            label={
              deployment.build_status === "failed"
                ? "Build failed"
                : `Build ${deployment.build_status}`
            }
            tone={deployment.build_status === "failed" ? "warning" : "neutral"}
          />
        ) : null}
        {deployment.status === "superseded" ? (
          <ResourceStatusChip label="Superseded" tone="neutral" />
        ) : null}
      </div>
      <p className="m-0 truncate text-[12.5px] text-[var(--projects-muted)]">
        {source} · {formatBytes(deployment.size_bytes)} ·{" "}
        {formatDate(deployment.created_at)}
      </p>
      {deployment.error_message ? (
        <p className="m-0 text-[12px] text-[var(--projects-danger)]">
          {deployment.error_message}
        </p>
      ) : null}

      <div className="mt-1 flex flex-wrap items-center gap-1.5 [&>*]:shrink-0">
        <button
          type="button"
          onClick={onSelect}
          className="inline-flex h-8 items-center rounded-md border border-[var(--projects-border)] px-2 text-[12px] font-medium text-[var(--projects-text)] transition-colors hover:border-[var(--projects-border-hover)] hover:bg-[var(--projects-control)]"
        >
          View source
        </button>
        <button
          type="button"
          onClick={() => setLogsOpen((value) => !value)}
          aria-expanded={logsOpen}
          className="inline-flex h-8 items-center rounded-md border border-[var(--projects-border)] px-2 text-[12px] font-medium text-[var(--projects-text)] transition-colors hover:border-[var(--projects-border-hover)] hover:bg-[var(--projects-control)]"
        >
          {logsOpen ? "Hide build log" : "Build log"}
        </button>
        {canManage && !isActive && deployment.build_status === "succeeded" ? (
          <button
            type="button"
            onClick={onActivate}
            disabled={busy}
            className="inline-flex h-8 items-center gap-1.5 rounded-md border border-[var(--projects-border)] px-2 text-[12px] font-medium text-[var(--projects-text)] transition-colors hover:border-[var(--projects-border-hover)] hover:bg-[var(--projects-control)] disabled:opacity-50"
          >
            <Rocket className="size-3.5" strokeWidth={2} aria-hidden="true" />
            Make live
          </button>
        ) : null}
        {canManage && !isActive ? (
          <button
            type="button"
            onClick={onDelete}
            disabled={busy}
            aria-label={`Delete release ${deployment.version}`}
            className="inline-flex size-8 items-center justify-center rounded-md border border-[var(--projects-border)] text-[var(--projects-muted)] transition-colors hover:border-[var(--projects-danger)]/50 hover:bg-[var(--projects-danger)]/10 hover:text-[var(--projects-danger)] disabled:opacity-50"
          >
            <Trash2 className="size-3.5" strokeWidth={2} aria-hidden="true" />
          </button>
        ) : null}
      </div>

      {logsOpen ? (
        <div className="mt-2 max-h-[220px] overflow-auto rounded-lg border border-[var(--projects-divider)] bg-[var(--projects-control)] p-2.5">
          {logs.isPending ? (
            <p className="m-0 text-[12px] text-[var(--projects-muted)]">
              Reading the build log…
            </p>
          ) : logs.isError ? (
            <p className="m-0 text-[12px] text-[var(--projects-danger)]">
              Could not read the build log.
            </p>
          ) : lines.length === 0 ? (
            <p className="m-0 text-[12px] text-[var(--projects-muted)]">
              {building
                ? "Waiting for the first build line…"
                : "This release has no build log."}
            </p>
          ) : (
            <ol className="m-0 flex list-none flex-col gap-0.5 p-0 font-mono text-[11.5px] leading-4">
              {lines.map((line) => (
                <li
                  key={line.id}
                  className={cn(
                    "whitespace-pre-wrap break-words",
                    line.level === "error"
                      ? "text-[var(--projects-danger)]"
                      : line.level === "warn"
                        ? "text-[var(--projects-warning)]"
                        : "text-[var(--projects-text)]",
                  )}
                >
                  {line.message}
                </li>
              ))}
            </ol>
          )}
        </div>
      ) : null}
    </li>
  );
}
