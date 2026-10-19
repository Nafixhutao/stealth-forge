"use client";

import { useState } from "react";
import { Globe, Plus, Upload } from "lucide-react";
import type { components } from "@/api/generated/schema";
import { useProjectSites, useProjectUsage } from "@/api/queries/projects";
import { EmptyState } from "@/components/feedback/empty-state";
import { ErrorState } from "@/components/feedback/error-state";
import { Button } from "@/components/ui/button";
import { CreateSiteDialog } from "@/features/project/create-site-dialog";
import { DeploySiteDialog } from "@/features/project/deploy-site-dialog";
import {
  RESOURCE_BOX,
  RESOURCE_CELL,
  RESOURCE_HEAD,
  ResourceIdentity,
  ResourceListFooter,
  ResourceListSkeleton,
  ResourceSectionHeader,
  ResourceStatusChip,
} from "@/features/project/resource-list";
import { PRIMARY_ACTION } from "@/features/user/action-styles";
import { formatBytes, formatDate, pluralize } from "@/lib/format";

type Site = components["schemas"]["Site"];

/**
 * The state that matters for one site: a disabled site is disabled, an enabled
 * site with no release has nothing to answer with, and only an enabled site with
 * an active release is actually live.
 */
function SiteState({ site }: { site: Site }) {
  if (site.status !== "active") {
    return <ResourceStatusChip label="Disabled" tone="neutral" />;
  }
  return site.active_deployment_id ? (
    <ResourceStatusChip label="Active" tone="positive" />
  ) : (
    <ResourceStatusChip label="Not deployed" tone="warning" />
  );
}

/**
 * The Sites section: every site in the project, with the artifact use the usage
 * snapshot reports for the project as a whole.
 *
 * The address column is the point of the row — a site without a platform
 * hostname has nowhere public to answer, and saying so beats an empty cell. A
 * site with an address still serves nothing until a deployment is published, so
 * an undeployed site says that too.
 *
 * Create and Deploy follow `can_manage` from the list itself: only owners and
 * admins may write, and a viewer is shown the list rather than a control that
 * would answer 403.
 */
