"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { LOGS, buildLiveLog } from "../data/admin-mock-data";
import { AdminHeader, AdminPageBody, Mono } from "../components/admin-panel";
import { AdminSelect } from "../components/admin-select";
import { TimeRangeSelect } from "../components/time-range-select";
import { LiveToggle, ToolbarSearch } from "../components/toolbar-search";
import { DetailDrawer, DetailField } from "../components/detail-drawer";
import { CopyButton } from "../components/copy-button";
import { StatusBadge } from "../components/status-badge";
import { useLiveTick } from "../hooks/use-live-updates";
import { cn } from "@/lib/utils";
import type { LogEntry, LogLevel } from "../types/logs";

const SERVICES = ["api", "worker", "sandbox", "database", "scheduler", "gateway"];
const LEVELS: LogLevel[] = ["INFO", "WARN", "ERROR", "DEBUG"];

const LEVEL_TONE: Record<LogLevel, { text: string; label: string }> = {
  INFO: { text: "text-[#a1a1aa]", label: "text-[#a1a1aa]" },
  WARN: { text: "text-[var(--projects-warning)]", label: "text-[var(--projects-warning)]" },
  ERROR: { text: "text-[var(--projects-danger)]", label: "text-[var(--projects-danger)]" },
  DEBUG: { text: "text-[#62626a]", label: "text-[#62626a]" },
};

type LevelFilter = "all" | LogLevel;
type ServiceFilter = "all" | string;
type EnvFilter = "all" | "production" | "staging";

/**
 * Logs — the log explorer: toolbar filters, mono stream, live tail, and a
 * right-hand detail drawer per entry. Frontend mock only.
 */
export function LogsPage() {
  const [query, setQuery] = useState("");
  const [range, setRange] = useState<"1h" | "6h" | "24h" | "7d">("1h");
  const [service, setService] = useState<ServiceFilter>("all");
  const [level, setLevel] = useState<LevelFilter>("all");
  const [environment, setEnvironment] = useState<EnvFilter>("all");
  const [liveTail, setLiveTail] = useState(false);
  const [entries, setEntries] = useState<LogEntry[]>(() => LOGS);
  const [selected, setSelected] = useState<LogEntry | null>(null);
  const tick = useLiveTick(4000, liveTail);
  const counterRef = useRef(0);

  // Live tail prepends a rotating mock entry every tick.
  useEffect(() => {
    if (tick === 0) return;
    const entry = buildLiveLog(counterRef.current, Date.now());
    counterRef.current += 1;
    setEntries((prev) => [entry, ...prev].slice(0, 80));
  }, [tick]);

  const visible = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase();
    return entries.filter((entry) => {
      if (service !== "all" && entry.service !== service) return false;
      if (level !== "all" && entry.level !== level) return false;
      if (environment !== "all" && entry.environment !== environment) return false;
      if (!normalizedQuery) return true;
      return [entry.message, entry.service, entry.agentRun ?? "", entry.requestId, entry.traceId]
        .join(" ")
        .toLowerCase()
        .includes(normalizedQuery);
    });
  }, [entries, query, service, level, environment]);

  return (
    <AdminPageBody>
      <AdminHeader title="Logs" subtitle="Search, filter, and inspect platform logs across services.">
        <TimeRangeSelect value={range} onChange={setRange} />
      </AdminHeader>

      {/* Toolbar */}
      <div className="flex flex-wrap items-center gap-2.5">
        <ToolbarSearch value={query} onChange={setQuery} placeholder="Search logs..." label="Search logs" />
        <AdminSelect
          label="Filter by service"
          value={service}
          onChange={(value) => setService(value as ServiceFilter)}
          options={[{ value: "all", label: "All services" }, ...SERVICES.map((item) => ({ value: item, label: item }))]}
        />
        <AdminSelect
          label="Filter by level"
          value={level}
          onChange={(value) => setLevel(value as LevelFilter)}
          options={[{ value: "all", label: "All levels" }, ...LEVELS.map((item) => ({ value: item, label: item }))]}
        />
        <AdminSelect
          label="Filter by environment"
          value={environment}
          onChange={(value) => setEnvironment(value as EnvFilter)}
          options={[
            { value: "all", label: "All environments" },
            { value: "production", label: "production" },
            { value: "staging", label: "staging" },
          ]}
        />
        <div className="ml-auto">
          <LiveToggle enabled={liveTail} onChange={setLiveTail} />
        </div>
      </div>

      {/* Stream */}
      <div className="overflow-hidden rounded-lg border border-[var(--projects-border)] bg-[#141416]">
        <div
          aria-hidden="true"
          className="hidden grid-cols-[92px_58px_84px_minmax(0,1fr)_auto] gap-3 border-b border-[var(--projects-divider)] px-3.5 py-2 md:grid"
        >
          <span className="text-[10.5px] font-medium uppercase tracking-[0.08em] text-[var(--projects-muted)]">Time</span>
          <span className="text-[10.5px] font-medium uppercase tracking-[0.08em] text-[var(--projects-muted)]">Level</span>
          <span className="text-[10.5px] font-medium uppercase tracking-[0.08em] text-[var(--projects-muted)]">Service</span>
          <span className="text-[10.5px] font-medium uppercase tracking-[0.08em] text-[var(--projects-muted)]">Message</span>
          <span className="text-[10.5px] font-medium uppercase tracking-[0.08em] text-[var(--projects-muted)]">Meta</span>
        </div>
        <div className="admin-scrollbar max-h-[640px] overflow-y-auto" aria-label="Log stream">
          {visible.length === 0 ? (
            <p className="m-0 px-4 py-12 text-center text-[13px] text-[var(--projects-muted)]">
              No log entries match the current filters.
            </p>
          ) : (
            visible.map((entry) => (
              <LogRow key={entry.id} entry={entry} onOpen={() => setSelected(entry)} />
            ))
          )}
        </div>
        <div className="flex items-center justify-between border-t border-[var(--projects-divider)] px-3.5 py-2">
          <Mono className="text-[11px] text-[var(--projects-muted)]">
            {visible.length} of {entries.length} entries
          </Mono>
          <Mono className="text-[11px] text-[var(--projects-muted)]">
            retention 7d · environment {environment === "all" ? "any" : environment}
          </Mono>
        </div>
      </div>

      <LogDetail entry={selected} onClose={() => setSelected(null)} />
    </AdminPageBody>
  );
}

