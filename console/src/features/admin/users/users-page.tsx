"use client";

import { useMemo, useState, type ReactNode } from "react";
import { USERS } from "../data/admin-mock-data";
import { AdminHeader, AdminPageBody, Mono } from "../components/admin-panel";
import { ToolbarSearch } from "../components/toolbar-search";
import { AdminSelect } from "../components/admin-select";
import { StatusBadge } from "../components/status-badge";

const STATUS_TONE = {
  active: { tone: "success" as const, label: "Active" },
  idle: { tone: "neutral" as const, label: "Idle" },
  suspended: { tone: "danger" as const, label: "Suspended" },
};

type RoleFilter = "all" | "Owner" | "Admin" | "Member";
type UserStatusFilter = "all" | "active" | "idle" | "suspended";

/** Users — platform member directory with a dense table. */
export function UsersPage() {
  const [query, setQuery] = useState("");
  const [role, setRole] = useState<RoleFilter>("all");
  const [status, setStatus] = useState<UserStatusFilter>("all");

  const visible = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase();
    return USERS.filter((user) => {
      if (role !== "all" && user.role !== role) return false;
      if (status !== "all" && user.status !== status) return false;
      if (!normalizedQuery) return true;
      return [user.name, user.email].join(" ").toLowerCase().includes(normalizedQuery);
    });
  }, [query, role, status]);

  return (
    <AdminPageBody>
      <AdminHeader title="Users" subtitle="People with access to the platform and their activity." />

      <div className="flex flex-wrap items-center gap-2.5">
        <ToolbarSearch value={query} onChange={setQuery} placeholder="Search name or email..." label="Search users" />
        <AdminSelect
          label="Filter by role"
          value={role}
          onChange={setRole}
          options={[
            { value: "all", label: "All roles" },
            { value: "Owner", label: "Owner" },
            { value: "Admin", label: "Admin" },
            { value: "Member", label: "Member" },
          ]}
        />
        <AdminSelect
          label="Filter by status"
          value={status}
          onChange={setStatus}
          options={[
            { value: "all", label: "All statuses" },
            { value: "active", label: "Active" },
            { value: "idle", label: "Idle" },
            { value: "suspended", label: "Suspended" },
          ]}
        />
        <Mono className="ml-auto text-[11.5px] text-[var(--projects-muted)]">{visible.length} users</Mono>
      </div>

      <div className="overflow-hidden rounded-lg border border-[var(--projects-border)] bg-[#141416]">
        <div
          aria-hidden="true"
          className="hidden grid-cols-[minmax(0,1.6fr)_110px_110px_110px_minmax(0,1fr)] gap-3 border-b border-[var(--projects-divider)] px-3.5 py-2 lg:grid"
        >
          <ColLabel>User</ColLabel>
          <ColLabel>Role</ColLabel>
          <ColLabel>Runs · 30d</ColLabel>
          <ColLabel>Status</ColLabel>
          <ColLabel>Last active</ColLabel>
        </div>
        <ul className="m-0 list-none p-0">
          {visible.length === 0 ? (
            <li className="px-4 py-12 text-center text-[13px] text-[var(--projects-muted)]">No users found.</li>
          ) : (
            visible.map((user) => {
              const statusMeta = STATUS_TONE[user.status];
              return (
                <li
                  key={user.id}
                  className="border-b border-[var(--projects-divider)] px-3.5 py-2.5 transition-colors last:border-b-0 hover:bg-white/[0.02] lg:grid lg:grid-cols-[minmax(0,1.6fr)_110px_110px_110px_minmax(0,1fr)] lg:items-center lg:gap-3"
                >
                  <div className="flex min-w-0 items-center gap-2.5">
                    <span className="flex size-7 shrink-0 items-center justify-center rounded-full border border-[var(--projects-border)] bg-[var(--projects-control)] text-[11px] font-semibold text-[var(--projects-muted)]">
                      {user.name.split(" ").map((part) => part[0]).join("")}
                    </span>
                    <div className="min-w-0">
                      <p className="m-0 truncate text-[13px] font-medium leading-5 text-[var(--projects-text)]">{user.name}</p>
                      <Mono className="m-0 block truncate text-[11px] leading-4 text-[var(--projects-muted)]">{user.email}</Mono>
                    </div>
                  </div>
                  <span className="mt-2 block text-[12px] text-[var(--projects-muted)] lg:mt-0">
                    <span className="lg:hidden">Role: </span>
                    {user.role}
                  </span>
                  <Mono className="mt-1 block text-[12px] text-[var(--projects-text)] lg:mt-0">
                    <span className="font-sans text-[11px] text-[var(--projects-muted)] lg:hidden">Runs: </span>
                    {user.runs}
                  </Mono>
                  <span className="mt-1 block lg:mt-0">
                    <StatusBadge tone={statusMeta.tone} label={statusMeta.label} />
                  </span>
                  <Mono className="mt-1 block text-[11.5px] text-[var(--projects-muted)] lg:mt-0">
                    {user.lastActive}
                  </Mono>
                </li>
              );
            })
          )}
        </ul>
      </div>
    </AdminPageBody>
  );
}

function ColLabel({ children }: { children: ReactNode }) {
  return (
    <span className="text-[10.5px] font-medium uppercase tracking-[0.08em] text-[var(--projects-muted)]">{children}</span>
  );
}
