"use client";

import { useEffect, useMemo, useState } from "react";
import { Cpu, HardDrive, MemoryStick, Network } from "lucide-react";
import { PRIMARY_METRICS, PLATFORM_STATS, overviewSeries } from "../data/admin-mock-data";
import { TIME_RANGES, type TimeRange } from "../types/telemetry";
import { AdminHeader, AdminPageBody } from "../components/admin-panel";
import { MetricCard } from "../components/metric-card";
import { StatTile } from "../components/stat-tile";
import { LiveIndicator, RefreshButton, UpdatedLabel } from "../components/live-indicator";
import { TimeRangeSelect } from "../components/time-range-select";
import { nudge, pushHistory, useLiveTick } from "../hooks/use-live-updates";
import { SystemStatus } from "./system-status";
import { ResourceOverview, type OverviewTab } from "./resource-overview";
import { ServiceHealthTable } from "./service-health-table";
import { RecentIncidents } from "./recent-incidents";
import { RecentRuns } from "./recent-runs";

interface LiveValues {
  cpu: number;
  memoryGb: number;
  netIn: number;
  netOut: number;
  running: number;
  queue: number;
  latency: number;
  errorRate: number;
}

const INITIAL_LIVE: LiveValues = {
  cpu: 34,
  memoryGb: 9.2,
  netIn: 24,
  netOut: 8,
  running: 8,
  queue: 14,
  latency: 182,
  errorRate: 0.42,
};

type ChartSeriesMap = Record<OverviewTab, { timestamp: string; value: number }[]>;

/**
 * Admin Overview — platform health, agent workloads, and telemetry.
 * All numbers are mock; the live feel comes from a 4s local interval
 * (cleaned up on unmount) that nudges values after hydration.
 */
