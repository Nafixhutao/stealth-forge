"use client";

import { useMemo, useState } from "react";
import { Activity, Plus, Trash2 } from "lucide-react";
import { useCreateAdminMonitor, useDeleteAdminMonitor } from "@/api/mutations";
import { useAdminMonitor, useAdminMonitors } from "@/api/queries";
import { ErrorState, errorMessage } from "@/components/feedback/error-state";
import { LoadingState } from "@/components/feedback/loading-state";
import { PageHeader } from "@/components/page-header";
import { Badge, StatusBadge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { formatDate, formatDuration } from "@/lib/format";
import { AdminShell } from "./admin-shell";
import { AdminEmptyState } from "./admin-empty-state";
import { AdminTimeRange, useAdminTimeRange } from "./admin-time-range";
import { CreateMonitorDialog } from "./admin-monitor-dialog";
import { MonitorDetails, MonitorSummary } from "./admin-monitor-details";

export function AdminMonitorsView() {
  const timeRange = useAdminTimeRange();
  const monitors = useAdminMonitors(
    { limit: 100 },
    { refetchInterval: timeRange.refreshInterval },
  );
  const [selectedId, setSelectedId] = useState<string>();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [heartbeatToken, setHeartbeatToken] = useState<string>();
  const [heartbeatEndpoint, setHeartbeatEndpoint] = useState<string>();
  const create = useCreateAdminMonitor();
  const remove = useDeleteAdminMonitor();
  const selected = useAdminMonitor(selectedId, {
    refetchInterval: timeRange.refreshInterval,
  });

  const sortedMonitors = useMemo(
    () => monitors.data?.items ?? [],
    [monitors.data?.items],
  );

  return (
    <AdminShell>
      <PageHeader
        eyebrow="Admin / Monitoring"
        title="Monitors"
        description="Persistent checks run by the trusted Stealth worker. Secrets stay encrypted and are never returned after creation."
        actions={
          <div className="flex flex-wrap items-center justify-end gap-3">
            <AdminTimeRange
              rangeKey={timeRange.rangeKey}
              refreshKey={timeRange.refreshKey}
              customRange={timeRange.customRange}
              onRangeChange={timeRange.setRange}
              onRefreshChange={timeRange.setRefresh}
              onCustomRangeChange={timeRange.setCustomRange}
            />
            <Dialog
              open={dialogOpen}
              onOpenChange={(open) => {
                setDialogOpen(open);
                if (!open) {
                  create.reset();
                  setHeartbeatToken(undefined);
                  setHeartbeatEndpoint(undefined);
                }
              }}
            >
              <DialogTrigger asChild>
                <Button size="sm">
                  <Plus className="size-3.5" aria-hidden="true" /> Add monitor
                </Button>
              </DialogTrigger>
              <CreateMonitorDialog
                mutation={create}
                heartbeatToken={heartbeatToken}
                heartbeatEndpoint={heartbeatEndpoint}
                onHeartbeatToken={(token, endpoint) => {
                  setHeartbeatToken(token);
                  setHeartbeatEndpoint(endpoint);
                }}
                onCreated={() => {
                  setDialogOpen(false);
                  setHeartbeatToken(undefined);
                  setHeartbeatEndpoint(undefined);
                }}
              />
            </Dialog>
          </div>
        }
      />

      {monitors.isPending ? <LoadingState rows={5} /> : null}
      {monitors.error ? (
        <ErrorState
          title="Could not load monitors"
          error={monitors.error}
          retry={() => monitors.refetch()}
        />
      ) : null}
      {monitors.data && !sortedMonitors.length ? (
        <AdminEmptyState
          icon={Activity}
          title="No monitors configured"
          description="Create an HTTP, TCP, DNS, TLS, or heartbeat check. Results will appear here after the worker completes its first run."
          descriptionClassName="mx-auto max-w-md leading-6"
          actionLabel="Add monitor"
          onAction={() => setDialogOpen(true)}
        />
      ) : null}
      {sortedMonitors.length ? (
        <div className="space-y-4">
          <div className="grid gap-3 sm:grid-cols-3">
            <MonitorSummary
              label="Total"
              value={sortedMonitors.length}
              tone="neutral"
            />
            <MonitorSummary
              label="Healthy"
              value={
                sortedMonitors.filter((item) => item.status === "healthy")
                  .length
              }
              tone="success"
            />
            <MonitorSummary
              label="Needs attention"
              value={
                sortedMonitors.filter((item) =>
                  ["failing", "degraded"].includes(item.status),
                ).length
              }
              tone="error"
            />
          </div>
          <div className="overflow-hidden rounded-xl border border-graphite bg-carbon">
            <div className="overflow-x-auto">
              <table className="w-full min-w-[880px] text-left text-sm">
                <thead className="border-b border-graphite bg-white/[0.02] text-xs uppercase tracking-[0.1em] text-fog">
                  <tr>
                    <th className="px-4 py-3 font-medium">Monitor</th>
                    <th className="px-4 py-3 font-medium">Type</th>
                    <th className="px-4 py-3 font-medium">Status</th>
                    <th className="px-4 py-3 font-medium">Last check</th>
                    <th className="px-4 py-3 font-medium">Next check</th>
                    <th
                      className="px-4 py-3 font-medium"
                      aria-label="Actions"
                    />
                  </tr>
                </thead>
                <tbody className="divide-y divide-graphite">
                  {sortedMonitors.map((monitor) => (
                    <tr
                      key={monitor.id}
                      className="cursor-pointer align-top transition-colors duration-150 hover:bg-white/[0.025]"
                      onClick={() => setSelectedId(monitor.id)}
                    >
                      <td className="px-4 py-4">
                        <p className="text-mist">{monitor.name}</p>
                        <p className="mt-1 max-w-[360px] truncate font-mono text-[11px] text-fog">
                          {monitor.target}
                        </p>
                      </td>
                      <td className="px-4 py-4">
                        <Badge variant="neutral">{monitor.kind}</Badge>
                      </td>
                      <td className="px-4 py-4">
                        <StatusBadge
                          status={monitor.enabled ? monitor.status : "paused"}
                        />
                      </td>
                      <td className="px-4 py-4 text-xs text-fog">
                        <p>{formatDate(monitor.last_checked_at)}</p>
                        {monitor.last_latency_ms !== null &&
                        monitor.last_latency_ms !== undefined ? (
                          <p className="mt-1 font-mono text-[11px] text-mist">
                            {formatDuration(monitor.last_latency_ms)}
                          </p>
                        ) : null}
                      </td>
                      <td className="px-4 py-4 text-xs text-fog">
                        {monitor.enabled
                          ? formatDate(monitor.next_check_at)
                          : "Paused"}
                      </td>
                      <td className="px-4 py-4 text-right">
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          aria-label={`Delete ${monitor.name}`}
                          onClick={(event) => {
                            event.stopPropagation();
                            if (
                              window.confirm(
                                `Delete the monitor “${monitor.name}” and its check history?`,
                              )
                            ) {
                              remove.mutate(monitor.id);
                            }
                          }}
                          disabled={remove.isPending}
                        >
                          <Trash2
                            className="size-4 text-fog"
                            aria-hidden="true"
                          />
                        </Button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
          {remove.error ? (
            <p className="text-sm text-coral-red" role="alert">
              {errorMessage(remove.error)}
            </p>
          ) : null}
        </div>
      ) : null}

      <Dialog
        open={Boolean(selectedId)}
        onOpenChange={(open) => {
          if (!open) setSelectedId(undefined);
        }}
      >
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>
              {selected.data?.monitor.name ?? "Monitor details"}
            </DialogTitle>
            <DialogDescription>
              Recent worker checks and the safe monitor projection. Secret
              configuration is intentionally unavailable.
            </DialogDescription>
          </DialogHeader>
          {selected.isPending ? <LoadingState rows={3} /> : null}
          {selected.error ? (
            <ErrorState
              error={selected.error}
              retry={() => selected.refetch()}
            />
          ) : null}
          {selected.data ? <MonitorDetails data={selected.data} /> : null}
          <DialogFooter>
            <DialogClose asChild>
              <Button variant="secondary">Close</Button>
            </DialogClose>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </AdminShell>
  );
}
