"use client";

import { useMemo, useState, type ReactNode } from "react";
import { TRACES } from "../data/admin-mock-data";
import { AdminHeader, AdminPageBody, Mono } from "../components/admin-panel";
import { StatTile } from "../components/stat-tile";
import { DetailDrawer, DetailField } from "../components/detail-drawer";
import { CopyButton } from "../components/copy-button";
import { ToolbarSearch } from "../components/toolbar-search";
import { AdminSelect } from "../components/admin-select";
import { StatusBadge } from "../components/status-badge";
import { formatDuration } from "../lib/format";
import { TraceWaterfall } from "./trace-waterfall";
import type { Trace } from "../types/traces";
import { cn } from "@/lib/utils";

const TRACE_STATS = [
  { id: "requests", label: "Requests · 1h", value: "12.8k" },
  { id: "p50", label: "P50 Latency", value: "82 ms" },
  { id: "p95", label: "P95 Latency", value: "340 ms" },
  { id: "p99", label: "P99 Latency", value: "821 ms" },
  { id: "error-rate", label: "Error Rate", value: "0.42%", tone: "warning" as const },
];

type TraceFilter = "all" | "success" | "error";
type ServiceFilter = "all" | string;

const TRACE_SERVICES = ["api", "worker", "sandbox", "gateway"];

/**
 * Traces — request metrics plus the trace table; selecting a row opens the
 * span waterfall in a detail drawer.
 */
export function TracesPage() {
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState<TraceFilter>("all");
  const [service, setService] = useState<ServiceFilter>("all");
  const [selected, setSelected] = useState<Trace | null>(null);

  const visible = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase();
    return TRACES.filter((trace) => {
      if (status !== "all" && trace.status !== status) return false;
      if (service !== "all" && trace.service !== service) return false;
      if (!normalizedQuery) return true;
      return [trace.id, trace.operation, trace.service].join(" ").toLowerCase().includes(normalizedQuery);
    });
  }, [query, status, service]);

  return (
    <AdminPageBody>
      <AdminHeader title="Traces" subtitle="Distributed request traces across api, workers, and sandboxes." />

      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-5">
        {TRACE_STATS.map((stat) => (
          <StatTile key={stat.id} label={stat.label} value={stat.value} tone={stat.tone} />
        ))}
      </div>

      <div className="flex flex-wrap items-center gap-2.5">
        <ToolbarSearch value={query} onChange={setQuery} placeholder="Search traces..." label="Search traces" />
        <AdminSelect
          label="Filter by service"
          value={service}
          onChange={(value) => setService(value as ServiceFilter)}
          options={[{ value: "all", label: "All services" }, ...TRACE_SERVICES.map((item) => ({ value: item, label: item }))]}
        />
        <AdminSelect
          label="Filter by status"
          value={status}
          onChange={(value) => setStatus(value as TraceFilter)}
          options={[
            { value: "all", label: "All statuses" },
            { value: "success", label: "Success" },
            { value: "error", label: "Error" },
          ]}
        />
      </div>

      <div className="overflow-hidden rounded-lg border border-[var(--projects-border)] bg-[#141416]">
        <div
          aria-hidden="true"
          className="hidden grid-cols-[110px_90px_minmax(0,1.6fr)_90px_80px_100px_70px] gap-3 border-b border-[var(--projects-divider)] px-3.5 py-2 lg:grid"
        >
          <ColumnLabel>Trace</ColumnLabel>
          <ColumnLabel>Service</ColumnLabel>
          <ColumnLabel>Operation</ColumnLabel>
          <ColumnLabel>Duration</ColumnLabel>
          <ColumnLabel>Spans</ColumnLabel>
          <ColumnLabel>Status</ColumnLabel>
          <ColumnLabel>Time</ColumnLabel>
        </div>
        <ul className="m-0 list-none p-0">
          {visible.length === 0 ? (
            <li className="px-4 py-12 text-center text-[13px] text-[var(--projects-muted)]">
              No traces match the current filters.
            </li>
          ) : (
            visible.map((trace) => (
              <li key={trace.id} className="border-b border-[var(--projects-divider)] last:border-b-0">
                <button
                  type="button"
                  onClick={() => setSelected(trace)}
                  aria-label={`Inspect trace ${trace.id}`}
                  className="block w-full px-3.5 py-2 text-left transition-colors hover:bg-white/[0.03]"
                >
                  <span className="hidden items-center gap-3 lg:grid lg:grid-cols-[110px_90px_minmax(0,1.6fr)_90px_80px_100px_70px]">
                    <Mono className="text-[12px] font-medium text-[var(--projects-text)]">{trace.id}</Mono>
                    <Mono className="text-[11.5px] text-[#b3b0ba]">{trace.service}</Mono>
                    <Mono className="truncate text-[12px] text-[var(--projects-text)]">{trace.operation}</Mono>
                    <Mono className={cn("text-[11.5px]", trace.status === "error" ? "text-[var(--projects-danger)]" : "text-[var(--projects-text)]")}>
                      {formatDuration(trace.duration)}
                    </Mono>
                    <Mono className="text-[11.5px] text-[var(--projects-muted)]">{trace.spanList.length} spans</Mono>
                    <span>
                      {trace.status === "success" ? (
                        <StatusBadge tone="success" label="Success" />
                      ) : (
                        <StatusBadge tone="danger" label="Error" />
                      )}
                    </span>
                    <Mono className="text-[11px] text-[var(--projects-muted)]">{trace.timestamp}</Mono>
                  </span>
                  <span className="block lg:hidden">
                    <span className="flex items-center gap-2">
                      <Mono className="text-[12px] font-medium text-[var(--projects-text)]">{trace.id}</Mono>
                      <Mono className="truncate text-[11px] text-[#b3b0ba]">{trace.operation}</Mono>
                      <span className="ml-auto shrink-0">
                        {trace.status === "success" ? (
                          <StatusBadge tone="success" label="Success" />
                        ) : (
                          <StatusBadge tone="danger" label="Error" />
                        )}
                      </span>
                    </span>
                    <Mono className="mt-1 block text-[11px] text-[var(--projects-muted)]">
                      {trace.service} · {formatDuration(trace.duration)} · {trace.spanList.length} spans · {trace.timestamp}
                    </Mono>
                  </span>
                </button>
              </li>
            ))
          )}
        </ul>
      </div>

      <TraceDetail trace={selected} onClose={() => setSelected(null)} />
    </AdminPageBody>
  );
}

