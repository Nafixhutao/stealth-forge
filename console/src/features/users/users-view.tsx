"use client";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";
import { toast } from "sonner";
import { Search } from "lucide-react";
import { useCreateProjectUser } from "@/api/mutations";
import { nextCursor } from "@/api/pagination";
import { useProjectUsers } from "@/api/queries";
import type { ProjectUser } from "@/api/types";
import { CreateDialog } from "@/components/create-dialog";
import { DataTable, type DataTableColumnDef } from "@/components/data-table";
import { EmptyState } from "@/components/empty-state";
import { ErrorState } from "@/components/feedback/error-state";
import { PageHeader } from "@/components/page-header";
import { Badge, StatusBadge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { formatDate } from "@/lib/format";
import { useCursorPagination } from "@/hooks/use-cursor-pagination";
import { pageControls } from "@/lib/pagination";
import {
  type UserFormValues,
  userFields,
  userPayload,
} from "@/features/users/user-form";
import {
  RelativeDate,
  UserAvatar,
  UsersStatStrip,
} from "@/features/users/user-avatars";

export function UsersView({
  organizationId,
  projectId,
}: {
  organizationId: string;
  projectId: string;
}) {
  const router = useRouter();
  const navigation = useCursorPagination();
  const [createOpen, setCreateOpen] = useState(false);
  const [search, setSearch] = useState("");
  const query = useProjectUsers(projectId, { cursor: navigation.cursor });
  const create = useCreateProjectUser(projectId);
  const users = useMemo(() => query.data?.users ?? [], [query.data]);

  // Search filters the current page client-side; server pagination still owns
  // the data window, so this behaves like Vercel's in-page member filter.
  const filtered = useMemo(() => {
    const term = search.trim().toLowerCase();
    if (!term) return users;
    return users.filter(
      (user) =>
        user.email.toLowerCase().includes(term) ||
        (user.name ?? "").toLowerCase().includes(term),
    );
  }, [users, search]);

  const stats = useMemo(
    () => ({
      total: users.length,
      active: users.filter((user) => user.status === "active").length,
      blocked: users.filter((user) => user.status === "blocked").length,
      pending: users.filter((user) => !user.email_verified).length,
    }),
    [users],
  );

  const handleCreateUser = async (values: UserFormValues) => {
    const result = await create.mutateAsync(userPayload(values));
    toast.success("User created");
    if (result?.user.id)
      router.push(
        "/organizations/" +
          organizationId +
          "/projects/" +
          projectId +
          "/users/" +
          result.user.id,
      );
  };
  const canManage = query.data?.can_manage === true;
  const columns = useMemo<DataTableColumnDef<ProjectUser>[]>(
    () => [
      {
        accessorKey: "email",
        header: "User",
        cell: ({ row }) => (
          <div className="flex min-w-0 items-center gap-3">
            <UserAvatar email={row.original.email} name={row.original.name} />
            <div className="min-w-0">
              <Link
                href={
                  "/organizations/" +
                  organizationId +
                  "/projects/" +
                  projectId +
                  "/users/" +
                  row.original.id
                }
                className="wrap-anywhere block truncate font-medium text-white hover:text-cyan-200"
                title={row.original.email}
              >
                {row.original.email}
              </Link>
              <p className="truncate text-xs text-slate-500">
                {row.original.name ?? "Unnamed user"}
              </p>
            </div>
          </div>
        ),
      },
      {
        accessorKey: "status",
        header: "Status",
        cell: ({ row }) => <StatusBadge status={row.original.status} />,
      },
      {
        accessorKey: "email_verified",
        header: "Verification",
        cell: ({ row }) =>
          row.original.email_verified ? (
            <Badge variant="success">Verified</Badge>
          ) : (
            <Badge variant="warning">Pending</Badge>
          ),
      },
      {
        accessorKey: "created_at",
        header: "Joined",
        cell: ({ row }) => (
          <span className="text-slate-400">
            <RelativeDate iso={row.original.created_at} />
            <span className="block text-xs text-slate-600">
              {formatDate(row.original.created_at)}
            </span>
          </span>
        ),
      },
      {
        id: "actions",
        header: "",
        cell: ({ row }) => (
          <Button asChild variant="ghost" size="sm">
            <Link
              href={
                "/organizations/" +
                organizationId +
                "/projects/" +
                projectId +
                "/users/" +
                row.original.id
              }
            >
              Manage
            </Link>
          </Button>
        ),
      },
    ],
    [organizationId, projectId],
  );
  return (
    <>
      <PageHeader
        eyebrow="Auth"
        title="Users"
        description="Application users for this project. Console account sessions and project users are separate domains."
        actions={
          canManage ? (
            <CreateDialog<UserFormValues>
              open={createOpen}
              onOpenChange={setCreateOpen}
              triggerLabel="Create user"
              submitLabel="Create user"
              pendingLabel="Creating user…"
              title="Create an application user"
              description="The password is accepted by the API and is not stored in the console."
              fields={userFields}
              pending={create.isPending}
              onSubmit={handleCreateUser}
            />
          ) : null
        }
      />

      {/* Lifecycle counts from the loaded page — honest to the current window,
          like the Platform tiles on the admin Overview. */}
      {!query.isPending && !query.isError && <UsersStatStrip {...stats} />}

      {/* In-page filter, only rendered once there is something to filter. */}
      {!query.isPending && !query.isError && users.length > 0 ? (
        <div className="relative">
          <Search
            size={14}
            strokeWidth={1.8}
            aria-hidden="true"
            className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-slate-500"
          />
          <input
            type="search"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Search by email or name…"
            aria-label="Search users"
            className="h-9 w-full rounded-lg border border-stealth-border bg-[#141416] pl-9 pr-3 text-sm text-white placeholder:text-slate-600 focus:border-cyan-300/40 focus:outline-none focus:ring-1 focus:ring-cyan-300/40"
          />
        </div>
      ) : null}

      {query.isError ? (
        <ErrorState
          title="Could not load application users"
          error={query.error}
          retry={() => query.refetch()}
        />
      ) : query.isPending ? (
        <Card>
          <DataTable data={[]} columns={columns} loading />
        </Card>
      ) : filtered.length || navigation.canFirst || nextCursor(query.data) ? (
        <Card>
          <DataTable
            data={filtered}
            columns={columns}
            serverPagination={pageControls(
              navigation,
              nextCursor(query.data),
              query.isFetching,
            )}
          />
        </Card>
      ) : search ? (
        <EmptyState
          title="No users match this search"
          description={`Nothing on this page matches "${search}". Clear the filter to see all users.`}
        />
      ) : (
        <EmptyState
          title="No application users yet"
          description={
            canManage
              ? "Create an application identity to test authentication and user-scoped data."
              : "No application identities are visible in this project."
          }
          actionLabel={canManage ? "Create user" : undefined}
          action={canManage ? () => setCreateOpen(true) : undefined}
        />
      )}
    </>
  );
}
