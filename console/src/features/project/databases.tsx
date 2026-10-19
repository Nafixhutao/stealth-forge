"use client";

import { Database } from "lucide-react";
import { useProjectDatabases, useProjectUsage } from "@/api/queries/projects";
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
import { formatDate, pluralize } from "@/lib/format";

/**
 * The Databases section: every database in the project, with the table and row
 * counts the usage snapshot reports for the project as a whole.
 *
 * Tables are a database's contents, and the API exposes them one database at a
 * time, so this list stays a list: the counts above it are the project's, and
 * they are labelled as such rather than attributed to any one row.
 */
export function ProjectDatabases({ projectId }: { projectId: string }) {
  const databases = useProjectDatabases(projectId);
  const usage = useProjectUsage(projectId);

  const items = databases.data?.pages.flatMap((page) => page?.databases ?? []) ?? [];
  const snapshot = usage.data?.usage;
  const description = snapshot
    ? `${pluralize(snapshot.database_count, "database")} · ${pluralize(
        snapshot.database_table_count,
        "table",
      )} · ${pluralize(snapshot.database_row_count, "row")}`
    : "The project's databases and their tables.";

  return (
    <div className="flex flex-col gap-4">
      <ResourceSectionHeader title="Databases" description={description} />

      {databases.isPending ? (
        <ResourceListSkeleton />
      ) : databases.isError ? (
        <ErrorState
          error={databases.error}
          retry={() => void databases.refetch()}
          title="Unable to load databases"
        />
      ) : items.length === 0 ? (
        <EmptyState
          className="mx-4 py-10 sm:mx-0 sm:py-14"
          icon={Database}
          title="No databases in this project yet"
          description="A database holds tables, and a table holds rows. Create one from the API to see it listed here."
        />
      ) : (
        <div className={RESOURCE_BOX}>
          <table className="w-full table-fixed border-collapse text-left">
            <thead>
              <tr className="border-b border-[var(--projects-divider)]">
                <th scope="col" className={RESOURCE_HEAD}>
                  Database
                </th>
                <th scope="col" className={`${RESOURCE_HEAD} w-[34%]`}>
                  Database ID
                </th>
              </tr>
            </thead>
            <tbody>
              {items.map((database) => (
                <tr
                  key={database.id}
                  className="border-b border-[var(--projects-divider)] transition-colors hover:bg-[var(--projects-control)]"
                >
                  <td className={RESOURCE_CELL}>
                    <ResourceIdentity
                      name={database.name}
                      meta={`Created ${formatDate(database.created_at)}`}
                    />
                  </td>
                  <td className={RESOURCE_CELL}>
                    <span className="block truncate" title={database.id}>
                      <Mono className="text-[12px] text-[var(--projects-muted)]">
                        {database.id}
                      </Mono>
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <ResourceListFooter
            shown={items.length}
            noun="database"
            hasMore={Boolean(databases.hasNextPage)}
            isLoadingMore={databases.isFetchingNextPage}
            onLoadMore={() => void databases.fetchNextPage()}
          />
        </div>
      )}
    </div>
  );
}
