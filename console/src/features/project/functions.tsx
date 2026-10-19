"use client";

import { Zap } from "lucide-react";
import { useProjectFunctions, useProjectUsage } from "@/api/queries/projects";
import { EmptyState } from "@/components/feedback/empty-state";
import { ErrorState } from "@/components/feedback/error-state";
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
import { formatBytes, formatDate, pluralize } from "@/lib/format";

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
 * project's, and it says so. The list itself is read-only because the console
 * has no mutation for a function yet, and a disabled row is the API's own
 * status rather than a second reading of `enabled`.
 */
export function ProjectFunctions({ projectId }: { projectId: string }) {
  const functions = useProjectFunctions(projectId);
  const usage = useProjectUsage(projectId);

  const items =
    functions.data?.pages.flatMap((page) => page?.functions ?? []) ?? [];
  const snapshot = usage.data?.usage;
  const description = snapshot
    ? `${pluralize(snapshot.function_count, "function")} · ${pluralize(
        snapshot.function_invocation_count_30d,
        "invocation",
      )} in 30 days`
    : "The project's functions.";

  return (
    <div className="flex flex-col gap-4">
      <ResourceSectionHeader title="Functions" description={description} />

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
          description="A function runs on demand. Create one from the API to see it listed here."
        />
      ) : (
        <div className={RESOURCE_BOX}>
          <table className="w-full table-fixed border-collapse text-left">
            <thead>
              <tr className="border-b border-[var(--projects-divider)]">
                <th
                  scope="col"
                  className={`${RESOURCE_HEAD} w-[70%] sm:w-auto`}
                >
                  Function
                </th>
                <th
                  scope="col"
                  className={`${RESOURCE_HEAD} w-[30%] sm:w-[22%]`}
                >
                  Status
                </th>
                <th
                  scope="col"
                  className={`${RESOURCE_HEAD} hidden sm:table-cell sm:w-[32%]`}
                >
                  Runtime / artifact
                </th>
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
                      <ResourceStatusChip
                        tone={statusTone(fn.status)}
                        label={capitalize(fn.status)}
                      />
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
    </div>
  );
}
