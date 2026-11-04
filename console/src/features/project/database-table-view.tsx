"use client";

import { useState } from "react";
import type { components } from "@/api/generated/schema";
import { useDatabaseColumns, useDatabaseRows } from "@/api/queries/databases";
import { EmptyState } from "@/components/feedback/empty-state";
import { ErrorState } from "@/components/feedback/error-state";
import { Panel, PanelHeader } from "@/components/panel";
import { Button } from "@/components/ui/button";
import {
  ResourceListFooter,
  ResourceListSkeleton,
} from "@/features/project/resource-list";
import { CreateRowDialog } from "@/features/project/create-row-dialog";
import { PRIMARY_ACTION } from "@/features/user/action-styles";
import { Plus, Rows3 } from "lucide-react";

type Table = components["schemas"]["DatabaseTable"];
type Column = components["schemas"]["DatabaseColumn"];
type Row = components["schemas"]["DatabaseRow"];

const HEAD =
  "whitespace-nowrap px-4 py-2.5 text-left text-[12px] font-normal text-[var(--projects-muted)] sm:px-5 sm:py-3";
const CELL =
  "max-w-[32ch] truncate px-4 py-2.5 align-middle text-[13px] text-[var(--projects-text)] sm:px-5 sm:py-3";

function formatCell(value: unknown): string {
  if (value === null || value === undefined) return "—";
  if (typeof value === "string") return value;
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

function isMuted(value: unknown) {
  return value === null || value === undefined;
}

/**
 * One table's schema and rows.
 *
 * The grid is deliberately read-only for now: the API already exposes create,
 * update, delete, import, and export, and this surface starts with the path an
 * operator uses most, which is looking at what an application wrote. Adding a
 * row opens a form generated from the declared columns, so the control never
 * offers a field the server would reject.
 */
export function DatabaseTableView({
  projectId,
  databaseId,
  table,
  canManage,
}: {
  projectId: string;
  databaseId: string;
  table: Table;
  canManage: boolean;
}) {
  const columns = useDatabaseColumns(projectId, databaseId, table.id);
  const rows = useDatabaseRows(projectId, databaseId, table.id);
  const [rowOpen, setRowOpen] = useState(false);

  const columnItems =
    columns.data?.pages.flatMap((page) => page?.columns ?? []) ?? [];
  const rowItems = rows.data?.pages.flatMap((page) => page?.rows ?? []) ?? [];

  return (
    <div className="flex min-w-0 flex-col gap-4">
      <div className="flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
        <div className="min-w-0">
          <h3 className="m-0 truncate text-[16px] font-semibold tracking-[-0.01em] text-[var(--projects-text)]">
            {table.name}
          </h3>
          <p className="m-0 mt-0.5 flex flex-wrap items-center gap-x-2 gap-y-1 text-[12.5px] leading-5 text-[var(--projects-muted)]">
            <span className="admin-mono">{table.id}</span>
            <span aria-hidden="true">·</span>
            <span>
              {table.row_security ? "Row security on" : "Row security off"}
            </span>
          </p>
        </div>
        {canManage ? (
          <Button
            type="button"
            className={PRIMARY_ACTION}
            onClick={() => setRowOpen(true)}
          >
            <Plus
              className="size-4 shrink-0"
              strokeWidth={2.2}
              aria-hidden="true"
            />
            Add row
          </Button>
        ) : null}
      </div>

      <Panel labelledBy="database-columns-heading">
        <PanelHeader
          id="database-columns-heading"
          title="Columns"
          subtitle="The typed schema each row is validated against."
        />
        {columns.isPending ? (
          <div className="flex flex-wrap gap-2">
            {Array.from({ length: 4 }).map((_, index) => (
              <span
                key={index}
                className="h-7 w-28 animate-pulse rounded-md bg-[var(--projects-control)] motion-reduce:animate-none"
              />
            ))}
          </div>
        ) : columns.isError ? (
          <ErrorState
            error={columns.error}
            title="Unable to load columns"
            retry={() => void columns.refetch()}
          />
        ) : columnItems.length === 0 ? (
          <p className="m-0 text-[13px] leading-5 text-[var(--projects-muted)]">
            No columns are declared yet. Add them through the API, then rows can
            carry typed values.
          </p>
        ) : (
          <ul className="m-0 flex list-none flex-wrap gap-2 p-0">
            {columnItems.map((column: Column) => (
              <li
                key={column.id}
                className="inline-flex min-h-8 items-center gap-2 rounded-md border border-[var(--projects-border)] bg-[var(--projects-card-bg)] px-2.5 text-[12.5px]"
              >
                <span className="font-mono text-[var(--projects-text)]">
                  {column.key}
                </span>
                <span className="text-[var(--projects-muted)]">
                  {column.type}
                  {column.varchar_size ? `(${column.varchar_size})` : ""}
                </span>
                {column.required ? (
                  <span
                    className="text-[var(--projects-danger)]"
                    title="Required"
                    aria-label="Required"
                  >
                    *
                  </span>
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </Panel>

      <Panel flush labelledBy="database-rows-heading">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2 border-b border-[var(--projects-divider)] px-4 py-3 sm:px-5">
          <h3
            id="database-rows-heading"
            className="m-0 text-[12px] font-semibold uppercase tracking-[0.08em] text-[var(--projects-muted)]"
          >
            Rows
          </h3>
          <p className="m-0 text-[12.5px] leading-5 text-[var(--projects-muted)]">
            Read newest by id. Use the API for filters, search, and export.
          </p>
        </div>

        {rows.isPending ? (
          <div className="p-4 sm:p-5">
            <ResourceListSkeleton rows={4} />
          </div>
        ) : rows.isError ? (
          <div className="p-4 sm:p-5">
            <ErrorState
              error={rows.error}
              title="Unable to load rows"
              retry={() => void rows.refetch()}
            />
          </div>
        ) : rowItems.length === 0 ? (
          <EmptyState
            className="px-4 py-10"
            icon={Rows3}
            title="No rows in this table yet"
            description="Rows written by the Console or by an application appear here."
            action={
              canManage ? (
                <Button
                  type="button"
                  className={PRIMARY_ACTION}
                  onClick={() => setRowOpen(true)}
                >
                  <Plus
                    className="size-4 shrink-0"
                    strokeWidth={2.2}
                    aria-hidden="true"
                  />
                  Add row
                </Button>
              ) : undefined
            }
          />
        ) : (
          <>
            <div className="overflow-x-auto">
              <table className="w-full min-w-[640px] table-fixed border-collapse text-left">
                <thead>
                  <tr className="border-b border-[var(--projects-divider)]">
                    <th scope="col" className={`${HEAD} w-[38%]`}>
                      id
                    </th>
                    {columnItems.map((column) => (
                      <th key={column.id} scope="col" className={HEAD}>
                        {column.key}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {rowItems.map((row: Row) => (
                    <tr
                      key={row.id}
                      className="border-b border-[var(--projects-divider)] transition-colors last:border-b-0 hover:bg-[var(--projects-control)]"
                    >
                      <td className={CELL}>
                        <span
                          className="admin-mono text-[12px] text-[var(--projects-muted)]"
                          title={row.id}
                        >
                          {row.id}
                        </span>
                      </td>
                      {columnItems.map((column) => {
                        const value = row.data[column.key];
                        return (
                          <td key={column.id} className={CELL}>
                            <span
                              title={formatCell(value)}
                              className={
                                isMuted(value)
                                  ? "text-[var(--projects-muted)]"
                                  : undefined
                              }
                            >
                              {formatCell(value)}
                            </span>
                          </td>
                        );
                      })}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <ResourceListFooter
              shown={rowItems.length}
              noun="row"
              hasMore={Boolean(rows.hasNextPage)}
              isLoadingMore={rows.isFetchingNextPage}
              onLoadMore={() => void rows.fetchNextPage()}
            />
          </>
        )}
      </Panel>

      {canManage && rowOpen ? (
        <CreateRowDialog
          projectId={projectId}
          databaseId={databaseId}
          tableId={table.id}
          tableName={table.name}
          columns={columnItems}
          open
          onOpenChange={(next) => {
            if (!next) setRowOpen(false);
          }}
        />
      ) : null}
    </div>
  );
}
