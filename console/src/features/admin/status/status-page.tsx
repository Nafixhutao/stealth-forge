"use client";

import Link from "next/link";
import { INCIDENTS, STATUS_SERVICES } from "../data/admin-mock-data";
import { AdminHeader, AdminPageBody, Mono } from "../components/admin-panel";
import { AdminPanel, AdminPanelHeader } from "../components/admin-panel";
import {
  IncidentStatusBadge,
  SeverityBadge,
} from "../components/domain-badges";
import { StatusBar } from "@/components/charts/status-bar";
import type {
  StatusBarDatum,
  StatusBarStatus,
} from "@/components/charts/status-bar/context";
import { StatusBarLegend } from "@/components/charts/status-bar/legend";
import { StatusBarPlot } from "@/components/charts/status-bar/plot";
import { StatusBarTooltip } from "@/components/charts/status-bar/tooltip";
import type { ServiceHealth } from "../types/telemetry";
import { cn } from "@/lib/utils";

// Colors come from the Console palette, not the chart's defaults, so the
// history reads as part of the admin surface.
const UPTIME_STATUSES: readonly StatusBarStatus[] = [
  { id: "operational", label: "Operational", color: "var(--projects-accent)" },
  { id: "degraded", label: "Degraded", color: "var(--projects-warning)" },
  { id: "outage", label: "Outage", color: "var(--projects-danger)" },
];

// The chart reports state in words rather than percentages, matching the
// reference chart: every period and every row is described by its status, so a
// reader never has to translate a number into a condition.
const STATUS_COPY: Record<
  ServiceHealth["status"],
  { id: string; label: string; description: string }
> = {
  healthy: {
    id: "operational",
    label: "Operational",
    description: "All checks passed.",
  },
  degraded: {
    id: "degraded",
    label: "Degraded",
    description: "Some checks degraded.",
  },
  down: {
    id: "outage",
    label: "Outage",
    description: "Checks failed.",
  },
};

/** Same thresholds the previous hand-rolled strip used. */
function uptimeStatus(value: number) {
  if (value >= 99.9) return "healthy";
  if (value >= 99) return "degraded";
  return "down";
}

/**
 * The chart never aggregates: one period per observed day, oldest first.
 * Labels are index-based rather than dates so the server and the client render
 * the same string.
 */
function uptimePeriods(
  serviceId: string,
  history: readonly number[],
): StatusBarDatum[] {
  return history.map((value, index) => {
    const status = STATUS_COPY[uptimeStatus(value)];
    return {
      id: `${serviceId}-${index}`,
      label: `Day ${index + 1}`,
      status: status.id,
      description: status.description,
    };
  });
}

/**
 * Status Page — the operator-facing health view: overall banner, 45-day
 * uptime history per service, and open incidents. Each service is described by
 * its state in words, so color never carries the data alone.
 */
export function StatusPage() {
  const degraded = STATUS_SERVICES.filter(
    (service) => service.status !== "healthy",
  );

  return (
    <AdminPageBody>
      <AdminHeader
        title="Status Page"
        subtitle="Public-style view of platform availability and history."
      />

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
              degraded.length === 0
                ? "bg-[var(--projects-accent)]"
                : "bg-[var(--projects-warning)]",
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
        <AdminPanelHeader
          title="Service availability"
          subtitle="Last 45 days · percent of successful checks per day."
        />
        {/* One legend for the whole panel; each row renders its own plot. */}
        <StatusBar
          data={[]}
          statuses={UPTIME_STATUSES}
          label="Uptime status legend"
          className="pb-2"
        >
          <StatusBarLegend />
        </StatusBar>
        <ul className="m-0 list-none p-0">
          {STATUS_SERVICES.map((service) => (
            <li
              key={service.id}
              className="flex flex-col gap-2 border-b border-[var(--projects-divider)] py-3 last:border-b-0 lg:flex-row lg:items-center lg:gap-4"
            >
              <div className="flex w-full min-w-0 items-center gap-2.5 lg:w-[220px] lg:shrink-0">
                <span className="truncate text-[13px] font-medium text-[var(--projects-text)]">
                  {service.name}
                </span>
                {/* Word, not a percentage, and in the UI font rather than mono:
                    the reference chart describes each service by its state. */}
                <span className="ml-auto shrink-0 text-[13px] text-[var(--projects-muted)] lg:ml-0">
                  {STATUS_COPY[service.status].label}
                </span>
              </div>
              <StatusBar
                data={uptimePeriods(service.id, service.history)}
                statuses={UPTIME_STATUSES}
                label={`${service.name} status over the last 45 days`}
                // Capped to match the reference chart's density: bars are
                // flex-1, so a full-width plot on a desktop makes them roughly
                // 50% wider than intended and the history reads as chunky
                // rather than as a dense strip.
                className="min-w-0 flex-1 lg:max-w-[672px]"
              >
                {/* Fixed-width bars on a desktop, where the vendored flex-1
                    bars stretch to roughly 11px and read as chunky. The
                    leftover width is pushed into the gaps (see the
                    status-bar-plot rule in globals.css), so the strip still
                    runs Day 1 to Day 45 across the full plot. Below the
                    breakpoint the bars stay flex-1, which is what keeps the
                    45 periods fitting on a phone. Passed through the vendored
                    plot's own prop, so its source is untouched. */}
                <StatusBarPlot barClassName="lg:flex-[0_0_7px]" />
                <StatusBarTooltip />
              </StatusBar>
            </li>
          ))}
        </ul>
      </AdminPanel>

      <AdminPanel>
        <AdminPanelHeader
          title="Recent incidents"
          right={
            <Link
              href="/admin/incidents"
              className="text-[12px] font-medium text-[var(--projects-muted)] transition-colors hover:text-[var(--projects-text)]"
            >
              Manage
            </Link>
          }
        />
        <ul className="m-0 list-none p-0">
          {INCIDENTS.slice(0, 3).map((incident) => (
            <li
              key={incident.id}
              className="flex flex-wrap items-center gap-2.5 border-b border-[var(--projects-divider)] py-2.5 last:border-b-0"
            >
              <SeverityBadge severity={incident.severity} />
              <span className="text-[13px] text-[var(--projects-text)]">
                {incident.title}
              </span>
              <span className="ml-auto flex items-center gap-3">
                <Mono className="text-[11px] text-[var(--projects-muted)]">
                  {incident.startedAt}
                </Mono>
                <IncidentStatusBadge status={incident.status} />
              </span>
            </li>
          ))}
        </ul>
      </AdminPanel>
    </AdminPageBody>
  );
}
