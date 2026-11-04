"use client";

import { useState } from "react";
import { toast } from "sonner";
import {
  ClipboardCopy,
  Copy,
  Database,
  ExternalLink,
  Eye,
  EyeOff,
  KeyRound,
  Plus,
  RotateCcw,
  Trash2,
} from "lucide-react";
import type { components } from "@/api/generated/schema";
import { ApiError } from "@/api/client";
import {
  useDeleteManagedDatabase,
  useRevealManagedDatabasePassword,
  useRotateManagedDatabasePassword,
} from "@/api/mutations/managed-databases";
import {
  useManagedDatabaseBackups,
  useRestoreManagedDatabase,
} from "@/api/queries/managed-database-backups";
import { useManagedDatabases } from "@/api/queries/managed-databases";
import { EmptyState } from "@/components/feedback/empty-state";
import { ErrorState } from "@/components/feedback/error-state";
import { Mono, Panel, PanelHeader } from "@/components/panel";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { CreateManagedDatabaseDialog } from "@/features/project/create-managed-database-dialog";
import { DeleteResourceDialog } from "@/features/project/delete-resource-dialog";
import { RotateDatabasePasswordDialog } from "@/features/project/rotate-managed-database-password-dialog";
import { ResourceSectionHeader } from "@/features/project/resource-list";
import {
  DANGER_ACTION,
  PRIMARY_ACTION,
  SECONDARY_ACTION,
} from "@/features/user/action-styles";
import { formatBytes } from "@/lib/format";
import { cn } from "@/lib/utils";

type ManagedDatabase = components["schemas"]["ManagedDatabase"];
type ManagedDatabaseBackup = components["schemas"]["ManagedDatabaseBackup"];

function formatBackupTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}

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

function envBlock(database: ManagedDatabase, password: string) {
  return [
    `DB_HOST=${database.host}`,
    `DB_PORT=${database.port}`,
    `DB_NAME=${database.database_name}`,
    `DB_USER=${database.username}`,
    `DB_PASSWORD=${password}`,
    `DB_URL=mysql://${database.username}:${encodeURIComponent(password)}@${database.host}:${database.port}/${database.database_name}`,
  ].join("\n");
}

function UsageBar({
  label,
  detail,
  percent,
}: {
  label: string;
  detail: string;
  percent: number;
}) {
  const clamped = Math.max(0, Math.min(100, Math.round(percent)));
  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex flex-wrap items-baseline justify-between gap-x-3">
        <span className="text-[12.5px] text-[var(--projects-text)]">
          {label}
        </span>
        <span className="text-[12px] text-[var(--projects-muted)]">
          {detail}
        </span>
      </div>
      <div
        role="progressbar"
        aria-label={`${label} used`}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={clamped}
        className="h-1.5 w-full overflow-hidden rounded-full bg-[var(--projects-control)]"
      >
        <div
          className={cn(
            "h-full rounded-full transition-[width] duration-300 motion-reduce:transition-none",
            clamped >= 90
              ? "bg-[var(--projects-danger)]"
              : "bg-[var(--user-accent)]",
          )}
          style={{ width: `${clamped}%` }}
        />
      </div>
    </div>
  );
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

