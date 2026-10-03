"use client";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { toast } from "sonner";
import { nextCursor } from "@/api/pagination";
import { useCreateDatabaseTable } from "@/api/mutations";
import {
  useDatabase,
  useDatabaseBackups,
  useDatabaseTables,
} from "@/api/queries";
import type { DatabaseTable } from "@/api/types";
import { CreateDialog } from "@/components/create-dialog";
import { DataTable, type DataTableColumnDef } from "@/components/data-table";
import { useCursorPagination } from "@/hooks/use-cursor-pagination";
import { EmptyState } from "@/components/empty-state";
import { ErrorState } from "@/components/feedback/error-state";
import { LoadingState } from "@/components/feedback/loading-state";
import { PageHeader } from "@/components/page-header";
import { ResourceId } from "@/components/resource-id";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { formatDate } from "@/lib/format";
import { pageControls } from "@/lib/pagination";
import { BackLink } from "@/components/back-link";
import {
  databaseTableFields,
  databaseTablePayload,
  type DatabaseTableFormValues,
} from "@/features/databases/database-table-form";
import { DatabaseBackupsPanel } from "./database-backups-panel";

export { DatabaseRowsView } from "./table-detail-view";

export function DatabaseDetailView({
  organizationId,
  projectId,
  databaseId,
}: {
  organizationId: string;
  projectId: string;
  databaseId: string;
}) {
  const router = useRouter();
  const query = useDatabase(projectId, databaseId);
  const tablesNavigation = useCursorPagination("tables_cursor");
  const tables = useDatabaseTables(projectId, databaseId, {
    cursor: tablesNavigation.cursor,
  });
  const backups = useDatabaseBackups(projectId, databaseId);
  const [createTableOpen, setCreateTableOpen] = useState(false);
  const createTable = useCreateDatabaseTable(projectId, databaseId);
  const database = query.data?.database;
  const base = `/organizations/${organizationId}/projects/${projectId}`;
  const handleCreateTable = async (values: DatabaseTableFormValues) => {
    const result = await createTable.mutateAsync(databaseTablePayload(values));
    toast.success("Table created");
    if (result?.table.id)
      router.push(`${base}/databases/${databaseId}/tables/${result.table.id}`);
    else tablesNavigation.goFirst();
  };
  if (query.error && !database)
    return (
      <ErrorState
        title="Could not load database"
        error={query.error}
        retry={() => query.refetch()}
      />
    );
  if (query.isPending) return <LoadingState />;
  if (!database)
    return (
      <EmptyState
        title="Database not found"
        description="The database may have been removed or is outside this project."
      />
    );
  const columns: DataTableColumnDef<DatabaseTable>[] = [
    {
      accessorKey: "name",
      header: "Table",
      cell: ({ row }) => (
        <Link
          href={`${base}/databases/${databaseId}/tables/${row.original.id}`}
          className="font-medium text-white hover:text-amber-200"
        >
          {row.original.name}
        </Link>
      ),
    },
    {
      accessorKey: "row_security",
      header: "Row security",
      cell: ({ row }) => (
        <Badge variant="neutral">
          {row.original.row_security ? "Enabled" : "Disabled"}
        </Badge>
      ),
    },
    {
      accessorKey: "created_at",
      header: "Created",
      cell: ({ row }) => formatDate(row.original.created_at),
    },
    {
      accessorKey: "updated_at",
      header: "Updated",
      cell: ({ row }) => formatDate(row.original.updated_at),
    },
    {
      id: "open",
      header: "",
      cell: ({ row }) => (
        <Button asChild size="sm" variant="ghost">
          <Link
            href={`${base}/databases/${databaseId}/tables/${row.original.id}`}
          >
            Open table
          </Link>
        </Button>
      ),
    },
  ];
  const tableData = tables.data?.tables ?? [];
  const tableCount =
    tables.data &&
    !tables.isPlaceholderData &&
    !tablesNavigation.cursor &&
    !nextCursor(tables.data)
      ? tableData.length
      : undefined;
  const backupItems = backups.data?.backups ?? [];
  const latestBackup =
    backups.data && !nextCursor(backups.data) && backupItems.length
      ? backupItems.reduce((latest, candidate) =>
          Date.parse(candidate.created_at) > Date.parse(latest.created_at)
            ? candidate
            : latest,
        )
      : undefined;
  return (
    <>
      <BackLink href={`${base}/databases`} label="Back to databases" />
      <PageHeader
        eyebrow="Database"
        title={database.name}
        description="Manage tables, inspect application data, and capture logical backups."
        actions={
          tables.data?.can_manage === true ? (
            <CreateDialog<DatabaseTableFormValues>
              open={createTableOpen}
              onOpenChange={setCreateTableOpen}
              triggerLabel="Create table"
              submitLabel="Create table"
              pendingLabel="Creating table…"
              title="Create a table"
              description="Define a table before adding columns and rows. Application permissions start denied."
              fields={databaseTableFields}
              onSubmit={handleCreateTable}
              pending={createTable.isPending}
            />
          ) : tables.data ? (
            <Badge variant="neutral">Read-only</Badge>
          ) : null
        }
      />
      <div className="mb-5 flex flex-wrap items-center gap-3 text-xs text-slate-500">
        <ResourceId id={database.id} label="Database ID" />
        <span>Created {formatDate(database.created_at)}</span>
        <span>Updated {formatDate(database.updated_at)}</span>
      </div>
      {query.error ? (
        <ErrorState
          title="Could not refresh database"
          error={query.error}
          retry={() => query.refetch()}
        />
      ) : null}
      <Tabs defaultValue="overview">
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="tables">Tables</TabsTrigger>
          <TabsTrigger value="backups">Backups</TabsTrigger>
          <TabsTrigger value="settings">Settings</TabsTrigger>
        </TabsList>
        <TabsContent value="overview">
          <Card>
            <CardHeader>
              <CardTitle>Database overview</CardTitle>
            </CardHeader>
            <CardContent>
              <dl className="grid gap-5 sm:grid-cols-2">
                <div>
                  <dt className="text-xs text-slate-500">Name</dt>
                  <dd className="mt-1">{database.name}</dd>
                </div>
                {tableCount !== undefined ? (
                  <div>
                    <dt className="text-xs text-slate-500">Tables</dt>
                    <dd className="mt-1">{tableCount}</dd>
                  </div>
                ) : null}
                {latestBackup ? (
                  <div>
                    <dt className="text-xs text-slate-500">Latest backup</dt>
                    <dd className="mt-1">
                      <ResourceId id={latestBackup.id} label="Backup ID" />{" "}
                      {formatDate(latestBackup.created_at)}
                    </dd>
                  </div>
                ) : null}
              </dl>
              {!tables.isPending &&
              !tables.error &&
              tableData.length === 0 &&
              !nextCursor(tables.data) &&
              !tablesNavigation.canFirst ? (
                <div className="mt-5">
                  <EmptyState
                    title="No tables yet"
                    description="Create a table to start storing structured application data."
                    actionLabel={
                      tables.data?.can_manage === true
                        ? "Create table"
                        : undefined
                    }
                    action={
                      tables.data?.can_manage === true
                        ? () => setCreateTableOpen(true)
                        : undefined
                    }
                  />
                </div>
              ) : null}
              {tables.error ? (
                <ErrorState
                  title="Could not load database tables"
                  error={tables.error}
                  retry={() => tables.refetch()}
                />
              ) : null}
              {backups.error ? (
                <ErrorState
                  title="Could not load database backups"
                  error={backups.error}
                  retry={() => backups.refetch()}
                />
              ) : null}
            </CardContent>
          </Card>
        </TabsContent>
        <TabsContent value="tables">
          <Card>
            <CardHeader>
              <CardTitle>Tables</CardTitle>
            </CardHeader>
            <CardContent>
              {tables.error ? (
                <ErrorState
                  title="Could not load database tables"
                  error={tables.error}
                  retry={() => tables.refetch()}
                />
              ) : null}
              {!tables.isPending &&
              !tables.error &&
              tableData.length === 0 &&
              !nextCursor(tables.data) &&
              !tablesNavigation.canFirst ? (
                <EmptyState
                  title="No tables yet"
                  description="Create a table to start storing structured application data."
                  actionLabel={
                    tables.data?.can_manage === true
                      ? "Create table"
                      : undefined
                  }
                  action={
                    tables.data?.can_manage === true
                      ? () => setCreateTableOpen(true)
                      : undefined
                  }
                />
              ) : (
                <DataTable
                  data={tableData}
                  columns={columns}
                  loading={tables.isPending}
                  empty="No tables on this page."
                  serverPagination={pageControls(
                    tablesNavigation,
                    nextCursor(tables.data),
                    tables.isFetching,
                  )}
                />
              )}
            </CardContent>
          </Card>
        </TabsContent>
        <TabsContent value="backups">
          <DatabaseBackupsPanel projectId={projectId} databaseId={databaseId} />
        </TabsContent>
        <TabsContent value="settings">
          <Card>
            <CardHeader>
              <CardTitle>Database settings</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              <p className="text-sm text-slate-400">Name: {database.name}</p>
              <ResourceId id={database.id} label="Database ID" />
              <p className="text-sm text-slate-500">
                Database names are read-only here. Configure columns and row
                access in each table.
              </p>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </>
  );
}
