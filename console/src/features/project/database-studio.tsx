"use client";

import { useState } from "react";
import Link from "next/link";
import {
  ChevronLeft,
  Database as DatabaseIcon,
  Plus,
  Table2,
} from "lucide-react";
import { useDatabase, useDatabaseTables } from "@/api/queries/databases";
import { EmptyState } from "@/components/feedback/empty-state";
import { ErrorState } from "@/components/feedback/error-state";
import { Panel } from "@/components/panel";
import {
  ResourceListFooter,
  ResourceSectionHeader,
} from "@/features/project/resource-list";
import { CreateTableDialog } from "@/features/project/create-table-dialog";
import { DatabaseTableView } from "@/features/project/database-table-view";
import { projectPath } from "@/features/project/paths";
import { cn } from "@/lib/utils";

/**
 * The database browser behind one database's Console URL.
 *
 * It follows the same rule as the rest of the project surface: management
 * metadata (tables, columns) is read through the Console session, while rows
 * are read from the same endpoint an application uses. The left rail picks a
 * table and the panel beside it renders that table's schema and rows, which is
 * the shape an operator expects from a database admin surface without exposing
 * raw SQL or another tenant's data.
 */
export function DatabaseStudio({
  projectId,
  databaseId,
}: {
  projectId: string;
  databaseId: string;
}) {
  const database = useDatabase(projectId, databaseId);
  const tables = useDatabaseTables(projectId, databaseId);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [createOpen, setCreateOpen] = useState(false);

  const items = tables.data?.pages.flatMap((page) => page?.tables ?? []) ?? [];
  const canManage = tables.data?.pages[0]?.can_manage ?? false;
  const selected = items.find((table) => table.id === selectedId) ?? items[0];

  const heading = database.data?.database.name ?? "Database";

  return (
    <div className="flex flex-col gap-4">
      <Link
        href={projectPath(projectId, "databases")}
        className="inline-flex w-fit items-center gap-0.5 rounded text-[12.5px] leading-5 text-[var(--projects-muted)] transition-colors hover:text-[var(--projects-text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--user-accent)]/40"
      >
        <ChevronLeft size={14} strokeWidth={2} aria-hidden="true" />
        Databases
      </Link>

      <ResourceSectionHeader
        title={heading}
        description="Browse the tables, columns, and rows this database holds."
      />

      {database.isError ? (
        <ErrorState
          error={database.error}
          title="Unable to load this database"
          retry={() => void database.refetch()}
        />
      ) : (
        <div className="grid gap-4 lg:grid-cols-[240px_minmax(0,1fr)] lg:items-start">
          <Panel flush className="lg:sticky lg:top-4">
            <div className="flex items-center gap-2 border-b border-[var(--projects-divider)] px-4 py-3">
              <h2 className="m-0 text-[12px] font-semibold uppercase tracking-[0.08em] text-[var(--projects-muted)]">
                Tables
              </h2>
              {canManage ? (
                <button
                  type="button"
                  onClick={() => setCreateOpen(true)}
                  aria-label="Create table"
                  title="Create table"
                  className="ml-auto inline-flex size-8 items-center justify-center rounded-md border border-[var(--projects-border)] text-[var(--projects-text)] transition-colors hover:border-[var(--projects-border-hover)] hover:bg-[var(--projects-control)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--user-accent)]/40"
                >
                  <Plus size={15} strokeWidth={2.2} aria-hidden="true" />
                </button>
              ) : null}
            </div>

            {tables.isPending ? (
              <div className="flex flex-col gap-1 p-2">
                {Array.from({ length: 4 }).map((_, index) => (
                  <span
                    key={index}
                    className="h-9 animate-pulse rounded-md bg-[var(--projects-control)] motion-reduce:animate-none"
                  />
                ))}
              </div>
            ) : tables.isError ? (
              <div className="p-4">
                <ErrorState
                  error={tables.error}
                  title="Unable to load tables"
                  retry={() => void tables.refetch()}
                />
              </div>
            ) : items.length === 0 ? (
              <EmptyState
                className="px-4 py-8"
                icon={Table2}
                title="No tables yet"
                description="Create a table, then add its typed columns through the API."
                action={
                  canManage ? (
                    <button
                      type="button"
                      onClick={() => setCreateOpen(true)}
                      className="inline-flex min-h-9 items-center gap-1.5 rounded-md border border-[var(--projects-border)] px-2.5 text-[12.5px] font-medium text-[var(--projects-text)] transition-colors hover:border-[var(--projects-border-hover)] hover:bg-[var(--projects-control)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--user-accent)]/40"
                    >
                      <Plus size={14} strokeWidth={2.2} aria-hidden="true" />
                      New table
                    </button>
                  ) : undefined
                }
              />
            ) : (
              <>
                <ul className="m-0 flex list-none flex-col gap-0.5 p-2">
                  {items.map((table) => {
                    const active = table.id === selected?.id;
                    return (
                      <li key={table.id}>
                        <button
                          type="button"
                          onClick={() => setSelectedId(table.id)}
                          aria-current={active ? "true" : undefined}
                          className={cn(
                            "flex w-full flex-col items-start gap-0.5 rounded-md px-2.5 py-2 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--user-accent)]/40",
                            active
                              ? "bg-[var(--projects-control)]"
                              : "hover:bg-[var(--projects-control)]",
                          )}
                        >
                          <span
                            className={cn(
                              "block max-w-full truncate text-[13.5px] leading-5",
                              active
                                ? "font-semibold text-[var(--projects-text)]"
                                : "font-medium text-[var(--projects-text)]",
                            )}
                          >
                            {table.name}
                          </span>
                          <span className="text-[11.5px] leading-4 text-[var(--projects-muted)]">
                            {table.row_security
                              ? "Row security on"
                              : "Row security off"}
                          </span>
                        </button>
                      </li>
                    );
                  })}
                </ul>
                <ResourceListFooter
                  shown={items.length}
                  noun="table"
                  hasMore={Boolean(tables.hasNextPage)}
                  isLoadingMore={tables.isFetchingNextPage}
                  onLoadMore={() => void tables.fetchNextPage()}
                />
              </>
            )}
          </Panel>

          {selected ? (
            <DatabaseTableView
              projectId={projectId}
              databaseId={databaseId}
              table={selected}
              canManage={canManage}
            />
          ) : (
            <Panel>
              <EmptyState
                icon={DatabaseIcon}
                title="Select a table"
                description="A table's columns and rows appear here once one exists."
                className="py-8"
              />
            </Panel>
          )}
        </div>
      )}

      {canManage ? (
        <CreateTableDialog
          projectId={projectId}
          databaseId={databaseId}
          open={createOpen}
          onOpenChange={setCreateOpen}
        />
      ) : null}
    </div>
  );
}