function BackupsPanel({
  projectId,
  databaseId,
}: {
  projectId: string;
  databaseId: string;
}) {
  const backups = useManagedDatabaseBackups(projectId, databaseId);
  const restore = useRestoreManagedDatabase(projectId, databaseId);
  const [restoreTarget, setRestoreTarget] =
    useState<ManagedDatabaseBackup | null>(null);

  const items = backups.data?.backups ?? [];
  const latestRestore = backups.data?.latest_restore;

  const restoreStatus = latestRestore
    ? ({
        queued: "Queued — waiting for the worker.",
        running: "Restoring…",
        succeeded: "Last restore completed.",
        failed: "Last restore failed.",
      }[latestRestore.status] ?? latestRestore.status)
    : null;

  const confirmRestore = async () => {
    if (!restoreTarget) return;
    try {
      await restore.mutateAsync(restoreTarget.id);
      toast.success("Restore queued. It runs within a minute.");
    } catch (cause) {
      toast.error(
        cause instanceof Error ? cause.message : "Could not queue the restore.",
      );
    } finally {
      setRestoreTarget(null);
    }
  };

  return (
    <Panel labelledBy="managed-database-backups">
      <PanelHeader
        id="managed-database-backups"
        title="Backups"
        subtitle="Daily snapshots are kept for about a week."
        right={
          latestRestore &&
          (latestRestore.status === "queued" ||
            latestRestore.status === "running") ? (
            <span className="inline-flex shrink-0 items-center gap-1.5 whitespace-nowrap text-[12px] text-[var(--projects-muted)]">
              <span className="size-2 animate-pulse rounded-full bg-[var(--user-accent)] motion-reduce:animate-none" />
              {restoreStatus}
            </span>
          ) : undefined
        }
      />

      {latestRestore?.status === "failed" ? (
        <p
          role="alert"
          className="m-0 mt-2 rounded-md border border-[var(--projects-danger)]/40 bg-[var(--projects-danger)]/10 px-3 py-2 text-[12.5px] leading-5 text-[var(--projects-danger)]"
        >
          {latestRestore.error || "The last restore failed."}
        </p>
      ) : null}

      {backups.isPending ? (
        <p className="m-0 text-[13px] text-[var(--projects-muted)]">
          Loading snapshots…
        </p>
      ) : backups.isError ? (
        <p className="m-0 text-[13px] text-[var(--projects-muted)]">
          Snapshots are unavailable right now.
        </p>
      ) : items.length === 0 ? (
        <p className="m-0 text-[13px] text-[var(--projects-muted)]">
          No snapshots yet. The first backup runs within a day of creating the
          database.
        </p>
      ) : (
        <ul className="m-0 flex list-none flex-col p-0">
          {items.map((backup) => (
            <li
              key={backup.id}
              className="flex flex-wrap items-center gap-x-3 gap-y-1 border-b border-[var(--projects-divider)] py-2.5 last:border-b-0"
            >
              <span className="min-w-0 flex-1 text-[13px] text-[var(--projects-text)]">
                {formatBackupTime(backup.created_at)}
              </span>
              <span className="text-[12px] text-[var(--projects-muted)]">
                {formatBytes(backup.size_bytes)}
              </span>
              <button
                type="button"
                disabled={restore.isPending}
                title="Restore this snapshot (overwrites current data)"
                onClick={() => setRestoreTarget(backup)}
                className="inline-flex items-center gap-1.5 rounded-md border border-[var(--projects-border)] px-2 py-1 text-[12px] font-medium text-[var(--projects-text)] transition-colors hover:border-[var(--projects-border-hover)] hover:bg-[var(--projects-control)] disabled:opacity-50"
              >
                <RotateCcw
                  className="size-3.5"
                  strokeWidth={2}
                  aria-hidden="true"
                />
                Restore
              </button>
            </li>
          ))}
        </ul>
      )}

      <RestoreDatabaseDialog
        open={restoreTarget !== null}
        onOpenChange={(next) => {
          if (!next) setRestoreTarget(null);
        }}
        backupTime={
          restoreTarget ? formatBackupTime(restoreTarget.created_at) : ""
        }
        onConfirm={confirmRestore}
        pending={restore.isPending}
      />
    </Panel>
  );
}

