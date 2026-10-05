"use client";

import { useMemo, useState } from "react";
import { Clock, Cpu, HardDrive, MemoryStick, Network } from "lucide-react";
import {
  useAdminHostMetrics,
  useAdminIncidents,
  useAdminOverview,
} from "@/api/queries";
import { AdminHeader, AdminPageBody } from "../components/admin-panel";
import { AdminSelect } from "../components/admin-select";
import { MetricCard } from "../components/metric-card";
import { StatTile } from "../components/stat-tile";
import {
  LiveIndicator,
  RefreshButton,
  UpdatedLabel,
} from "../components/live-indicator";
import { formatBytes } from "@/lib/format";
import { toIncident } from "../data/admin-adapters";
import { SystemStatus } from "./system-status";
import { ResourceOverview, type OverviewTab } from "./resource-overview";
import { ServiceHealthTable } from "./service-health-table";
import { RecentIncidents } from "./recent-incidents";
import type { MetricPoint } from "../types/telemetry";

const GIB = 1024 ** 3;
const KIB = 1024;

// The host metrics collector retains one hour, so only ranges within that
// window are offered — a wider choice would silently show the same data.
const OVERVIEW_RANGES = [
  { value: "5m", label: "Last 5 minutes", minutes: 5, noun: "5 minutes" },
  { value: "15m", label: "Last 15 minutes", minutes: 15, noun: "15 minutes" },
  { value: "1h", label: "Last 1 hour", minutes: 60, noun: "hour" },
] as const;
type OverviewRange = (typeof OVERVIEW_RANGES)[number]["value"];

type ChartSeriesMap = Record<OverviewTab, MetricPoint[]>;

/**
 * Admin Overview — platform health, host resources, and telemetry. Host
 * CPU/memory/disk/network come from the in-process host metrics endpoint (no
 * ClickHouse); component status, operations, and incidents come from the admin
 * API. Sandboxes/latency/error-rate tiles are intentionally absent because
 * Stealth does not expose those resources.
 */
