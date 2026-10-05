"use client";

import { useMemo, useState } from "react";
import { ERROR_GROUPS } from "../data/admin-mock-data";
import { AdminHeader, AdminPageBody, Mono } from "../components/admin-panel";
import { StatTile } from "../components/stat-tile";
import { DetailDrawer, DetailField } from "../components/detail-drawer";
import { CopyButton } from "../components/copy-button";
import { Sparkline } from "../components/sparkline";
import { StatusBadge } from "../components/status-badge";
import { AdminSelect } from "../components/admin-select";
import type { ErrorGroup } from "../types/errors";

const ERROR_STATS = [
  { id: "unresolved", label: "Unresolved", value: "12", tone: "danger" as const },
  { id: "new-today", label: "New today", value: "4", tone: "warning" as const },
  { id: "users", label: "Affected users", value: "18" },
  { id: "rate", label: "Error rate", value: "0.42%", tone: "warning" as const },
];

type ErrorFilter = "all" | "unresolved" | "resolved";

/**
 * Errors — grouped error signatures with trends; selecting a group opens
 * its sample stack trace in a detail drawer.
 */
export function ErrorsPage() {
  const [filter, setFilter] = useState<ErrorFilter>("all");
  const [service, setService] = useState<"all" | string>("all");
  const [selected, setSelected] = useState<ErrorGroup | null>(null);

  const services = useMemo(
    () => ["all", ...new Set(ERROR_GROUPS.map((group) => group.service))],
    [],
  );

  const visible = useMemo(
    () =>
      ERROR_GROUPS.filter((group) => {
        if (filter !== "all" && group.status !== filter) return false;
        if (service !== "all" && group.service !== service) return false;
        return true;
      }),
    [filter, service],
  );

  return (
    <AdminPageBody>
      <AdminHeader title="Errors" subtitle="Grouped error signatures across platform services." />

      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        {ERROR_STATS.map((stat) => (
          <StatTile key={stat.id} label={stat.label} value={stat.value} tone={stat.tone} />
        ))}
      </div>

      <div className="flex flex-wrap items-center gap-2.5">
        <AdminSelect
          label="Filter by status"
          value={filter}
          onChange={setFilter}
          options={[
            { value: "all", label: "All statuses" },
            { value: "unresolved", label: "Unresolved" },
            { value: "resolved", label: "Resolved" },
          ]}
        />
        <AdminSelect
          label="Filter by service"
          value={service}
          onChange={setService}
          options={services.map((item) => ({ value: item, label: item === "all" ? "All services" : item }))}
        />
        <Mono className="ml-auto text-[11.5px] text-[var(--projects-muted)]">
          {visible.length} groups
        </Mono>
      </div>

      <div className="overflow-hidden rounded-lg border border-[var(--projects-border)] bg-[#141416]">
        <ul className="m-0 list-none p-0">
          {visible.map((group) => (
            <li key={group.id} className="border-b border-[var(--projects-divider)] last:border-b-0">
              <button
                type="button"
                onClick={() => setSelected(group)}
                aria-label={`Inspect error ${group.name}`}
                className="block w-full px-4 py-3 text-left transition-colors hover:bg-white/[0.03]"
              >
                <div className="flex flex-wrap items-center gap-2.5">
                  <Mono className="text-[13px] font-medium text-[var(--projects-text)]">{group.name}</Mono>
                  {group.status === "unresolved" ? (
                    <StatusBadge tone="danger" label="Unresolved" pulse />
                  ) : (
                    <StatusBadge tone="success" label="Resolved" />
                  )}
                  <span className="ml-auto text-[12px] text-[var(--projects-muted)]">{group.service}</span>
                </div>
                <div className="mt-1.5 flex flex-wrap items-center gap-x-4 gap-y-1 text-[11.5px] text-[var(--projects-muted)]">
                  <span>
                    <Mono className="text-[var(--projects-text)]">{group.events}</Mono> events
                  </span>
                  <span>
                    <Mono className="text-[var(--projects-text)]">{group.users}</Mono> users
                  </span>
                  <span>last seen {group.lastSeen}</span>
                  <span>first seen {group.firstSeen}</span>
                </div>
                <p className="m-0 mt-1.5 truncate text-[12px] leading-4 text-[var(--projects-muted)]/85">
                  {group.message}
                </p>
                <div className="mt-2 max-w-[220px]">
                  <Sparkline data={group.trend} tone={group.status === "unresolved" ? "danger" : "neutral"} height={22} />
                  <p className="m-0 mt-0.5 text-[10px] uppercase tracking-[0.06em] text-[var(--projects-muted)]/70">
                    events · last 24 h
                  </p>
                </div>
              </button>
            </li>
          ))}
          {visible.length === 0 && (
            <li className="px-4 py-12 text-center text-[13px] text-[var(--projects-muted)]">
              No error groups match the current filters.
            </li>
          )}
        </ul>
      </div>

      <ErrorDetail group={selected} onClose={() => setSelected(null)} />
    </AdminPageBody>
  );
}

