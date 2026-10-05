"use client";

import Link from "next/link";
import { TriangleAlert } from "lucide-react";
import { SERVICES } from "../data/admin-mock-data";
import { StatusBadge } from "../components/status-badge";
import { UpdatedLabel } from "../components/live-indicator";

/**
 * Top strip of the overview: overall system status derived from the service
 * list (any degraded service → amber "Degraded performance").
 */
export function SystemStatus({ resetKey }: { resetKey?: unknown }) {
  const degraded = SERVICES.filter((service) => service.status === "degraded");
  const down = SERVICES.filter((service) => service.status === "down");
  const operational = degraded.length === 0 && down.length === 0;
  const affected = [...down, ...degraded].map((service) => service.name).join(", ");

  return (
    <section
      aria-label="System status"
      className={
        operational
          ? "flex flex-wrap items-center gap-x-4 gap-y-2 rounded-lg border border-[var(--projects-border)] bg-[#141416] px-4 py-3.5"
          : "flex flex-wrap items-center gap-x-4 gap-y-2 rounded-lg border border-[color-mix(in_srgb,var(--projects-warning)_40%,var(--projects-border))] bg-[color-mix(in_srgb,var(--projects-warning)_7%,#141416)] px-4 py-3.5"
      }
    >
      {operational ? (
        <StatusBadge tone="success" label="All systems operational" className="text-[13px]" pulse />
      ) : (
        <StatusBadge tone="warning" label="Degraded performance" className="text-[13px]" pulse />
      )}
      <p className="m-0 min-w-0 flex-1 text-[12.5px] leading-5 text-[var(--projects-muted)]">
        {operational
          ? "Every platform service is responding within its latency budget."
          : `${affected} ${degraded.length + down.length === 1 ? "is" : "are"} degraded — the team is investigating.`}
      </p>
      <UpdatedLabel resetKey={resetKey} className="admin-mono shrink-0 text-[11.5px] text-[var(--projects-muted)]" />
      <Link
        href="/admin/incidents"
        className="inline-flex shrink-0 items-center gap-1.5 rounded-md border border-[var(--projects-border)] px-2.5 py-1.5 text-[12px] font-medium text-[var(--projects-text)] transition-colors hover:border-[var(--projects-border-hover)] hover:bg-white/[0.04]"
      >
        <TriangleAlert size={12} strokeWidth={1.8} aria-hidden="true" />
        View incidents
      </Link>
    </section>
  );
}