function RestoreDatabaseDialog({
  open,
  onOpenChange,
  backupTime,
  onConfirm,
  pending,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  backupTime: string;
  onConfirm: () => Promise<void>;
  pending: boolean;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md" data-user-surface="">
        <DialogHeader>
          <DialogTitle>Restore this snapshot?</DialogTitle>
          <DialogDescription>
            Restoring the {backupTime} snapshot overwrites everything currently
            in the database. This cannot be undone.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter className="mt-4">
          <Button
            type="button"
            className={SECONDARY_ACTION}
            onClick={() => onOpenChange(false)}
          >
            Cancel
          </Button>
          <Button
            type="button"
            className={DANGER_ACTION}
            disabled={pending}
            onClick={() => void onConfirm()}
          >
            {pending ? "Restoring…" : "Restore"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
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
  const rotate = useRotateManagedDatabasePassword(projectId);
  const [password, setPassword] = useState<string | null>(null);
  const [rotateOpen, setRotateOpen] = useState(false);

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
    <div className="grid grid-cols-1 gap-4 lg:grid-cols-[minmax(0,1fr)_20rem] lg:items-start">
      <div className="flex min-w-0 flex-col gap-4">
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
            <ConnectionRow
              label="Host"
              value={database.host}
              copyLabel="Host"
            />
            <ConnectionRow
              label="Port"
              value={String(database.port)}
              copyLabel="Port"
            />
            <ConnectionRow
              label="Nama database"
              value={database.database_name}
              copyLabel="Database name"
            />
            <ConnectionRow
              label="Username login"
              value={database.username}
              copyLabel="Username login"
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
                  className="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-md border border-[var(--projects-border)] px-2 text-[12px] font-medium text-[var(--projects-text)] transition-colors hover:border-[var(--projects-border-hover)] hover:bg-[var(--projects-control)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--user-accent)]/40 disabled:opacity-50"
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
                  {password ? "Hide" : "Reveal"}
                </button>
              </dd>
            </div>
          </dl>

          <div className="mt-3 rounded-lg border border-[var(--projects-border)] bg-[var(--projects-control)] px-3 py-2.5 text-[12.5px] leading-5 text-[var(--projects-muted)]">
            <strong className="font-semibold text-[var(--projects-text)]">
              Login phpMyAdmin:
            </strong>{" "}
            Nama Pengguna ={" "}
            <Mono className="text-[var(--projects-text)]">
              {database.username}
            </Mono>
            , Kata Sandi = hasil <em>Reveal</em>. Jangan masukkan{" "}
            <Mono className="text-[var(--projects-text)]">
              {database.database_name}
            </Mono>{" "}
            di kolom Nama Pengguna; itu nama database, bukan user.
          </div>

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
            <button
              type="button"
              disabled={!password}
              title={
                password
                  ? "Copy environment block"
                  : "Reveal the password first"
              }
              onClick={() => {
                if (password)
                  copyValue(".env block", envBlock(database, password));
              }}
              className={cn(
                SECONDARY_ACTION,
                "inline-flex items-center gap-1.5 px-3 text-[13px] disabled:cursor-not-allowed disabled:opacity-50",
              )}
            >
              <ClipboardCopy
                className="size-3.5"
                strokeWidth={2}
                aria-hidden="true"
              />
              Copy .env block
            </button>
            <button
              type="button"
              onClick={() => setRotateOpen(true)}
              className={cn(
                SECONDARY_ACTION,
                "inline-flex items-center gap-1.5 px-3 text-[13px]",
              )}
            >
              <KeyRound
                className="size-3.5"
                strokeWidth={2}
                aria-hidden="true"
              />
              Rotate password
            </button>
            <p className="m-0 text-[12px] leading-4 text-[var(--projects-muted)]">
              Log in with the username and password above.
            </p>
          </div>
        </Panel>

        <BackupsPanel projectId={projectId} databaseId={database.id} />
      </div>

      <div className="flex min-w-0 flex-col gap-4">
        <Panel labelledBy="managed-database-usage">
          <PanelHeader
            id="managed-database-usage"
            title="Usage"
            subtitle="Free-tier fair use, measured from MySQL itself."
          />
          {database.write_locked ? (
            <p
              role="alert"
              className="m-0 mb-3 rounded-md border border-[var(--projects-warning)]/50 bg-[var(--projects-warning)]/10 px-3 py-2 text-[12.5px] leading-5 text-[var(--projects-warning)]"
            >
              Storage is over the 256 MB fair-use limit, so writes are paused.
              Delete or export data to restore them.
            </p>
          ) : null}
          <div className="flex flex-col gap-3">
            <UsageBar
              label="Storage"
              detail={`${formatBytes(database.size_bytes)} of ${formatBytes(
                database.storage_limit_bytes,
              )}`}
              percent={
                database.storage_limit_bytes > 0
                  ? (database.size_bytes / database.storage_limit_bytes) * 100
                  : 0
              }
            />
            <UsageBar
              label="Connections"
              detail={`${database.connection_count} of ${database.connection_limit}`}
              percent={
                database.connection_limit > 0
                  ? (database.connection_count / database.connection_limit) *
                    100
                  : 0
              }
            />
          </div>
        </Panel>

        <Panel>
          <PanelHeader title="Free tier" />
          <ul className="m-0 flex list-none flex-col gap-1.5 p-0 text-[13px] leading-5 text-[var(--projects-muted)]">
            <li>One MySQL 8.4 database per project.</li>
            <li>256 MB fair-use storage, 10 concurrent connections.</li>
            <li>
              MySQL is not published to the internet; phpMyAdmin is the web
              door.
            </li>
            <li>
              Daily snapshots kept for about a week. Deleting is permanent.
            </li>
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

      {rotateOpen ? (
        <RotateDatabasePasswordDialog
          open
          onOpenChange={(next) => {
            if (!next) setRotateOpen(false);
          }}
          databaseName={database.name}
          onConfirm={async () => {
            const result = await rotate.mutateAsync(database.id);
            setPassword(result?.password ?? null);
            toast.success("Password rotated. Update your apps.");
          }}
        />
      ) : null}
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
        title="Databases"
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
