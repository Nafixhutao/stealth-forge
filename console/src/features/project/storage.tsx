"use client";

import { useState } from "react";
import { HardDrive, Plus, Trash2 } from "lucide-react";
import type { components } from "@/api/generated/schema";
import { useDeleteBucket } from "@/api/mutations/storage";
import { useProjectBuckets, useProjectUsage } from "@/api/queries/projects";
import { EmptyState } from "@/components/feedback/empty-state";
import { ErrorState } from "@/components/feedback/error-state";
import { Mono } from "@/components/panel";
import { Button } from "@/components/ui/button";
import { CreateBucketDialog } from "@/features/project/create-bucket-dialog";
import { DeleteResourceDialog } from "@/features/project/delete-resource-dialog";
import {
  RESOURCE_BOX,
  RESOURCE_CELL,
  RESOURCE_HEAD,
  ResourceIdentity,
  ResourceListFooter,
  ResourceListSkeleton,
  ResourceRowAction,
  ResourceSectionHeader,
} from "@/features/project/resource-list";
import { PRIMARY_ACTION } from "@/features/user/action-styles";
import { formatBytes, formatDate, pluralize } from "@/lib/format";

type Bucket = components["schemas"]["StorageBucket"];

/**
 * The Storage section: every bucket in the project, with the file count and
 * bytes the usage snapshot reports for the project as a whole.
 *
 * A bucket's files are listed one bucket at a time, so this stays a list: the
 * totals above it are the project's, and they are labelled as such rather than
 * attributed to any one row.
 *
 * Create and Delete follow `can_manage` from the list itself. Deleting a bucket
 * takes its files with it, so the confirmation asks for the bucket's name.
 */
export function ProjectStorage({ projectId }: { projectId: string }) {
  const buckets = useProjectBuckets(projectId);
  const usage = useProjectUsage(projectId);
  const remove = useDeleteBucket(projectId);
  const [createOpen, setCreateOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<Bucket | null>(null);

  const items =
    buckets.data?.pages.flatMap((page) => page?.buckets ?? []) ?? [];
  const canManage = buckets.data?.pages[0]?.can_manage ?? false;
  const snapshot = usage.data?.usage;
  const description = snapshot
    ? `${pluralize(snapshot.storage_file_count, "file")} · ${formatBytes(
        snapshot.storage_bytes,
      )} of ${formatBytes(snapshot.storage_quota_bytes)}`
    : "The project's storage buckets.";

  const createButton = (
    <Button
      type="button"
      onClick={() => setCreateOpen(true)}
      className={PRIMARY_ACTION}
    >
      <Plus className="size-4 shrink-0" strokeWidth={2.2} aria-hidden="true" />
      Create bucket
    </Button>
  );

  return (
    <div className="flex flex-col gap-4">
      <ResourceSectionHeader
        title="Storage"
        description={description}
        action={canManage && items.length > 0 ? createButton : undefined}
      />

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
          description="A bucket holds the project's files. Create one, then upload files to it."
          action={canManage ? createButton : undefined}
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
                  className={`${RESOURCE_HEAD} w-[47%] sm:w-[32%]`}
                >
                  Used
                </th>
                <th
                  scope="col"
                  className={`${RESOURCE_HEAD} hidden sm:table-cell sm:w-[30%]`}
                >
                  Bucket ID
                </th>
                {canManage ? (
                  <th
                    scope="col"
                    className={`${RESOURCE_HEAD} w-[44px] px-2 sm:w-[72px] sm:px-5`}
                  >
                    <span className="sr-only">Actions</span>
                  </th>
                ) : null}
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
                    {/* A figure, not prose: it may shrink, but truncating
                        "32 MB of 100 MB" to "32 MB of 100 …" loses the quota
                        the column exists to show. */}
                    <span className="block whitespace-nowrap text-[13px] text-[var(--projects-text)]">
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
                  {canManage ? (
                    <td className={`${RESOURCE_CELL} px-2 text-right sm:px-5`}>
                      <ResourceRowAction
                        tone="danger"
                        icon={Trash2}
                        label={`Delete bucket ${bucket.name}`}
                        onClick={() => setDeleteTarget(bucket)}
                      />
                    </td>
                  ) : null}
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

      {canManage ? (
        <>
          <CreateBucketDialog
            projectId={projectId}
            open={createOpen}
            onOpenChange={setCreateOpen}
          />
          {deleteTarget ? (
            <DeleteResourceDialog
              open
              onOpenChange={(next) => {
                if (!next) setDeleteTarget(null);
              }}
              resourceLabel="bucket"
              name={deleteTarget.name}
              consequence={`This deletes ${deleteTarget.name} and every file stored in it. It cannot be undone.`}
              onConfirm={() => remove.mutateAsync(deleteTarget.id)}
            />
          ) : null}
        </>
      ) : null}
    </div>
  );
}
