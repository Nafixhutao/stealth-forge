"use client";

import Link from "next/link";
import { INCIDENTS, STATUS_SERVICES } from "../data/admin-mock-data";
import { AdminHeader, AdminPageBody, Mono } from "../components/admin-panel";
import { AdminPanel, AdminPanelHeader } from "../components/admin-panel";
import { IncidentStatusBadge, SeverityBadge } from "../components/domain-badges";
import { cn } from "@/lib/utils";

/**
 * Status Page — the operator-facing health view: overall banner, 45-day
 * uptime history per service, and open incidents. Uptime numbers are always
 * printed so color never carries the data alone.
 */
export function StatusPage() {
  const degraded = STATUS_SERVICES.filter((service) => service.status !== "healthy");

  return (
    <AdminPageBody>
      <AdminHeader title="Status Page" subtitle="Public-style view of platform availability and history." />

      <section
        aria-label="Overall status"
        className={cn(
          "flex flex-wrap items-center gap-x-3 gap-y-1.5 rounded-lg border px-4 py-4",
          degraded.length === 0
            ? "border-[color-mix(in_srgb,var(--projects-accent)_40%,var(--projects-border))] bg-[color-mix(in_srgb,var(--projects-accent)_7%,#141416)]"
            : "border-[color-mix(in_srgb,var(--projects-warning)_40%,var(--projects-border))] bg-[color-mix(in_srgb,var(--projects-warning)_7%,#141416)]",
        )}
      >
        <span className="relative flex size-2.5">
          {degraded.length === 0 ? (
            <span className="absolute inline-flex size-full animate-ping rounded-full bg-[var(--projects-accent)] opacity-50" />
          ) : null}
          <span
            className={cn(
              "relative inline-flex size-2.5 rounded-full",
              degraded.length === 0 ? "bg-[var(--projects-accent)]" : "bg-[var(--projects-warning)]",
            )}
          />
        </span>
        <p className="m-0 text-[15px] font-semibold text-[var(--projects-text)]">
          {degraded.length === 0
            ? "All systems operational"
            : `Degraded performance — ${degraded.map((service) => service.name).join(", ")}`}
        </p>
        <Mono className="ml-auto text-[11.5px] text-[var(--projects-muted)]">
          {degraded.length === 0 ? "0 incidents open" : "1 incident open"}
        </Mono>
      </section>

      <AdminPanel>
        <AdminPanelHeader title="Service availability" subtitle="Last 45 days · percent of successful checks per day." />
        <ul className="m-0 list-none p-0">
          {STATUS_SERVICES.map((service) => (
            <li key={service.id} className="flex flex-col gap-2 border-b border-[var(--projects-divider)] py-3 last:border-b-0 lg:flex-row lg:items-center lg:gap-4">
              <div className="flex w-full min-w-0 items-center gap-2.5 lg:w-[220px] lg:shrink-0">
                <span className="truncate text-[13px] font-medium text-[var(--projects-text)]">{service.name}</span>
                <Mono className="ml-auto shrink-0 text-[11.5px] text-[var(--projects-muted)] lg:ml-0">
                  {service.uptime}
                </Mono>
              </div>
              <div className="flex h-8 min-w-0 flex-1 items-stretch gap-[2px]" role="img" aria-label={`${service.name} uptime ${service.uptime} over the last 45 days`}>
                {service.history.map((value, index) => (
                  <span
                    key={index}
                    title={`${value.toFixed(2)}%`}
                    className={cn(
                      "min-w-[3px] flex-1 rounded-[2px]",
                      value >= 99.9
                        ? "bg-[color-mix(in_srgb,var(--projects-accent)_65%,transparent)]"
                        : value >= 99
                          ? "bg-[var(--projects-warning)]"
                          : "bg-[var(--projects-danger)]",
                    )}
                  />
                ))}
              </div>
            </li>
          ))}
        </ul>
      </AdminPanel>

      <AdminPanel>
        <AdminPanelHeader
          title="Recent incidents"
          right={
            <Link href="/admin/incidents" className="text-[12px] font-medium text-[var(--projects-muted)] transition-colors hover:text-[var(--projects-text)]">
              Manage
            </Link>
          }
        />
        <ul className="m-0 list-none p-0">
          {INCIDENTS.slice(0, 3).map((incident) => (
            <li key={incident.id} className="flex flex-wrap items-center gap-2.5 border-b border-[var(--projects-divider)] py-2.5 last:border-b-0">
              <SeverityBadge severity={incident.severity} />
              <span className="text-[13px] text-[var(--projects-text)]">{incident.title}</span>
              <span className="ml-auto flex items-center gap-3">
                <Mono className="text-[11px] text-[var(--projects-muted)]">{incident.startedAt}</Mono>
                <IncidentStatusBadge status={incident.status} />
              </span>
            </li>
          ))}
        </ul>
      </AdminPanel>
    </AdminPageBody>
  );
}
