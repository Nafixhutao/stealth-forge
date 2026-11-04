"use client";

import { useState } from "react";
import { toast } from "sonner";
import {
  Copy,
  Database,
  ExternalLink,
  Eye,
  EyeOff,
  Plus,
  Trash2,
} from "lucide-react";
import type { components } from "@/api/generated/schema";
import { ApiError } from "@/api/client";
import {
  useDeleteManagedDatabase,
  useRevealManagedDatabasePassword,
} from "@/api/mutations/managed-databases";
import { useManagedDatabases } from "@/api/queries/managed-databases";
import { EmptyState } from "@/components/feedback/empty-state";
import { ErrorState } from "@/components/feedback/error-state";
import { Mono, Panel, PanelHeader } from "@/components/panel";
import { Button } from "@/components/ui/button";
import { CreateManagedDatabaseDialog } from "@/features/project/create-managed-database-dialog";
import { DeleteResourceDialog } from "@/features/project/delete-resource-dialog";
import { ResourceSectionHeader } from "@/features/project/resource-list";
import { PRIMARY_ACTION } from "@/features/user/action-styles";
import { cn } from "@/lib/utils";

type ManagedDatabase = components["schemas"]["ManagedDatabase"];

function copyValue(label: string, value: string) {
  if (!navigator.clipboard) {
    toast.error("Clipboard is unavailable in this browser.");
    return;
  }
  void navigator.clipboard
    .writeText(value)
    .then(() => toast.success(`${label} copied`))
    .catch(() => toast.error(`Could not copy ${label.toLowerCase()}.`));
}

function CopyButton({ label, value }: { label: string; value: string }) {
  return (
    <button
      type="button"
      onClick={() => copyValue(label, value)}
      aria-label={`Copy ${label.toLowerCase()}`}
      title={`Copy ${label.toLowerCase()}`}
      className="inline-flex size-8 shrink-0 items-center justify-center rounded-md border border-[var(--projects-border)] text-[var(--projects-muted)] transition-colors hover:border-[var(--projects-border-hover)] hover:bg-[var(--projects-control)] hover:text-[var(--projects-text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--user-accent)]/40"
    >
      <Copy className="size-3.5" strokeWidth={2} aria-hidden="true" />
    </button>
  );
}

function ConnectionRow({
  label,
  value,
  muted = false,
  copyLabel,
}: {
  label: string;
  value: string;
  muted?: boolean;
  copyLabel?: string;
}) {
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 border-b border-[var(--projects-divider)] py-2.5 last:border-b-0">
      <dt className="w-24 shrink-0 text-[12.5px] text-[var(--projects-muted)]">
        {label}
      </dt>
      <dd className="m-0 flex min-w-0 flex-1 items-center gap-2">
        <Mono
          className={cn(
            "block min-w-0 truncate text-[13px]",
            muted
              ? "text-[var(--projects-muted)]"
              : "text-[var(--projects-text)]",
          )}
        >
          {value}
        </Mono>
        {copyLabel ? <CopyButton label={copyLabel} value={value} /> : null}
      </dd>
    </div>
  );
}

function DatabaseCard({
  projectId,
  database,
  onDelete,
}: {
  projectId: string;
  database: ManagedDatabase;
  onDelete: () => void;
}) {
  const reveal = useRevealManagedDatabasePassword(projectId);
  const [password, setPassword] = useState<string | null>(null);

  const togglePassword = async () => {
    if (password) {
      setPassword(null);
      return;
    }
    try {
      const result = await reveal.mutateAsync(database.id);
      setPassword(result?.password ?? null);
    } catch (cause) {
      toast.error(
        cause instanceof Error
          ? cause.message
          : "Could not reveal the password.",
      );
    }
  };

  return (
    <div className="flex flex-col gap-4">
      <Panel labelledBy="managed-database-connection">
        <PanelHeader
          id="managed-database-connection"
          title="Connection"
          subtitle="Use these values in your application or in phpMyAdmin."
          right={
            <span
              className={cn(
                "inline-flex shrink-0 items-center whitespace-nowrap rounded-full border px-2 py-0.5 text-[11.5px] font-medium leading-4",
                database.status === "ready"
                  ? "border-[var(--projects-accent-border)]/40 text-[var(--projects-accent)]"
                  : "border-[var(--projects-warning)]/40 text-[var(--projects-warning)]",
              )}
            >
              {database.status}
            </span>
          }
        />

        <dl className="m-0">
          <ConnectionRow label="Host" value={database.host} copyLabel="Host" />
          <ConnectionRow
            label="Port"
            value={String(database.port)}
            copyLabel="Port"
          />
          <ConnectionRow
            label="Database"
            value={database.database_name}
            copyLabel="Database name"
          />
          <ConnectionRow
            label="Username"
            value={database.username}
            copyLabel="Username"
          />
          <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 border-b border-[var(--projects-divider)] py-2.5 last:border-b-0">
            <dt className="w-24 shrink-0 text-[12.5px] text-[var(--projects-muted)]">
              Password
            </dt>
            <dd className="m-0 flex min-w-0 flex-1 items-center gap-2">
              <Mono className="block min-w-0 truncate text-[13px] text-[var(--projects-text)]">
                {password ?? "••••••••••••••••"}
              </Mono>
              {password ? (
                <CopyButton label="Password" value={password} />
              ) : null}
              <button
                type="button"
                onClick={() => void togglePassword()}
                disabled={reveal.isPending}
                aria-label={password ? "Hide password" : "Reveal password"}
                title={password ? "Hide password" : "Reveal password"}
                className="inline-flex size-8 shrink-0 items-center justify-center rounded-md border border-[var(--projects-border)] text-[var(--projects-muted)] transition-colors hover:border-[var(--projects-border-hover)] hover:bg-[var(--projects-control)] hover:text-[var(--projects-text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--user-accent)]/40 disabled:opacity-50"
              >
                {password ? (
                  <EyeOff
                    className="size-3.5"
                    strokeWidth={2}
                    aria-hidden="true"
                  />
                ) : (
                  <Eye
                    className="size-3.5"
                    strokeWidth={2}
                    aria-hidden="true"
                  />
                )}
              </button>
            </dd>
          </div>
        </dl>

        <div className="mt-4 flex flex-wrap items-center gap-2">
          <a
            href={database.phpmyadmin_url}
            target="_blank"
            rel="noopener noreferrer"
            className={cn(
              PRIMARY_ACTION,
              "inline-flex items-center justify-center no-underline",
            )}
          >
            Open phpMyAdmin
            <ExternalLink
              className="size-4"
              strokeWidth={2}
              aria-hidden="true"
            />
          </a>
          <p className="m-0 text-[12px] leading-4 text-[var(--projects-muted)]">
            Log in with the username and password above.
          </p>
        </div>
      </Panel>

      <Panel>
        <PanelHeader title="Free tier" />
        <ul className="m-0 flex list-none flex-col gap-1.5 p-0 text-[13px] leading-5 text-[var(--projects-muted)]">
          <li>One MySQL 8.4 database per project.</li>
          <li>256 MB fair-use storage, 10 concurrent connections.</li>
          <li>
            MySQL is not published to the internet; phpMyAdmin is the web door.
          </li>
          <li>No automatic backups in the free tier. Deleting is permanent.</li>
        </ul>
        <button
          type="button"
          onClick={onDelete}
          className="mt-4 inline-flex min-h-9 items-center gap-1.5 rounded-md border border-[var(--projects-border)] px-2.5 text-[12.5px] font-medium text-[var(--projects-danger)] transition-colors hover:border-[var(--projects-danger)]/50 hover:bg-[var(--projects-danger)]/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--projects-danger)]/40"
        >
          <Trash2 className="size-3.5" strokeWidth={2} aria-hidden="true" />
          Delete database
        </button>
      </Panel>
    </div>
  );
}

