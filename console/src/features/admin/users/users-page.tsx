"use client";

import { useMemo, useState, type ReactNode } from "react";
import { ChevronLeft, ChevronRight, RefreshCw } from "lucide-react";
import { useAdminAccounts } from "@/api/queries/admin-telemetry";
import { cn } from "@/lib/utils";
import { AdminPageBody, Mono } from "../components/admin-panel";
import { ToolbarSearch } from "../components/toolbar-search";
import { GradientAvatar, VerifiedBadge, maskEmail } from "./user-table-pieces";

/**
 * Column template shared by the header row and every data row, mirroring the
 * redesigned Projects list table. Kept on one string so the header and rows can
 * never drift apart.
 */
const tableColumns =
  "grid-cols-[minmax(220px,1.6fr)_minmax(88px,.55fr)_minmax(120px,.85fr)_minmax(110px,.7fr)]";

function roleOf(instanceRole?: string): string {
  return instanceRole ? instanceRole.replace("instance_", "") : "member";
}

/** Short date like "9/10/2026". */
function shortDate(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return "—";
  return date.toLocaleDateString();
}

/**
 * Users — the real Console account directory, laid out as the redesigned
 * Projects table: a title row, a toolbar, then a bordered panel with a header
 * row, data rows, and a "Showing X of Y" footer. Emails are masked and verified
 * addresses carry a blue check. Nothing here is mock data.
 */