function ErrorDetail({ group, onClose }: { group: ErrorGroup | null; onClose: () => void }) {
  return (
    <DetailDrawer
      open={group !== null}
      onClose={onClose}
      title={<Mono className="truncate">{group?.name}</Mono>}
      subtitle={group ? `${group.service} · first seen ${group.firstSeen}` : undefined}
    >
      {group && (
        <div className="flex flex-col gap-4">
          <div className="grid grid-cols-2 gap-x-4">
            <DetailField label="Status">
              {group.status === "unresolved" ? (
                <StatusBadge tone="danger" label="Unresolved" />
              ) : (
                <StatusBadge tone="success" label="Resolved" />
              )}
            </DetailField>
            <DetailField label="Last seen">{group.lastSeen}</DetailField>
            <DetailField label="Events (24h)">{group.events}</DetailField>
            <DetailField label="Users affected">{group.users}</DetailField>
          </div>

          <div>
            <p className="m-0 mb-1.5 text-[11px] font-medium uppercase tracking-[0.06em] text-[var(--projects-muted)]">
              Message
            </p>
            <p className="m-0 rounded-lg border border-[var(--projects-border)] bg-[#0f0f11] p-3 text-[12.5px] leading-5 text-[var(--projects-text)]">
              {group.sample.message}
            </p>
          </div>

          <div>
            <p className="m-0 mb-1.5 text-[11px] font-medium uppercase tracking-[0.06em] text-[var(--projects-muted)]">
              Sample stack trace
            </p>
            <pre className="admin-scrollbar m-0 overflow-x-auto rounded-lg border border-[var(--projects-border)] bg-[#0f0f11] p-3 text-[11.5px] leading-5 text-[#c9c5cd]">
              {group.sample.stack.join("\n")}
            </pre>
            <div className="mt-2 flex justify-end">
              <span className="inline-flex items-center gap-1.5 text-[11.5px] text-[var(--projects-muted)]">
                Copy trace
                <CopyButton text={group.sample.stack.join("\n")} />
              </span>
            </div>
          </div>

          <div className="grid grid-cols-2 gap-x-4">
            <DetailField label="Environment">{group.sample.environment}</DetailField>
            <DetailField label="Timestamp">
              <Mono>{group.sample.timestamp}</Mono>
            </DetailField>
            <DetailField label="Request ID">
              <Mono className="flex items-center gap-1">
                {group.sample.requestId}
                <CopyButton text={group.sample.requestId} />
              </Mono>
            </DetailField>
            <DetailField label="Trace ID">
              <Mono className="flex items-center gap-1">
                {group.sample.traceId}
                <CopyButton text={group.sample.traceId} />
              </Mono>
            </DetailField>
          </div>
        </div>
      )}
    </DetailDrawer>
  );
}