/**
 * The hosted MySQL section.
 *
 * The free tier is a real MySQL database and user on the shared tenant cluster,
 * with phpMyAdmin as the browser door. Passwords are revealed only on demand
 * and are not kept in the query cache.
 */
export function ProjectManagedDatabases({ projectId }: { projectId: string }) {
  const databases = useManagedDatabases(projectId);
  const remove = useDeleteManagedDatabase(projectId);
  const [createOpen, setCreateOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<ManagedDatabase | null>(
    null,
  );

  const items = databases.data?.databases ?? [];
  const disabled =
    databases.error instanceof ApiError &&
    databases.error.code === "feature_disabled";

  return (
    <div className="flex flex-col gap-4">
      <ResourceSectionHeader
        title="MySQL"
        description="A real MySQL database for this project, reachable through phpMyAdmin."
        action={
          items.length === 0 && !disabled ? (
            <Button
              type="button"
              onClick={() => setCreateOpen(true)}
              className={PRIMARY_ACTION}
            >
              <Plus
                className="size-4 shrink-0"
                strokeWidth={2.2}
                aria-hidden="true"
              />
              Create database
            </Button>
          ) : undefined
        }
      />

      {databases.isPending ? (
        <Panel>
          <div className="flex flex-col gap-2">
            {Array.from({ length: 4 }).map((_, index) => (
              <span
                key={index}
                className="h-4 w-full max-w-md animate-pulse rounded bg-[var(--projects-control)] motion-reduce:animate-none"
              />
            ))}
          </div>
        </Panel>
      ) : disabled ? (
        <EmptyState
          className="py-10"
          icon={Database}
          title="MySQL hosting is not enabled"
          description="This installation has not provisioned the tenant MySQL cluster yet."
        />
      ) : databases.isError ? (
        <ErrorState
          error={databases.error}
          title="Unable to load the database"
          retry={() => void databases.refetch()}
        />
      ) : items.length === 0 ? (
        <EmptyState
          className="py-10"
          icon={Database}
          title="No MySQL database yet"
          description="Create the project's free MySQL database, then open phpMyAdmin to manage it."
          action={
            <Button
              type="button"
              onClick={() => setCreateOpen(true)}
              className={PRIMARY_ACTION}
            >
              <Plus
                className="size-4 shrink-0"
                strokeWidth={2.2}
                aria-hidden="true"
              />
              Create database
            </Button>
          }
        />
      ) : (
        items.map((database) => (
          <DatabaseCard
            key={database.id}
            projectId={projectId}
            database={database}
            onDelete={() => setDeleteTarget(database)}
          />
        ))
      )}

      {!disabled ? (
        <CreateManagedDatabaseDialog
          projectId={projectId}
          open={createOpen}
          onOpenChange={setCreateOpen}
        />
      ) : null}

      {deleteTarget ? (
        <DeleteResourceDialog
          open
          onOpenChange={(next) => {
            if (!next) setDeleteTarget(null);
          }}
          resourceLabel="database"
          name={deleteTarget.name}
          consequence={`This drops ${deleteTarget.database_name} and its MySQL user. Every table and row inside it is deleted permanently.`}
          onConfirm={() => remove.mutateAsync(deleteTarget.id)}
        />
      ) : null}
    </div>
  );
}