export function UsersPage() {
  const [query, setQuery] = useState("");
  const accounts = useAdminAccounts();
  const items = useMemo(() => accounts.data?.items ?? [], [accounts.data]);

  const visible = useMemo(() => {
    const term = query.trim().toLowerCase();
    if (!term) return items;
    // Search matches the real email even though the list renders it masked,
    // so an operator can still find an account without reading it aloud.
    return items.filter(
      (account) =>
        account.email.toLowerCase().includes(term) ||
        (account.display_name ?? "").toLowerCase().includes(term) ||
        account.id.toLowerCase().includes(term),
    );
  }, [items, query]);

  return (
    <AdminPageBody>
      {/* Title row — big heading + count pill + subtitle, Refresh on the right. */}
      <header className="flex flex-wrap items-start justify-between gap-x-4 gap-y-3 border-b border-[var(--projects-border)] pb-4">
        <div className="min-w-0">
          <div className="flex items-center gap-2.5">
            <h1 className="m-0 text-[26px] font-semibold leading-8 tracking-[-0.035em] text-[var(--projects-text)]">
              Users
            </h1>
            <span className="inline-flex h-7 items-center rounded-full bg-[color-mix(in_srgb,var(--projects-accent)_14%,transparent)] px-2.5 text-xs font-medium text-[var(--projects-accent)]">
              {items.length} {items.length === 1 ? "account" : "accounts"}
            </span>
          </div>
          <p className="m-0 mt-2 text-[14px] leading-5 text-[var(--projects-muted)]">
            People with access to the platform and their activity.
          </p>
        </div>
        <button
          type="button"
          onClick={() => void accounts.refetch()}
          disabled={accounts.isFetching}
          className="inline-flex h-10 items-center justify-center gap-2 rounded-[10px] border border-[var(--projects-border)] bg-[var(--projects-surface)] px-4 text-[13px] font-medium leading-none text-[var(--projects-text)] transition-colors hover:bg-white/[0.04] disabled:opacity-60"
        >
          <RefreshCw
            size={15}
            strokeWidth={1.8}
            aria-hidden="true"
            className={accounts.isFetching ? "animate-spin" : undefined}
          />
          Refresh
        </button>
      </header>

      {/* Toolbar. */}
      <div className="flex flex-wrap items-center gap-2.5">
        <ToolbarSearch
          value={query}
          onChange={setQuery}
          placeholder="Search name or email..."
          label="Search users"
        />
        <Mono className="ml-auto text-[11.5px] text-[var(--projects-muted)]">
          {visible.length} of {items.length} shown
        </Mono>
      </div>

      {/* Panel: header row + data rows + pagination footer, exactly like the
          redesigned Projects list. Horizontal scroll on narrow screens. */}
      <div className="overflow-hidden rounded-lg border border-[var(--projects-border)] bg-[#141416]">
        <div className="overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
          <div className="min-w-[640px]">
            <div
              className={cn(
                "grid items-center bg-[var(--projects-control)] px-4 py-3 text-[12px] font-medium text-[var(--projects-muted)] sm:px-5",
                tableColumns,
              )}
            >
              <ColLabel>User</ColLabel>
              <ColLabel>Role</ColLabel>
              <ColLabel>Last login</ColLabel>
              <ColLabel>Added</ColLabel>
            </div>

            {accounts.isPending ? (
              Array.from({ length: 3 }).map((_, index) => (
                <div
                  key={index}
                  className={cn(
                    "grid items-center border-t border-[var(--projects-divider)] bg-[var(--projects-card-bg)] px-4 py-3.5 sm:px-5",
                    tableColumns,
                  )}
                >
                  <div className="flex min-w-0 items-center gap-3">
                    <div className="size-8 animate-pulse rounded-full bg-white/[0.05] motion-reduce:animate-none" />
                    <div className="space-y-1.5">
                      <div className="h-3.5 w-28 animate-pulse rounded bg-white/[0.05] motion-reduce:animate-none" />
                      <div className="h-3 w-36 animate-pulse rounded bg-white/[0.05] motion-reduce:animate-none" />
                    </div>
                  </div>
                  <div className="h-3.5 w-14 animate-pulse rounded bg-white/[0.05] motion-reduce:animate-none" />
                  <div className="h-3.5 w-16 animate-pulse rounded bg-white/[0.05] motion-reduce:animate-none" />
                  <div className="h-3.5 w-16 animate-pulse rounded bg-white/[0.05] motion-reduce:animate-none" />
                </div>
              ))
            ) : accounts.isError ? (
              <div className="border-t border-[var(--projects-divider)] bg-[var(--projects-card-bg)] px-4 py-14 text-center text-[13px] text-[var(--projects-danger)]">
                Could not load accounts. Retry in a moment.
              </div>
            ) : visible.length === 0 ? (
              <div className="border-t border-[var(--projects-divider)] bg-[var(--projects-card-bg)] px-4 py-14 text-center text-[13px] text-[var(--projects-muted)]">
                {items.length === 0
                  ? "No accounts yet."
                  : "No accounts match this search."}
              </div>
            ) : (
              visible.map((account) => {
                const role = roleOf(account.instance_role);
                const roleLabel = role.charAt(0).toUpperCase() + role.slice(1);
                const name = account.display_name?.trim();
                return (
                  <div
                    key={account.id}
                    className={cn(
                      "grid items-center border-t border-[var(--projects-divider)] bg-[var(--projects-card-bg)] px-4 py-3.5 transition-colors hover:bg-[var(--projects-control)] sm:px-5",
                      tableColumns,
                    )}
                  >
                    <div className="flex min-w-0 items-center gap-3">
                      <GradientAvatar
                        email={account.email}
                        url={account.avatar_url}
                        size={32}
                      />
                      <div className="min-w-0">
                        <p className="m-0 flex items-center gap-1.5 text-[14px] font-semibold leading-5 text-[var(--projects-text)]">
                          <span className="truncate">
                            {name || maskEmail(account.email)}
                          </span>
                          {account.email_verified ? <VerifiedBadge /> : null}
                        </p>
                        <Mono className="m-0 mt-0.5 block truncate text-[12px] leading-4 text-[var(--projects-muted)]">
                          {name
                            ? maskEmail(account.email)
                            : `${account.id.slice(0, 8)}…`}
                        </Mono>
                      </div>
                    </div>

                    <span className="inline-flex min-w-0 items-center gap-2 truncate text-[13px] text-[var(--projects-text)]">
                      <span
                        aria-hidden="true"
                        className="size-1.5 shrink-0 rounded-full bg-[var(--projects-muted)]"
                      />
                      <span className="truncate">{roleLabel}</span>
                    </span>

                    <Mono className="truncate text-[12.5px] text-[var(--projects-muted)]">
                      {account.last_sign_in_at
                        ? shortDate(account.last_sign_in_at)
                        : "Never"}
                    </Mono>

                    <Mono className="truncate text-[12.5px] text-[var(--projects-muted)]">
                      {shortDate(account.created_at)}
                    </Mono>
                  </div>
                );
              })
            )}
          </div>
        </div>

        <footer className="grid grid-cols-3 items-center gap-4 border-t border-[var(--projects-divider)] bg-[var(--projects-card-bg)] px-4 py-3 sm:px-5">
          <p className="m-0 text-xs text-[var(--projects-muted)]">
            Showing {visible.length} of {items.length}{" "}
            {items.length === 1 ? "account" : "accounts"}
          </p>
          <div className="flex items-center justify-center gap-1.5">
            <button
              type="button"
              aria-label="Previous page"
              disabled
              className="inline-flex size-7 items-center justify-center rounded-md border border-[var(--projects-border)] text-[var(--projects-muted)] opacity-50"
            >
              <ChevronLeft size={14} strokeWidth={2} aria-hidden="true" />
            </button>
            <Mono className="inline-flex h-7 min-w-7 items-center justify-center rounded-md bg-[var(--projects-control)] px-1.5 text-xs text-[var(--projects-text)]">
              1
            </Mono>
            <button
              type="button"
              aria-label="Next page"
              disabled
              className="inline-flex size-7 items-center justify-center rounded-md border border-[var(--projects-border)] text-[var(--projects-muted)] opacity-50"
            >
              <ChevronRight size={14} strokeWidth={2} aria-hidden="true" />
            </button>
          </div>
          <span aria-hidden="true" />
        </footer>
      </div>
    </AdminPageBody>
  );
}

function ColLabel({ children }: { children: ReactNode }) {
  return <span className="truncate">{children}</span>;
}