function TraceDetail({ trace, onClose }: { trace: Trace | null; onClose: () => void }) {
  return (
    <DetailDrawer
      open={trace !== null}
      onClose={onClose}
      title={
        <>
          <Mono className="truncate">{trace?.id}</Mono>
          {trace && (
            <span className="ml-2">
              {trace.status === "success" ? (
                <StatusBadge tone="success" label="Success" />
              ) : (
                <StatusBadge tone="danger" label="Error" />
              )}
            </span>
          )}
        </>
      }
      subtitle={trace ? `${trace.service} · ${trace.operation} · ${trace.timestamp}` : undefined}
    >
      {trace && (
        <div className="flex flex-col gap-4">
          <div className="grid grid-cols-2 gap-x-4">
            <DetailField label="Duration">{formatDuration(trace.duration)}</DetailField>
            <DetailField label="Spans">{trace.spanList.length}</DetailField>
            <DetailField label="Root service">{trace.service}</DetailField>
            <DetailField label="Trace ID">
              <span className="admin-mono flex items-center gap-1">
                {trace.id}
                <CopyButton text={trace.id} />
              </span>
            </DetailField>
          </div>
          <div>
            <p className="m-0 mb-2 text-[11px] font-medium uppercase tracking-[0.06em] text-[var(--projects-muted)]">
              Span waterfall
            </p>
            <TraceWaterfall spans={trace.spanList} totalDuration={trace.duration} />
          </div>
        </div>
      )}
    </DetailDrawer>
  );
}

function ColumnLabel({ children }: { children: ReactNode }) {
  return (
    <span className="text-[10.5px] font-medium uppercase tracking-[0.08em] text-[var(--projects-muted)]">{children}</span>
  );
}
