import Link from "next/link";
import type { ReactNode } from "react";
import { RUNS } from "../data/admin-mock-data";
import { AdminPanel, AdminPanelHeader, Mono } from "../components/admin-panel";
import { RunStatusBadge } from "../components/domain-badges";

/** The four most recent agent runs, dense table. */
export function RecentRuns({ className }: { className?: string }) {
  const runs = RUNS.slice(0, 4);

  return (
    <AdminPanel className={className}>
      <AdminPanelHeader
        title="Recent Agent Runs"
        right={
          <Link
            href="/admin/runs"
            className="text-[12px] font-medium text-[var(--projects-muted)] transition-colors hover:text-[var(--projects-text)]"
          >
            View all
          </Link>
        }
      />
      <div
        aria-hidden="true"
        className="hidden grid-cols-[minmax(0,1.1fr)_0.8fr_minmax(0,1.5fr)_1fr_0.8fr_1fr_0.8fr] gap-3 border-b border-[var(--projects-divider)] px-3 pb-2 lg:grid"
      >
        <ColumnLabel>Run ID</ColumnLabel>
        <ColumnLabel>User</ColumnLabel>
        <ColumnLabel>Agent</ColumnLabel>
        <ColumnLabel>Model</ColumnLabel>
        <ColumnLabel>Duration</ColumnLabel>
        <ColumnLabel>Status</ColumnLabel>
        <ColumnLabel>Started</ColumnLabel>
      </div>
      <ul className="m-0 list-none p-0">
        {runs.map((run) => (
          <li
            key={run.id}
            className="border-b border-[var(--projects-divider)] px-3 py-2.5 transition-colors last:border-b-0 hover:bg-white/[0.02] lg:grid lg:grid-cols-[minmax(0,1.1fr)_0.8fr_minmax(0,1.5fr)_1fr_0.8fr_1fr_0.8fr] lg:items-center lg:gap-3"
          >
            <div className="flex items-center justify-between gap-2">
              <Mono className="truncate text-[12px] font-medium leading-5 text-[var(--projects-text)]">{run.id}</Mono>
              <span className="lg:hidden">
                <RunStatusBadge status={run.status} />
              </span>
            </div>
            <span className="mt-2 block text-[12px] leading-5 text-[var(--projects-muted)] lg:mt-0 lg:text-[13px] lg:text-[var(--projects-text)]">
              {run.user}
            </span>
            <span className="mt-0.5 block truncate text-[12px] leading-5 text-[var(--projects-muted)] lg:mt-0 lg:text-[13px]">
              {run.agent}
            </span>
            <Mono className="mt-0.5 block truncate text-[11.5px] leading-5 text-[var(--projects-muted)] lg:mt-0">
              {run.model}
            </Mono>
            <Mono className="mt-0.5 block text-[11.5px] leading-5 text-[var(--projects-muted)] lg:mt-0">
              {run.duration}
            </Mono>
            <span className="mt-0 hidden lg:mt-0 lg:block">
              <RunStatusBadge status={run.status} />
            </span>
            <Mono className="mt-0 hidden text-[11.5px] leading-5 text-[var(--projects-muted)] lg:mt-0 lg:block">
              {run.startedAt}
            </Mono>
            <Mono className="mt-1 text-[11px] leading-4 text-[var(--projects-muted)] lg:hidden">
              started {run.startedAt}
            </Mono>
          </li>
        ))}
      </ul>
    </AdminPanel>
  );
}

function ColumnLabel({ children }: { children: ReactNode }) {
  return (
    <span className="text-[10.5px] font-medium uppercase tracking-[0.08em] text-[var(--projects-muted)]">{children}</span>
  );
}