function LogRow({ entry, onOpen }: { entry: LogEntry; onOpen: () => void }) {
  const tone = LEVEL_TONE[entry.level];
  return (
    <button
      type="button"
      onClick={onOpen}
      aria-label={`Inspect log: ${entry.message}`}
      className="block w-full border-b border-[var(--projects-divider)] px-3.5 py-1.5 text-left transition-colors last:border-b-0 hover:bg-white/[0.03] aria-selected:bg-white/[0.04]"
    >
      {/* desktop layout */}
      <span className="hidden items-baseline gap-3 md:grid md:grid-cols-[92px_58px_84px_minmax(0,1fr)_auto]">
        <Mono className="text-[11.5px] leading-5 text-[#8a8791]">{entry.timestamp}</Mono>
        <span className={cn("text-[11px] font-semibold leading-5 tracking-wide", tone.text)}>{entry.level}</span>
        <Mono className="truncate text-[11.5px] leading-5 text-[#b3b0ba]">{entry.service}</Mono>
        <span className="min-w-0">
          <Mono className="block truncate text-[12px] leading-5 text-[var(--projects-text)]">{entry.message}</Mono>
          {entry.environment === "staging" && (
            <Mono className="mt-0.5 block text-[10px] leading-3 text-[var(--admin-info)]">staging</Mono>
          )}
        </span>
        <Mono className="text-[11.5px] leading-5 text-[#8a8791]">{entry.meta}</Mono>
      </span>
      {/* compact layout */}
      <span className="block md:hidden">
        <span className="flex items-baseline gap-2">
          <span className={cn("text-[10.5px] font-semibold tracking-wide", tone.text)}>{entry.level}</span>
          <Mono className="truncate text-[11.5px] leading-5 text-[#b3b0ba]">{entry.service}</Mono>
          <Mono className="ml-auto shrink-0 text-[10.5px] leading-5 text-[#8a8791]">{entry.timestamp}</Mono>
        </span>
        <Mono className="mt-0.5 block break-words text-[12px] leading-4 text-[var(--projects-text)]">{entry.message}</Mono>
      </span>
    </button>
  );
}

function LogDetail({ entry, onClose }: { entry: LogEntry | null; onClose: () => void }) {
  return (
    <DetailDrawer
      open={entry !== null}
      onClose={onClose}
      title={
        <>
          <span className="truncate">Log detail</span>
          {entry && <StatusBadge tone="neutral" label={entry.service} className="ml-2" />}
        </>
      }
      subtitle={entry?.message}
    >
      {entry && (
        <div className="flex flex-col gap-4">
          <div className="grid grid-cols-2 gap-x-4">
            <DetailField label="Timestamp">
              <span className="admin-mono flex items-center gap-1">
                {entry.timestamp}
                <CopyButton text={entry.timestamp} />
              </span>
            </DetailField>
            <DetailField label="Level">
              <span className={cn("text-[12px] font-semibold", LEVEL_TONE[entry.level].label)}>{entry.level}</span>
            </DetailField>
            <DetailField label="Service">{entry.service}</DetailField>
            <DetailField label="Environment">{entry.environment}</DetailField>
            <DetailField label="Request ID">
              <span className="admin-mono flex items-center gap-1">
                {entry.requestId}
                <CopyButton text={entry.requestId} />
              </span>
            </DetailField>
            <DetailField label="Trace ID">
              <span className="admin-mono flex items-center gap-1">
                {entry.traceId}
                <CopyButton text={entry.traceId} />
              </span>
            </DetailField>
            <DetailField label="User">{entry.user ?? "—"}</DetailField>
            <DetailField label="Agent Run">{entry.agentRun ?? "—"}</DetailField>
          </div>

          <div>
            <p className="m-0 mb-1.5 text-[11px] font-medium uppercase tracking-[0.06em] text-[var(--projects-muted)]">
              Attributes
            </p>
            <pre className="admin-scrollbar m-0 overflow-x-auto rounded-lg border border-[var(--projects-border)] bg-[#0f0f11] p-3 text-[11.5px] leading-5 text-[#c9c5cd]">
              {JSON.stringify(entry.attributes, null, 2)}
            </pre>
            <div className="mt-2 flex justify-end">
              <span className="inline-flex items-center gap-1.5 text-[11.5px] text-[var(--projects-muted)]">
                Copy JSON
                <CopyButton text={JSON.stringify(entry.attributes, null, 2)} />
              </span>
            </div>
          </div>
        </div>
      )}
    </DetailDrawer>
  );
}
