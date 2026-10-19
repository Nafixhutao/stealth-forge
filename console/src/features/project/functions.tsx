"use client";

import { useState } from "react";
import { Plus, Trash2, Zap } from "lucide-react";
import type { components } from "@/api/generated/schema";
import { useDeleteFunction } from "@/api/mutations/functions";
import { useProjectFunctions, useProjectUsage } from "@/api/queries/projects";
import { EmptyState } from "@/components/feedback/empty-state";
import { ErrorState } from "@/components/feedback/error-state";
import { Button } from "@/components/ui/button";
import { CreateFunctionDialog } from "@/features/project/create-function-dialog";
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
  ResourceStatusChip,
} from "@/features/project/resource-list";
import { PRIMARY_ACTION } from "@/features/user/action-styles";
import { formatBytes, formatDate, pluralize } from "@/lib/format";

type Fn = components["schemas"]["Function"];

/** The API reports status as a lowercase value; the chip shows it as written. */
function capitalize(value: string) {
  return value.charAt(0).toUpperCase() + value.slice(1);
}

/** `active` is the only state that reads as healthy; everything else is quiet. */
function statusTone(status: string): "positive" | "neutral" {
  return status === "active" ? "positive" : "neutral";
}

/**
 * The Functions section: every function in the project, with the invocation
 * counter the usage snapshot reports for the project as a whole.
 *
 * A function is invoked on demand, so its activity lives in the rolling
 * 30-day counters rather than on the row: the figure above the list is the
 * project's, and it says so. Create writes metadata only — the function has no
 * code until a deployment is uploaded, and the console has no function upload
 * yet — so a freshly created row is called out as undeployed instead of
 * looking live.
 *
 * Create and Delete follow `can_manage` from the list itself.
 */
export function ProjectFunctions({ projectId }: { projectId: string }) {
  const functions = useProjectFunctions(projectId);
  const usage = useProjectUsage(projectId);
  const remove = useDeleteFunction(projectId);
  const [createOpen, setCreateOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<Fn | null>(null);

  const items =
    functions.data?.pages.flatMap((page) => page?.functions ?? []) ?? [];
  const canManage = functions.data?.pages[0]?.can_manage ?? false;
  const snapshot = usage.data?.usage;
  const description = snapshot
    ? `${pluralize(snapshot.function_count, "function")} · ${pluralize(
        snapshot.function_invocation_count_30d,
        "invocation",
      )} in 30 days`
    : "The project's functions.";

  const createButton = (
    <Button
      type="button"
      onClick={() => setCreateOpen(true)}
      className={PRIMARY_ACTION}
    >
      <Plus className="size-4 shrink-0" strokeWidth={2.2} aria-hidden="true" />
      Create function
    </Button>
  );

  return (
    <div className="flex flex-col gap-4">
      <ResourceSectionHeader
        title="Functions"
        description={description}
        action={canManage && items.length > 0 ? createButton : undefined}
      />

      {functions.isPending ? (
        <ResourceListSkeleton />
      ) : functions.isError ? (
        <ErrorState
          error={functions.error}
          retry={() => void functions.refetch()}
          title="Unable to load functions"
        />
      ) : items.length === 0 ? (
        <EmptyState
          className="mx-4 py-10 sm:mx-0 sm:py-14"
          icon={Zap}
          title="No functions in this project yet"
          description="A function runs on demand. Create one, then upload a deployment to give it code."
          action={canManage ? createButton : undefined}
        />
      ) : (
        <div className={RESOURCE_BOX}>
          <table className="w-full table-fixed border-collapse text-left">
            <thead>
              <tr className="border-b border-[var(--projects-divider)]">
                <th scope="col" className={RESOURCE_HEAD}>
                  Function
                </th>
                <th
                  scope="col"
                  className={`${RESOURCE_HEAD} w-[30%] sm:w-[20%]`}
                >
                  Status
                </th>
                <th
                  scope="col"
                  className={`${RESOURCE_HEAD} hidden sm:table-cell sm:w-[30%]`}
                >
                  Runtime / artifact
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
              {items.map((fn) => {
                // The description is optional; the created date stands in so
                // every row keeps a line of context.
                const meta =
                  fn.description?.trim() ||
                  `Created ${formatDate(fn.created_at)}`;

                return (
                  <tr
                    key={fn.id}
                    className="border-b border-[var(--projects-divider)] transition-colors hover:bg-[var(--projects-control)]"
                  >
                    <td className={RESOURCE_CELL}>
                      <ResourceIdentity name={fn.name} meta={meta} />
                    </td>
                    <td className={RESOURCE_CELL}>
                      <span className="flex min-w-0 flex-wrap items-center gap-1.5">
                        <ResourceStatusChip
                          tone={statusTone(fn.status)}
                          label={capitalize(fn.status)}
                        />
                        {/* Metadata exists but no release does: the function
                            has nothing to run yet. */}
                        {fn.active_deployment_id ? null : (
                          <ResourceStatusChip
                            tone="warning"
                            label="Not deployed"
                          />
                        )}
                      </span>
                    </td>
                    <td className={`${RESOURCE_CELL} hidden sm:table-cell`}>
                      <span className="flex min-w-0 flex-col">
                        <span
                          title={fn.runtime}
                          className="block truncate text-[12.5px] leading-5 text-[var(--projects-muted)]"
                        >
                          {fn.runtime}
                        </span>
                        <span className="block truncate text-[12px] leading-5 text-[var(--projects-muted)]">
                          {`${formatBytes(fn.artifact_used_bytes)} of ${formatBytes(
                            fn.artifact_quota_bytes,
                          )}`}
                        </span>
                      </span>
                    </td>
                    {canManage ? (
                      <td className={`${RESOURCE_CELL} px-2 text-right sm:px-5`}>
                        <ResourceRowAction
                          tone="danger"
                          icon={Trash2}
                          label={`Delete function ${fn.name}`}
                          onClick={() => setDeleteTarget(fn)}
                        />
                      </td>
                    ) : null}
                  </tr>
                );
              })}
            </tbody>
          </table>
          <ResourceListFooter
            shown={items.length}
            noun="function"
            hasMore={Boolean(functions.hasNextPage)}
            isLoadingMore={functions.isFetchingNextPage}
            onLoadMore={() => void functions.fetchNextPage()}
          />
        </div>
      )}

      {canManage ? (
        <>
          <CreateFunctionDialog
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
              resourceLabel="function"
              name={deleteTarget.name}
              consequence={`This deletes ${deleteTarget.name} and its deployments. It cannot be undone.`}
              onConfirm={() => remove.mutateAsync(deleteTarget.id)}
            />
          ) : null}
        </>
      ) : null}
    </div>
  );
}
