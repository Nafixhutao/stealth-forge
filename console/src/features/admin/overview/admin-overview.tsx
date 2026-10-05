"use client";

import { useMemo, useState } from "react";
import { Cpu, HardDrive, MemoryStick, Network } from "lucide-react";
import {
  useAdminHostMetrics,
  useAdminIncidents,
  useAdminOverview,
} from "@/api/queries";
import { TIME_RANGES, type TimeRange } from "../types/telemetry";
import { AdminHeader, AdminPageBody } from "../components/admin-panel";
import { MetricCard } from "../components/metric-card";
import { StatTile } from "../components/stat-tile";
import { LiveIndicator, RefreshButton } from "../components/live-indicator";
import { TimeRangeSelect } from "../components/time-range-select";
import { formatBytes } from "@/lib/format";
import { toIncident } from "../data/admin-adapters";
import { SystemStatus } from "./system-status";
import { ResourceOverview, type OverviewTab } from "./resource-overview";
import { ServiceHealthTable } from "./service-health-table";
import { RecentIncidents } from "./recent-incidents";
import type { MetricPoint } from "../types/telemetry";

const GIB = 1024 ** 3;
const MIB = 1024 ** 2;

type ChartSeriesMap = Record<OverviewTab, MetricPoint[]>;

/**
 * Admin Overview — platform health, host resources, and telemetry. Host
 * CPU/memory/disk/network come from the in-process host metrics endpoint (no
 * ClickHouse); component status, operations, and incidents come from the admin
 * API. Sandboxes/latency/error-rate tiles are intentionally absent because
 * Stealth does not expose those resources.
 */
export function AdminOverview() {
  const [range, setRange] = useState<TimeRange>("1h");
  const [tab, setTab] = useState<OverviewTab>("cpu");

  const overview = useAdminOverview();
  const host = useAdminHostMetrics();
  const incidentsQuery = useAdminIncidents(
    { limit: 20 },
    { refetchInterval: 30_000 },
  );

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
  const networkHistory = useMemo(
    () => history.map((sample) => sample.network_rx_bytes_per_sec / MIB),
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
        value: sample.network_rx_bytes_per_sec / MIB,
      })),
    }),
    [history],
  );

  const incidents = useMemo(
    () => (incidentsQuery.data?.items ?? []).map(toIncident),
    [incidentsQuery.data],
  );

  const rangeLabel = useMemo(
    () => TIME_RANGES.find((option) => option.value === range)?.label ?? range,
    [range],
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
            MIB,
          " MB/s",
        )
      : undefined;

  return (
    <AdminPageBody>
      <AdminHeader
        title="Admin Overview"
        subtitle="Monitor platform health, host resources, and telemetry."
      >
        <TimeRangeSelect value={range} onChange={setRange} />
        <RefreshButton
          onClick={() => {
            void overview.refetch();
            void host.refetch();
            void incidentsQuery.refetch();
          }}
        />
        <LiveIndicator />
      </AdminHeader>

      <SystemStatus components={components} />

      {/* Primary host resource metrics */}
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <MetricCard
          icon={Cpu}
          label="CPU Usage"
          value={current ? `${Math.round(current.cpu_percent)}%` : "—"}
          change={cpuDelta}
          changeLabel="from previous hour"
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
          history={[]}
          sparkTone="neutral"
        />
        <MetricCard
          icon={Network}
          label="Network"
          value={
            current
              ? `${(current.network_rx_bytes_per_sec / MIB).toFixed(1)} MB/s`
              : "—"
          }
          hint={
            current
              ? `↑ ${(current.network_tx_bytes_per_sec / MIB).toFixed(1)} MB/s out`
              : undefined
          }
          change={networkDelta}
          changeLabel="ingress, from previous hour"
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