export function AdminOverview() {
  const [range, setRange] = useState<TimeRange>("1h");
  // Bumped on refresh; chart regeneration stays deterministic per (range, seed).
  const [seed, setSeed] = useState(0);
  const [tab, setTab] = useState<OverviewTab>("cpu");
  const [live, setLive] = useState<LiveValues>(INITIAL_LIVE);
  const [chart, setChart] = useState<ChartSeriesMap>(() => overviewSeries("1h"));
  const [history, setHistory] = useState<Record<string, number[]>>(() =>
    Object.fromEntries(PRIMARY_METRICS.map((metric) => [metric.id, metric.history])),
  );
  const tick = useLiveTick(4000);

  // Live nudge pass — never runs on the server or the first client render.
  useEffect(() => {
    if (tick === 0) return;
    setLive((prev) => {
      const next: LiveValues = {
        cpu: nudge(prev.cpu, 1.8, 22, 58),
        memoryGb: nudge(prev.memoryGb, 0.16, 8.4, 10.8),
        netIn: nudge(prev.netIn, 3, 12, 40),
        netOut: nudge(prev.netOut, 1.2, 4, 14),
        running:
          Math.random() > 0.6
            ? Math.min(10, Math.max(6, prev.running + (Math.random() > 0.5 ? 1 : -1)))
            : prev.running,
        queue: Math.round(nudge(prev.queue, 2, 10, 18)),
        latency: Math.round(nudge(prev.latency, 9, 150, 240)),
        errorRate: Math.round(nudge(prev.errorRate, 0.03, 0.28, 0.66) * 100) / 100,
      };
      // Keep sparks and the active chart in sync with the same sample.
      setHistory((prevHistory) => ({
        ...prevHistory,
        cpu: pushHistory(prevHistory.cpu, next.cpu),
        memory: pushHistory(prevHistory.memory, next.memoryGb),
        network: pushHistory(prevHistory.network, next.netIn),
      }));
      setChart((prevChart) => ({
        ...prevChart,
        cpu: replaceLast(prevChart.cpu, next.cpu),
        memory: replaceLast(prevChart.memory, next.memoryGb),
        network: replaceLast(prevChart.network, next.netIn),
      }));
      return next;
    });
  }, [tick]);

  // Range/refresh changes regenerate the chart deterministically.
  useEffect(() => {
    setChart(overviewSeries(range));
  }, [range, seed]);

  const rangeLabel = useMemo(
    () => TIME_RANGES.find((option) => option.value === range)?.label ?? range,
    [range],
  );

  return (
    <AdminPageBody>
      <AdminHeader title="Admin Overview" subtitle="Monitor platform health, agent workloads, and telemetry.">
        <TimeRangeSelect value={range} onChange={setRange} />
        <RefreshButton onClick={() => setSeed((value) => value + 1)} />
        <LiveIndicator />
      </AdminHeader>

      <SystemStatus resetKey={`${range}-${seed}`} />

      {/* Primary resource metrics */}
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <MetricCard
          icon={Cpu}
          label="CPU Usage"
          value={`${Math.round(live.cpu)}%`}
          change={PRIMARY_METRICS[0].change}
          changeLabel={PRIMARY_METRICS[0].changeLabel}
          changeTone="danger"
          history={history.cpu}
          sparkTone="accent"
        />
        <MetricCard
          icon={MemoryStick}
          label="Memory"
          value={`${live.memoryGb.toFixed(1)} GB`}
          change={PRIMARY_METRICS[1].change}
          changeLabel="of 16 GB"
          changeTone="success"
          history={history.memory}
          sparkTone="info"
        />
        <MetricCard
          icon={HardDrive}
          label="Storage"
          value="124 GB"
          change={PRIMARY_METRICS[2].change}
          changeLabel="of 250 GB"
          changeTone="neutral"
          history={history.storage}
          sparkTone="neutral"
        />
        <MetricCard
          icon={Network}
          label="Network"
          value={`${Math.round(live.netIn)} MB/s`}
          hint={`↑ ${Math.round(live.netOut)} MB/s out`}
          change={PRIMARY_METRICS[3].change}
          changeLabel="ingress, from previous hour"
          changeTone="neutral"
          history={history.network}
          sparkTone="warning"
        />
      </div>

      {/* Platform metrics */}
      <div>
        <div className="mb-2 flex items-center justify-between px-1">
          <h2 className="m-0 text-[12px] font-semibold uppercase tracking-[0.08em] text-[var(--projects-muted)]">
            Platform
          </h2>
          <UpdatedLabel className="text-[11px] text-[var(--projects-muted)]" />
        </div>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          {PLATFORM_STATS.map((stat) => (
            <StatTile
              key={stat.id}
              label={stat.label}
              value={liveStatValue(stat.id, stat.value, live)}
              hint={stat.hint}
              tone={
                stat.id === "running"
                  ? "success"
                  : stat.id === "failed"
                    ? "danger"
                    : stat.id === "queue" || stat.id === "error-rate"
                      ? "warning"
                      : "neutral"
              }
            />
          ))}
        </div>
      </div>

      <div className="grid gap-4 lg:grid-cols-3">
        <ResourceOverview series={chart} tab={tab} onTabChange={setTab} rangeLabel={rangeLabel} className="lg:col-span-2" />
        <RecentIncidents />
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <ServiceHealthTable />
        <RecentRuns />
      </div>
    </AdminPageBody>
  );
}

/** Replace the newest chart point with a fresh live sample. */
function replaceLast(points: ChartSeriesMap[OverviewTab], value: number): ChartSeriesMap[OverviewTab] {
  if (points.length === 0) return points;
  return points.map((point, index) => (index === points.length - 1 ? { ...point, value } : point));
}

/** Swap the live-updatable stat values into their display strings. */
function liveStatValue(id: string, base: string, live: LiveValues): string {
  switch (id) {
    case "running":
      return String(live.running);
    case "queue":
      return String(live.queue);
    case "latency":
      return `${live.latency} ms`;
    case "error-rate":
      return `${live.errorRate.toFixed(2)}%`;
    default:
      return base;
  }
}