export function AdminOverview() {
  const [range, setRange] = useState<OverviewRange>("1h");
  const [tab, setTab] = useState<OverviewTab>("cpu");

  const rangeMeta = OVERVIEW_RANGES.find((option) => option.value === range);

  const overview = useAdminOverview();
  const host = useAdminHostMetrics(rangeMeta?.minutes);
  const incidentsQuery = useAdminIncidents(
    { limit: 20 },
    { refetchInterval: 30_000 },
  );

  // The "Live" badge reflects real connectivity: any failing/again-loading
  // query flips it to amber instead of claiming a live connection.
  const connected =
    !overview.isError && !host.isError && !incidentsQuery.isError;

  const components = overview.data?.components ?? [];
  const operations = overview.data?.operations;
  const current = host.data?.current;
  const history = useMemo(() => host.data?.history ?? [], [host.data]);

  const cpuHistory = useMemo(
    () => history.map((sample) => sample.cpu_percent),
    [history],
  );
  const memoryHistory = useMemo(
    () => history.map((sample) => sample.memory_used_bytes / GIB),
    [history],
  );
  const diskHistory = useMemo(
    () => history.map((sample) => sample.disk_used_bytes / GIB),
    [history],
  );
  const networkHistory = useMemo(
    () => history.map((sample) => sample.network_rx_bytes_per_sec / KIB),
    [history],
  );

  const chart = useMemo<ChartSeriesMap>(
    () => ({
      cpu: history.map((sample) => ({
        timestamp: sample.timestamp,
        value: sample.cpu_percent,
      })),
      memory: history.map((sample) => ({
        timestamp: sample.timestamp,
        value: sample.memory_used_bytes / GIB,
      })),
      network: history.map((sample) => ({
        timestamp: sample.timestamp,
        value: sample.network_rx_bytes_per_sec / KIB,
      })),
    }),
    [history],
  );

  const incidents = useMemo(
    () => (incidentsQuery.data?.items ?? []).map(toIncident),
    [incidentsQuery.data],
  );

  const rangeLabel = useMemo(
    () => rangeMeta?.label ?? range,
    [rangeMeta, range],
  );

  // Deltas for the metric-card change indicators, computed from the retained
  // history so the cards read like the redesign ("+4.2% from previous hour").
  // The first sample is a cold-start with no baseline (CPU/network read 0), so
  // rate metrics use the first non-zero sample as their baseline.
  const hasDelta = history.length >= 2 && Boolean(current);
  const firstPositive = (
    pick: (sample: (typeof history)[number]) => number,
  ): (typeof history)[number] | undefined =>
    history.find((sample) => pick(sample) > 0) ?? history[0];
  const signed = (value: number, unit: string) =>
    `${value >= 0 ? "+" : ""}${Math.round(value * 10) / 10}${unit}`;
  const percentDelta = (now: number, before: number) =>
    before !== 0 ? ((now - before) / before) * 100 : 0;
  const cpuBaseline = firstPositive((sample) => sample.cpu_percent);
  const netBaseline = firstPositive(
    (sample) => sample.network_rx_bytes_per_sec,
  );
  const memoryBaseline = history[0];
  const diskBaseline = history[0];
  const cpuDelta =
    hasDelta && current && cpuBaseline
      ? signed(current.cpu_percent - cpuBaseline.cpu_percent, "%")
      : undefined;
  const memoryDelta =
    hasDelta && current && memoryBaseline
      ? signed(
          percentDelta(
            current.memory_used_bytes,
            memoryBaseline.memory_used_bytes,
          ),
          "%",
        )
      : undefined;
  const diskDelta =
    hasDelta && current && diskBaseline
      ? signed(
          percentDelta(current.disk_used_bytes, diskBaseline.disk_used_bytes),
          "%",
        )
      : undefined;
  const networkDelta =
    hasDelta && current && netBaseline
      ? signed(
          (current.network_rx_bytes_per_sec -
            netBaseline.network_rx_bytes_per_sec) /
            KIB,
          " KB/s",
        )
      : undefined;

  return (
    <AdminPageBody>
      <AdminHeader
        title="Admin Overview"
        subtitle="Monitor platform health, host resources, and telemetry."
      >
        <AdminSelect
          label="Time range"
          icon={<Clock size={13} strokeWidth={1.8} aria-hidden="true" />}
          value={range}
          options={OVERVIEW_RANGES}
          onChange={setRange}
        />
        <RefreshButton
          onClick={() => {
            void overview.refetch();
            void host.refetch();
            void incidentsQuery.refetch();
          }}
        />
        <LiveIndicator live={connected} />
      </AdminHeader>

      <SystemStatus components={components} />

      {/* Primary host resource metrics */}
      {/* 2×2 on phones so all four host metrics read at a glance; the tall
          single-column stack pushed Platform and the charts far below the
          fold. Desktop stays one row of four. */}
      <div className="grid grid-cols-2 gap-2.5 sm:gap-3 xl:grid-cols-4">
        <MetricCard
          icon={Cpu}
          label="CPU Usage"
          value={current ? `${Math.round(current.cpu_percent)}%` : "—"}
          change={cpuDelta}
          changeLabel={`from previous ${rangeMeta?.noun ?? "hour"}`}
          changeTone="danger"
          history={cpuHistory}
          sparkTone="accent"
        />
        <MetricCard
          icon={MemoryStick}
          label="Memory"
          value={current ? formatBytes(current.memory_used_bytes) : "—"}
          change={memoryDelta}
          changeLabel={
            current
              ? `of ${formatBytes(current.memory_total_bytes)}`
              : undefined
          }
          changeTone="success"
          history={memoryHistory}
          sparkTone="info"
        />
        <MetricCard
          icon={HardDrive}
          label="Storage"
          value={current ? formatBytes(current.disk_used_bytes) : "—"}
          change={diskDelta}
          changeLabel={
            current ? `of ${formatBytes(current.disk_total_bytes)}` : undefined
          }
          changeTone="neutral"
          history={diskHistory}
          sparkTone="neutral"
        />
        <MetricCard
          icon={Network}
          label="Network"
          value={
            current
              ? `${(current.network_rx_bytes_per_sec / KIB).toFixed(1)} KB/s`
              : "—"
          }
          hint={
            current
              ? `↑ ${(current.network_tx_bytes_per_sec / KIB).toFixed(1)} KB/s out`
              : undefined
          }
          change={networkDelta}
          changeLabel={`ingress, from previous ${rangeMeta?.noun ?? "hour"}`}
          changeTone="neutral"
          history={networkHistory}
          sparkTone="warning"
        />
      </div>

      {/* Platform metrics */}
      <div>
        <div className="mb-2 flex items-center justify-between px-1">
          <h2 className="m-0 text-[12px] font-semibold uppercase tracking-[0.08em] text-[var(--projects-muted)]">
            Platform
          </h2>
          <UpdatedLabel
            resetKey={overview.data?.checked_at}
            className="admin-mono text-[11px] text-[var(--projects-muted)]"
          />
        </div>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <StatTile
            label="Active deployments"
            value={String(operations?.active_deployments ?? 0)}
          />
          <StatTile
            label="Running jobs"
            value={String(operations?.running_jobs ?? 0)}
            tone="success"
          />
          <StatTile
            label="Failed jobs"
            value={String(operations?.failed_jobs ?? 0)}
            tone={
              operations && operations.failed_jobs > 0 ? "danger" : "neutral"
            }
          />
          <StatTile
            label="Queued jobs"
            value={String(operations?.queued_jobs ?? 0)}
            tone="warning"
          />
        </div>
      </div>

      <div className="grid gap-4 lg:grid-cols-3">
        <ResourceOverview
          series={chart}
          tab={tab}
          onTabChange={setTab}
          rangeLabel={rangeLabel}
          className="lg:col-span-2"
        />
        <RecentIncidents incidents={incidents} />
      </div>

      <ServiceHealthTable components={components} />
    </AdminPageBody>
  );
}
