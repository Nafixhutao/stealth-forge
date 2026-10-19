"use client";

import { HardDrive } from "lucide-react";
import { useProjectBuckets, useProjectUsage } from "@/api/queries/projects";
import { EmptyState } from "@/components/feedback/empty-state";
import { ErrorState } from "@/components/feedback/error-state";
import { Mono } from "@/components/panel";
import {
  RESOURCE_BOX,
  RESOURCE_CELL,
  RESOURCE_HEAD,
  ResourceIdentity,
  ResourceListFooter,
  ResourceListSkeleton,
  ResourceSectionHeader,
} from "@/features/project/resource-list";
import { formatBytes, formatDate, pluralize } from "@/lib/format";

/**
 * The Storage section: every bucket in the project, with the file count and
 * bytes the usage snapshot reports for the project as a whole.
 *
 * A bucket's files are listed one bucket at a time, so this stays a list: the
 * totals above it are the project's, and they are labelled as such rather than
 * attributed to any one row. This slice is read-only: the console has no bucket
 * create or delete, so rows carry no controls.
 */
export function ProjectStorage({ projectId }: { projectId: string }) {
  const buckets = useProjectBuckets(projectId);
  const usage = useProjectUsage(projectId);

  const items =
    buckets.data?.pages.flatMap((page) => page?.buckets ?? []) ?? [];
  const snapshot = usage.data?.usage;
  const description = snapshot
    ? `${pluralize(snapshot.storage_file_count, "file")} · ${formatBytes(
        snapshot.storage_bytes,
      )} of ${formatBytes(snapshot.storage_quota_bytes)}`
    : "The project's storage buckets.";

  return (
    <div className="flex flex-col gap-4">
      <ResourceSectionHeader title="Storage" description={description} />

      {buckets.isPending ? (
        <ResourceListSkeleton />
      ) : buckets.isError ? (
        <ErrorState
          error={buckets.error}
          retry={() => void buckets.refetch()}
          title="Unable to load buckets"
        />
      ) : items.length === 0 ? (
        <EmptyState
          className="mx-4 py-10 sm:mx-0 sm:py-14"
          icon={HardDrive}
          title="No buckets in this project yet"
          description="A bucket holds the project's files. Create one from the API to see it listed here."
        />
      ) : (
        <div className={RESOURCE_BOX}>
          <table className="w-full table-fixed border-collapse text-left">
            <thead>
              <tr className="border-b border-[var(--projects-divider)]">
                <th scope="col" className={RESOURCE_HEAD}>
                  Bucket
                </th>
                <th
                  scope="col"
                  className={`${RESOURCE_HEAD} w-[46%] sm:w-[32%]`}
                >
                  Used
                </th>
                <th
                  scope="col"
                  className={`${RESOURCE_HEAD} hidden sm:table-cell sm:w-[34%]`}
                >
                  Bucket ID
                </th>
              </tr>
            </thead>
            <tbody>
              {items.map((bucket) => (
                <tr
                  key={bucket.id}
                  className="border-b border-[var(--projects-divider)] transition-colors hover:bg-[var(--projects-control)]"
                >
                  <td className={RESOURCE_CELL}>
                    <ResourceIdentity
                      name={bucket.name}
                      meta={`Created ${formatDate(bucket.created_at)}`}
                    />
                  </td>
                  <td className={RESOURCE_CELL}>
                    <span className="block truncate text-[13px] text-[var(--projects-text)]">
                      {`${formatBytes(bucket.used_bytes)} of ${formatBytes(
                        bucket.quota_bytes,
                      )}`}
                    </span>
                  </td>
                  <td className={`${RESOURCE_CELL} hidden sm:table-cell`}>
                    <span className="block truncate" title={bucket.id}>
                      <Mono className="text-[12px] text-[var(--projects-muted)]">
                        {bucket.id}
                      </Mono>
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <ResourceListFooter
            shown={items.length}
            noun="bucket"
            hasMore={Boolean(buckets.hasNextPage)}
            isLoadingMore={buckets.isFetchingNextPage}
            onLoadMore={() => void buckets.fetchNextPage()}
          />
        </div>
      )}
    </div>
  );
}
