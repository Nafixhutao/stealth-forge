import type { ReactNode } from "react";
import { SERVICES } from "../data/admin-mock-data";
import { AdminPanel, AdminPanelHeader, Mono } from "../components/admin-panel";
import { ServiceStatusBadge } from "../components/domain-badges";

/**
 * Service health list — Service / Status / Latency / Availability / Last
 * check. Desktop renders the column grid; compact viewports fold the
 * metrics into one inline metadata row.
 */
export function ServiceHealthTable({ className }: { className?: string }) {
  return (
    <AdminPanel className={className}>
      <AdminPanelHeader title="Service Health" subtitle="Synthetic checks run every 15 seconds from all regions." />
      <div
        aria-hidden="true"
        className="hidden grid-cols-[minmax(0,1.6fr)_1fr_0.9fr_1.1fr_0.9fr] gap-3 border-b border-[var(--projects-divider)] px-3 pb-2 lg:grid"
      >
        <ColumnLabel>Service</ColumnLabel>
        <ColumnLabel>Status</ColumnLabel>
        <ColumnLabel>Latency</ColumnLabel>
        <ColumnLabel>Availability</ColumnLabel>
        <ColumnLabel>Last check</ColumnLabel>
      </div>
      <ul className="m-0 list-none p-0">
        {SERVICES.map((service) => (
          <li
            key={service.id}
            className="border-b border-[var(--projects-divider)] px-3 py-2.5 transition-colors last:border-b-0 hover:bg-white/[0.02] lg:grid lg:grid-cols-[minmax(0,1.6fr)_1fr_0.9fr_1.1fr_0.9fr] lg:items-center lg:gap-3"
          >
            <div className="flex items-center justify-between gap-2 lg:block">
              <span className="text-[13px] font-medium leading-5 text-[var(--projects-text)]">{service.name}</span>
              <span className="lg:hidden">
                <ServiceStatusBadge status={service.status} />
              </span>
            </div>
            <span className="mt-0 hidden lg:block">
              <ServiceStatusBadge status={service.status} />
            </span>
            <Mono className="mt-2 hidden text-[12px] leading-5 text-[var(--projects-text)] lg:mt-0 lg:block">
              {service.latency}
            </Mono>
            <Mono className="hidden text-[12px] leading-5 text-[var(--projects-muted)] lg:block">
              {service.availability}
            </Mono>
            <Mono className="hidden text-[12px] leading-5 text-[var(--projects-muted)] lg:block">
              {service.lastCheck}
            </Mono>
            {/* compact metadata row */}
            <Mono className="mt-1.5 text-[11.5px] leading-4 text-[var(--projects-muted)] lg:hidden">
              {service.latency} · {service.availability} · checked {service.lastCheck}
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