export function ProjectSites({ projectId }: { projectId: string }) {
  const sites = useProjectSites(projectId);
  const usage = useProjectUsage(projectId);
  const [createOpen, setCreateOpen] = useState(false);
  const [deployTarget, setDeployTarget] = useState<Site | null>(null);

  const items = sites.data?.pages.flatMap((page) => page?.sites ?? []) ?? [];
  const canManage = sites.data?.pages[0]?.can_manage ?? false;
  const snapshot = usage.data?.usage;
  const description = snapshot
    ? `${pluralize(snapshot.site_count, "site")} · ${formatBytes(
        snapshot.site_artifact_bytes,
      )} of ${formatBytes(snapshot.site_quota_bytes)}`
    : "The project's sites.";

  const createButton = (
    <Button
      type="button"
      onClick={() => setCreateOpen(true)}
      className={PRIMARY_ACTION}
    >
      <Plus className="size-4 shrink-0" strokeWidth={2.2} aria-hidden="true" />
      Create site
    </Button>
  );

  return (
    <div className="flex flex-col gap-4">
      <ResourceSectionHeader
        title="Sites"
        description={description}
        action={canManage && items.length > 0 ? createButton : undefined}
      />

      {sites.isPending ? (
        <ResourceListSkeleton />
      ) : sites.isError ? (
        <ErrorState
          error={sites.error}
          retry={() => void sites.refetch()}
          title="Unable to load sites"
        />
      ) : items.length === 0 ? (
        <EmptyState
          className="mx-4 py-10 sm:mx-0 sm:py-14"
          icon={Globe}
          title="No sites in this project yet"
          description="A site serves built static files from one address. Create one, then upload a built site to publish it."
          action={canManage ? createButton : undefined}
        />
      ) : (
        <div className={RESOURCE_BOX}>
          <table className="w-full table-fixed border-collapse text-left">
            <thead>
              <tr className="border-b border-[var(--projects-divider)]">
                <th scope="col" className={RESOURCE_HEAD}>
                  Site
                </th>
                <th
                  scope="col"
                  className={`${RESOURCE_HEAD} hidden w-[38%] sm:table-cell`}
                >
                  Address
                </th>
                {canManage ? (
                  <th
                    scope="col"
                    className={`${RESOURCE_HEAD} w-[44px] px-2 sm:w-[104px] sm:px-5`}
                  >
                    <span className="sr-only">Actions</span>
                  </th>
                ) : null}
              </tr>
            </thead>
            <tbody>
              {items.map((site) => (
                <tr
                  key={site.id}
                  className="border-b border-[var(--projects-divider)] transition-colors hover:bg-[var(--projects-control)]"
                >
                  <td className={RESOURCE_CELL}>
                    <ResourceIdentity
                      name={site.name}
                      meta={
                        <>
                          <SiteState site={site} />
                          <span className="min-w-0">{`Created ${formatDate(
                            site.created_at,
                          )}`}</span>
                        </>
                      }
                    />
                  </td>
                  <td className={`${RESOURCE_CELL} hidden sm:table-cell`}>
                    <span className="flex min-w-0 flex-col gap-0.5">
                      {site.platform_hostname ? (
                        <a
                          href={`https://${site.platform_hostname}`}
                          target="_blank"
                          rel="noreferrer"
                          title={site.platform_hostname}
                          className="block truncate text-[13px] font-medium text-[var(--projects-text)] underline decoration-[var(--projects-border-hover)] underline-offset-2 transition-colors hover:decoration-[var(--projects-text)]"
                        >
                          {site.platform_hostname}
                        </a>
                      ) : (
                        <span className="block truncate text-[12.5px] leading-5 text-[var(--projects-muted)] sm:text-[13px]">
                          No address yet
                        </span>
                      )}
                      <span className="block truncate text-[12.5px] leading-5 text-[var(--projects-muted)] sm:text-[13px]">
                        {`${formatBytes(site.artifact_used_bytes)} of ${formatBytes(
                          site.artifact_quota_bytes,
                        )}`}
                      </span>
                    </span>
                  </td>
                  {canManage ? (
                    <td className={`${RESOURCE_CELL} text-right`}>
                      <button
                        type="button"
                        onClick={() => setDeployTarget(site)}
                        title={`Deploy to ${site.name}`}
                        aria-label={`Deploy to ${site.name}`}
                        className="inline-flex h-7 items-center gap-1.5 rounded-md border border-[var(--projects-border)] px-2.5 text-[12px] font-medium text-[var(--projects-text)] transition-colors hover:border-[var(--projects-border-hover)] hover:bg-[var(--projects-control)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--user-accent)]/40"
                      >
                        <Upload
                          className="size-3.5 shrink-0"
                          strokeWidth={2}
                          aria-hidden="true"
                        />
                        {/* The row is tight at 360px; the glyph and the title
                            carry the action there and the label returns at sm. */}
                        <span className="hidden sm:inline">Deploy</span>
                      </button>
                    </td>
                  ) : null}
                </tr>
              ))}
            </tbody>
          </table>
          <ResourceListFooter
            shown={items.length}
            noun="site"
            hasMore={Boolean(sites.hasNextPage)}
            isLoadingMore={sites.isFetchingNextPage}
            onLoadMore={() => void sites.fetchNextPage()}
          />
        </div>
      )}

      {canManage ? (
        <>
          <CreateSiteDialog
            projectId={projectId}
            open={createOpen}
            onOpenChange={setCreateOpen}
            // Creating a site and then leaving it empty is half a task: the
            // dialog hands the new site straight to the deploy step.
            onCreated={setDeployTarget}
          />
          {deployTarget ? (
            <DeploySiteDialog
              projectId={projectId}
              site={deployTarget}
              open
              onOpenChange={(next) => {
                if (!next) setDeployTarget(null);
              }}
            />
          ) : null}
        </>
      ) : null}
    </div>
  );
}
