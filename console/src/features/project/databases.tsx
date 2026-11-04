"use client";

import { useState } from "react";
import Link from "next/link";
import { ArrowUpRight, Database, Plus, Trash2 } from "lucide-react";
import type { components } from "@/api/generated/schema";
import { useDeleteDatabase } from "@/api/mutations/databases";
import { useProjectDatabases, useProjectUsage } from "@/api/queries/projects";
import { EmptyState } from "@/components/feedback/empty-state";
import { ErrorState } from "@/components/feedback/error-state";
import { Mono } from "@/components/panel";
import { Button } from "@/components/ui/button";
import { CreateDatabaseDialog } from "@/features/project/create-database-dialog";
import { DeleteResourceDialog } from "@/features/project/delete-resource-dialog";
import { projectPath } from "@/features/project/paths";
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
import { formatDate, pluralize } from "@/lib/format";

type Database = components["schemas"]["ProjectDatabase"];

/**
 * The Databases section: every database in the project, with the table and row
 * counts the usage snapshot reports for the project as a whole.
 *
 * Tables are a database's contents, and the API exposes them one database at a
 * time, so this list stays a list: the counts above it are the project's, and
 * they are labelled as such rather than attributed to any one row.
 *
 * Create and Delete follow `can_manage` from the list itself, so a viewer sees
 * the list rather than controls that would answer 403.
 */
export function ProjectDatabases({ projectId }: { projectId: string }) {
  const databases = useProjectDatabases(projectId);
  const usage = useProjectUsage(projectId);
  const remove = useDeleteDatabase(projectId);
  const [createOpen, setCreateOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<Database | null>(null);

  const items =
    databases.data?.pages.flatMap((page) => page?.databases ?? []) ?? [];
  const canManage = databases.data?.pages[0]?.can_manage ?? false;
  const snapshot = usage.data?.usage;
  const description = snapshot
    ? `${pluralize(snapshot.database_count, "collection")} · ${pluralize(
        snapshot.database_table_count,
        "table",
      )} · ${pluralize(snapshot.database_row_count, "row")}`
    : "The project's built-in JSON data collections.";

  const createButton = (
    <Button
      type="button"
      onClick={() => setCreateOpen(true)}
      className={PRIMARY_ACTION}
    >
      <Plus className="size-4 shrink-0" strokeWidth={2.2} aria-hidden="true" />
      Create database
    </Button>
  );

  return (
    <div className="flex flex-col gap-4">
      <ResourceSectionHeader
        title="Data"
        description={description}
        action={canManage && items.length > 0 ? createButton : undefined}
      />

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
          description="A database holds tables, and a table holds rows. Create one, then add its tables."
          action={canManage ? createButton : undefined}
        />
      ) : (
        <div className={RESOURCE_BOX}>
          <table className="w-full table-fixed border-collapse text-left">
            <thead>
              <tr className="border-b border-[var(--projects-divider)]">
                <th scope="col" className={RESOURCE_HEAD}>
                  Database
                </th>
                <th
                  scope="col"
                  className={`${RESOURCE_HEAD} hidden w-[34%] sm:table-cell`}
                >
                  Database ID
                </th>
                <th
                  scope="col"
                  className={`${RESOURCE_HEAD} w-[44px] px-2 sm:w-[72px] sm:px-5`}
                >
                  <span className="sr-only">Actions</span>
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
                      href={projectPath(projectId, `databases/${database.id}`)}
                    />
                  </td>
                  <td className={`${RESOURCE_CELL} hidden sm:table-cell`}>
                    <span className="block truncate" title={database.id}>
                      <Mono className="text-[12px] text-[var(--projects-muted)]">
                        {database.id}
                      </Mono>
                    </span>
                  </td>
                  <td className={`${RESOURCE_CELL} px-2 sm:px-5`}>
                    <div className="flex items-center justify-end gap-1">
                      <Link
                        href={projectPath(
                          projectId,
                          `databases/${database.id}`,
                        )}
                        aria-label={`Open database ${database.name}`}
                        title={`Open database ${database.name}`}
                        className="inline-flex size-9 items-center justify-center rounded-md border border-[var(--projects-border)] text-[var(--projects-text)] transition-colors hover:border-[var(--projects-border-hover)] hover:bg-[var(--projects-control)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--user-accent)]/40"
                      >
                        <ArrowUpRight
                          className="size-3.5 shrink-0"
                          strokeWidth={2}
                          aria-hidden="true"
                        />
                      </Link>
                      {canManage ? (
                        <ResourceRowAction
                          tone="danger"
                          icon={Trash2}
                          label={`Delete database ${database.name}`}
                          onClick={() => setDeleteTarget(database)}
                        />
                      ) : null}
                    </div>
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

      {canManage ? (
        <>
          <CreateDatabaseDialog
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
              resourceLabel="database"
              name={deleteTarget.name}
              consequence={`This deletes ${deleteTarget.name} and every table, index, and row inside it. It cannot be undone.`}
              onConfirm={() => remove.mutateAsync(deleteTarget.id)}
            />
          ) : null}
        </>
      ) : null}
    </div>
  );
}
