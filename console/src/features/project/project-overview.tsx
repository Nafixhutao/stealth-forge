"use client";

import {
  Database,
  Globe,
  HardDrive,
  Users,
  Zap,
} from "lucide-react";
import {
  useProject,
  useProjectUsage,
  useProjectUsageMetering,
} from "@/api/queries/projects";
import { ErrorState } from "@/components/feedback/error-state";
import { projectPath } from "@/features/project/paths";
import { OverviewTile } from "@/features/project/overview-tile";
import { UsageChart } from "@/features/project/usage-chart";
import { formatBytes, formatCount, formatDate, pluralize } from "@/lib/format";

/**
 * The project's Overview: what the project currently holds, and what it has
 * served over the metering window. Every figure comes from the usage snapshot,
 * so the page states facts the API already tracks rather than estimates.
 *
 * The counts are the project's own resources; the rolling 30-day counters are
 * labelled as such, because a bare "Requests" figure on a project page is
 * otherwise read as all-time.
 */

const TILE_GRID = "grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-5";

export function ProjectOverview({ projectId }: { projectId: string }) {
  const project = useProject(projectId);
  const usage = useProjectUsage(projectId);
  const metering = useProjectUsageMetering(projectId);

  if (usage.isError) {
    return (
      <ErrorState
        error={usage.error}
        retry={() => void usage.refetch()}
        title="Unable to load project usage"
      />
    );
  }

  const snapshot = usage.data?.usage;
  const loading = usage.isPending;
  const created = project.data?.project.created_at;

  return (
    <div className="flex flex-col gap-4 px-4 sm:px-0">
      <section
        aria-label="Project usage"
        className={TILE_GRID}
      >
        <OverviewTile
          label="Databases"
          icon={Database}
          loading={loading}
          value={formatCount(snapshot?.database_count ?? 0)}
          detail={
            snapshot
              ? `${pluralize(snapshot.database_table_count, "table")} · ${pluralize(snapshot.database_row_count, "row")}`
              : undefined
          }
          href={projectPath(projectId, "databases")}
        />
        <OverviewTile
          label="Storage"
          icon={HardDrive}
          loading={loading}
          value={formatCount(snapshot?.storage_file_count ?? 0)}
          detail={
            snapshot
              ? `${formatBytes(snapshot.storage_bytes)} of ${formatBytes(snapshot.storage_quota_bytes)}`
              : undefined
          }
          href={projectPath(projectId, "storage")}
        />
        <OverviewTile
          label="Functions"
          icon={Zap}
          loading={loading}
          value={formatCount(snapshot?.function_count ?? 0)}
          detail={
            snapshot
              ? `${formatCount(snapshot.function_invocation_count_30d)} runs in 30 days`
              : undefined
          }
          href={projectPath(projectId, "functions")}
        />
        <OverviewTile
          label="Sites"
          icon={Globe}
          loading={loading}
          value={formatCount(snapshot?.site_count ?? 0)}
          detail={
            snapshot
              ? `${formatBytes(snapshot.site_artifact_bytes)} of ${formatBytes(snapshot.site_quota_bytes)}`
              : undefined
          }
          href={projectPath(projectId, "sites")}
        />
        <OverviewTile
          label="Application users"
          icon={Users}
          loading={loading}
          value={formatCount(snapshot?.application_users ?? 0)}
          detail="Accounts registered through this project"
        />
      </section>

      <section
        aria-label="Request metering"
        className="rounded-[13px] border border-[var(--projects-border)] bg-[var(--projects-card-bg)] px-4 py-3.5"
      >
        {metering.isError ? (
          <ErrorState
            error={metering.error}
            retry={() => void metering.refetch()}
            title="Unable to load metering"
          />
        ) : metering.data ? (
          <UsageChart metering={metering.data} />
        ) : (
          <div className="flex h-[104px] items-end gap-[2px]" aria-hidden="true">
            {Array.from({ length: 30 }).map((_, index) => (
              <span
                key={index}
                style={{ height: `${20 + ((index * 37) % 60)}%` }}
                className="w-full flex-1 animate-pulse rounded-[2px] bg-[var(--projects-control)] motion-reduce:animate-none"
              />
            ))}
          </div>
        )}
      </section>

      <section
        aria-label="Project details"
        className="flex flex-wrap items-center gap-x-4 gap-y-1 text-[12px] leading-5 text-[var(--projects-muted)]"
      >
        {created ? <span>Created {formatDate(created)}</span> : null}
        {snapshot ? (
          <>
            <span>
              {formatCount(snapshot.api_request_count_30d)} API requests in 30 days
            </span>
            <span>{formatBytes(snapshot.api_egress_bytes_30d)} egress</span>
          </>
        ) : null}
      </section>
    </div>
  );
}
