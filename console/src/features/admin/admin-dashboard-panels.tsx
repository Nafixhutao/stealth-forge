"use client";

import type { ReactNode } from "react";
import { BarChart3 } from "lucide-react";
import { useAdminLogs, useAdminMetrics, useAdminMonitors } from "@/api/queries";
import { LoadingState } from "@/components/feedback/loading-state";
import { StatusBadge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { formatDate } from "@/lib/format";
import { AdminMetricChart } from "./admin-metric-chart";
import type { Panel } from "./admin-dashboard-model";

export function DashboardPanel({
  panel,
  query,
  refreshInterval,
}: {
  panel: Panel;
  query: { from: string; to: string };
  refreshInterval: number | false;
}) {
  if (panel.type === "monitor_status")
    return <MonitorStatusPanel refreshInterval={refreshInterval} />;
  if (panel.type === "logs")
    return (
      <LogPanel panel={panel} query={query} refreshInterval={refreshInterval} />
    );
  return (
    <MetricPanel
      panel={panel}
      query={query}
      refreshInterval={refreshInterval}
    />
  );
}

function MetricPanel({
  panel,
  query,
  refreshInterval,
}: {
  panel: Panel;
  query: { from: string; to: string };
  refreshInterval: number | false;
}) {
  const metrics = useAdminMetrics(
    { ...query, name: panel.metric, service: panel.service, limit: 1000 },
    { refetchInterval: refreshInterval },
  );
  return (
    <PanelFrame title={panel.title ?? panel.metric ?? "Metrics"}>
      {metrics.isPending ? <LoadingState rows={2} /> : null}
      {metrics.error ? (
        <TelemetryPanelError
          message="Metric data unavailable."
          retry={() => metrics.refetch()}
        />
      ) : null}
      {metrics.data?.items.length ? (
        <AdminMetricChart items={metrics.data.items} />
      ) : null}
      {metrics.data && !metrics.data.items.length ? (
        <p className="py-8 text-center text-sm text-fog">
          No metric points in this window.
        </p>
      ) : null}
    </PanelFrame>
  );
}

function LogPanel({
  panel,
  query,
  refreshInterval,
}: {
  panel: Panel;
  query: { from: string; to: string };
  refreshInterval: number | false;
}) {
  const logs = useAdminLogs(
    {
      ...query,
      service: panel.service,
      level: panel.level,
      query: panel.query,
      limit: 20,
    },
    { refetchInterval: refreshInterval },
  );
  return (
    <PanelFrame title={panel.title ?? "Recent logs"}>
      {logs.isPending ? <LoadingState rows={2} /> : null}
      {logs.error ? (
        <TelemetryPanelError
          message="Log data unavailable."
          retry={() => logs.refetch()}
        />
      ) : null}
      {logs.data?.items.length ? (
        <div className="divide-y divide-graphite">
          {logs.data.items.slice(0, 8).map((item, index) => (
            <div key={`${item.timestamp}-${index}`} className="py-2 first:pt-0">
              <p className="truncate text-xs text-mist">{item.message}</p>
              <p className="mt-1 font-mono text-[10px] text-fog">
                {formatDate(item.timestamp)} · {item.service}
              </p>
            </div>
          ))}
        </div>
      ) : null}
      {logs.data && !logs.data.items.length ? (
        <p className="py-8 text-center text-sm text-fog">
          No logs in this window.
        </p>
      ) : null}
    </PanelFrame>
  );
}

function MonitorStatusPanel({
  refreshInterval,
}: {
  refreshInterval: number | false;
}) {
  const monitors = useAdminMonitors(
    { limit: 100 },
    { refetchInterval: refreshInterval },
  );
  return (
    <PanelFrame title="Monitor status">
      {monitors.isPending ? <LoadingState rows={2} /> : null}
      {monitors.error ? (
        <TelemetryPanelError
          message="Monitor state unavailable."
          retry={() => monitors.refetch()}
        />
      ) : null}
      {monitors.data?.items.length ? (
        <div className="space-y-2">
          {monitors.data.items.slice(0, 8).map((monitor) => (
            <div
              key={monitor.id}
              className="flex items-center justify-between gap-3"
            >
              <span className="truncate text-sm text-mist">{monitor.name}</span>
              <StatusBadge
                status={monitor.enabled ? monitor.status : "paused"}
              />
            </div>
          ))}
        </div>
      ) : null}
      {monitors.data && !monitors.data.items.length ? (
        <p className="py-8 text-center text-sm text-fog">
          No monitors configured.
        </p>
      ) : null}
    </PanelFrame>
  );
}

export function TelemetryPanelError({
  message,
  retry,
}: {
  message: string;
  retry: () => void;
}) {
  return (
    <div
      className="flex flex-wrap items-center justify-between gap-3 rounded-md border border-coral-red/20 bg-coral-red/[0.04] px-3 py-2.5"
      role="alert"
    >
      <p className="text-sm text-mist">{message}</p>
      <Button
        type="button"
        size="sm"
        variant="outline"
        onClick={() => void retry()}
      >
        Retry
      </Button>
    </div>
  );
}

function PanelFrame({
  title,
  children,
}: {
  title: string;
  children: ReactNode;
}) {
  return (
    <div className="min-w-0 rounded-xl border border-graphite bg-carbon p-5">
      <div className="mb-4 flex items-center gap-2">
        <BarChart3 className="size-4 text-fog" aria-hidden="true" />
        <h3 className="text-sm text-paper">{title}</h3>
      </div>
      {children}
    </div>
  );
}
