"use client";

import { useState } from "react";
import { toast } from "sonner";
import { apiUrl } from "@/api/client";
import { nextCursor } from "@/api/pagination";
import {
  useCreateDatabaseBackup,
  useDeleteDatabaseBackup,
  useRestoreDatabaseBackup,
} from "@/api/mutations";
import { useDatabaseBackups } from "@/api/queries";
import type { DatabaseBackup } from "@/api/types";
import { DataTable, type DataTableColumnDef } from "@/components/data-table";
import { useCursorPagination } from "@/hooks/use-cursor-pagination";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { EmptyState } from "@/components/empty-state";
import { ErrorState, errorMessage } from "@/components/feedback/error-state";
import { ResourceId } from "@/components/resource-id";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { formatBytes, formatDate } from "@/lib/format";
import { pageControls } from "@/lib/pagination";

export function DatabaseBackupsPanel({
  projectId,
  databaseId,
}: {
  projectId: string;
  databaseId: string;
}) {
  const navigation = useCursorPagination("backups_cursor");
  const query = useDatabaseBackups(projectId, databaseId, {
    cursor: navigation.cursor,
  });
  const create = useCreateDatabaseBackup(projectId, databaseId);
  const remove = useDeleteDatabaseBackup(projectId, databaseId);
  const restore = useRestoreDatabaseBackup(projectId, databaseId);
  const [created, setCreated] = useState<DatabaseBackup>();
  const columns: DataTableColumnDef<DatabaseBackup>[] = [
    {
      accessorKey: "id",
      header: "Backup ID",
      cell: ({ row }) => <ResourceId id={row.original.id} label="Backup ID" />,
    },
    {
      accessorKey: "created_at",
      header: "Created",
      cell: ({ row }) => formatDate(row.original.created_at),
    },
    {
      accessorKey: "size_bytes",
      header: "Size",
      cell: ({ row }) => formatBytes(row.original.size_bytes),
    },
    {
      accessorKey: "checksum_sha256",
      header: "SHA-256",
      cell: ({ row }) => (
        <ResourceId id={row.original.checksum_sha256} label="Backup checksum" />
      ),
    },
    {
      id: "actions",
      header: "",
      cell: ({ row }) => (
        <div className="flex items-center justify-end gap-1">
          <Button asChild variant="ghost" size="sm">
            <a
              href={apiUrl(
                `/v1/projects/${projectId}/databases/${databaseId}/backups/${row.original.id}/download`,
              )}
            >
              Download
            </a>
          </Button>
          {query.data?.can_manage === true ? (
            <>
              <ConfirmDialog
                trigger={
                  <Button variant="ghost" size="sm">
                    Restore
                  </Button>
                }
                title="Restore this backup?"
                description={`Backup ${row.original.id} replaces all current tables, rows, columns, indexes, and relationships in this database. Any validation failure cancels the entire restore.`}
                confirmLabel="Restore backup"
                pending={restore.isPending}
                onConfirm={async () => {
                  await restore.mutateAsync(row.original.id);
                  toast.success("Database restored");
                }}
              />
              <ConfirmDialog
                trigger={
                  <Button variant="ghost" size="sm" className="text-rose-300">
                    Delete
                  </Button>
                }
                title="Delete this backup?"
                description={`Permanently delete backup ${row.original.id}. Current database data is not changed.`}
                confirmLabel="Delete backup"
                pending={remove.isPending}
                onConfirm={async () => {
                  await remove.mutateAsync(row.original.id);
                  if (created?.id === row.original.id) setCreated(undefined);
                  toast.success("Backup deleted");
                }}
              />
            </>
          ) : (
            <span className="text-xs text-slate-600">Read-only</span>
          )}
        </div>
      ),
    },
  ];
  const handleCreate = () =>
    create.mutate(undefined, {
      onSuccess: (result) => {
        setCreated(result?.backup);
        toast.success("Backup created");
      },
      onError: (error) => toast.error(errorMessage(error)),
    });
  return (
    <Card>
      <CardHeader className="flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <CardTitle>Backups</CardTitle>
          <p className="mt-1 text-xs text-slate-500">
            Complete logical snapshots, up to 10,000 rows and 50 MB. Larger
            databases cannot be backed up through this operation.
          </p>
        </div>
        {query.data?.can_manage === true ? (
          <Button disabled={create.isPending} onClick={handleCreate}>
            {create.isPending ? "Creating backup…" : "Create backup"}
          </Button>
        ) : query.data?.can_manage === false ? (
          <Badge variant="neutral">Read-only</Badge>
        ) : null}
      </CardHeader>
      <CardContent className="space-y-4">
        {create.error ? (
          <ErrorState
            title="Could not create backup"
            error={create.error}
            retry={handleCreate}
          />
        ) : null}
        {created ? (
          <div
            role="status"
            className="flex flex-wrap items-center gap-2 text-sm text-emerald-200"
          >
            Backup created <ResourceId id={created.id} label="Backup ID" />{" "}
            {formatBytes(created.size_bytes)}
          </div>
        ) : null}
        {query.error ? (
          <ErrorState
            title="Could not load database backups"
            error={query.error}
            retry={() => query.refetch()}
          />
        ) : null}
        {!query.isPending &&
        !query.error &&
        !query.data?.backups.length &&
        !nextCursor(query.data) &&
        !navigation.canFirst ? (
          <EmptyState
            title="No backups yet"
            description="Create a backup to capture the current database state."
          />
        ) : (
          <DataTable
            data={query.data?.backups ?? []}
            columns={columns}
            loading={query.isPending}
            empty="No backups on this page."
            serverPagination={pageControls(
              navigation,
              nextCursor(query.data),
              query.isFetching,
            )}
          />
        )}
      </CardContent>
    </Card>
  );
}
