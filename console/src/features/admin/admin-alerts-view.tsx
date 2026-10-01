"use client";

import { useState } from "react";
import { BellRing, Plus, Trash2 } from "lucide-react";
import { useDeleteAdminAlert } from "@/api/mutations";
import {
  useAdminAlertEvents,
  useAdminAlerts,
  useAdminMonitors,
} from "@/api/queries";
import { ErrorState, errorMessage } from "@/components/feedback/error-state";
import { LoadingState } from "@/components/feedback/loading-state";
import { PageHeader } from "@/components/page-header";
import { Badge, StatusBadge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogTrigger } from "@/components/ui/dialog";
import { formatDate } from "@/lib/format";
import { CreateAlertDialog } from "./admin-alert-dialog";
import { AdminShell } from "./admin-shell";
import { AdminTimeRange, useAdminTimeRange } from "./admin-time-range";

export function AdminAlertsView() {
  const timeRange = useAdminTimeRange();
  const alerts = useAdminAlerts(
    { limit: 100 },
    { refetchInterval: timeRange.refreshInterval },
  );
  const history = useAdminAlertEvents(
    {
      from: timeRange.query.from,
      to: timeRange.query.to,
      limit: 50,
    },
    { refetchInterval: timeRange.refreshInterval },
  );
  const remove = useDeleteAdminAlert();
  const [dialogOpen, setDialogOpen] = useState(false);
  const monitors = useAdminMonitors({ limit: 100 });
  const historyItems =
    history.data?.pages.flatMap((page) => page?.items ?? []) ?? [];

  return (
    <AdminShell>
      <PageHeader
        eyebrow="Admin / Alerts"
        title="Alert rules"
        description="Durable rules evaluated by trusted workers. Rules are bounded definitions, never arbitrary ClickHouse queries."
        actions={
          <div className="flex flex-wrap items-center gap-3">
            <AdminTimeRange
              rangeKey={timeRange.rangeKey}
              refreshKey={timeRange.refreshKey}
              customRange={timeRange.customRange}
              onRangeChange={timeRange.setRange}
              onRefreshChange={timeRange.setRefresh}
              onCustomRangeChange={timeRange.setCustomRange}
            />
            <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
              <DialogTrigger asChild>
                <Button size="sm">
                  <Plus className="size-3.5" aria-hidden="true" /> Add rule
                </Button>
              </DialogTrigger>
              <CreateAlertDialog
                monitors={monitors.data?.items ?? []}
                onCreated={() => setDialogOpen(false)}
              />
            </Dialog>
          </div>
        }
      />
      {alerts.isPending ? <LoadingState rows={5} /> : null}
      {alerts.error ? (
        <ErrorState
          title="Could not load alert rules"
          error={alerts.error}
          retry={() => alerts.refetch()}
        />
      ) : null}
      {alerts.data && !alerts.data.items.length ? (
        <div className="rounded-xl border border-dashed border-graphite bg-carbon/50 p-10 text-center">
          <BellRing className="mx-auto size-5 text-fog" aria-hidden="true" />
          <h2 className="mt-4 text-sm text-paper">No alert rules configured</h2>
          <p className="mx-auto mt-2 max-w-md text-sm leading-6 text-fog">
            Start with a monitor failure rule. Its state changes are persisted
            with the monitor check, so a worker restart cannot erase an alert.
          </p>
        </div>
      ) : null}
      {alerts.data?.items.length ? (
        <div className="overflow-hidden rounded-xl border border-graphite bg-carbon">
          <div className="overflow-x-auto">
            <table className="w-full min-w-[880px] text-left text-sm">
              <thead className="border-b border-graphite bg-white/[0.02] text-xs uppercase tracking-[0.1em] text-fog">
                <tr>
                  <th className="px-4 py-3 font-medium">Rule</th>
                  <th className="px-4 py-3 font-medium">Severity</th>
                  <th className="px-4 py-3 font-medium">State</th>
                  <th className="px-4 py-3 font-medium">Last evaluated</th>
                  <th className="px-4 py-3 font-medium">Duration</th>
                  <th className="px-4 py-3 font-medium" aria-label="Actions" />
                </tr>
              </thead>
              <tbody className="divide-y divide-graphite">
                {alerts.data.items.map((rule) => (
                  <tr
                    key={rule.id}
                    className="align-top hover:bg-white/[0.025]"
                  >
                    <td className="px-4 py-4">
                      <p className="text-mist">{rule.name}</p>
                      <p className="mt-1 font-mono text-[11px] text-fog">
                        {rule.kind}
                      </p>
                    </td>
                    <td className="px-4 py-4">
                      <Badge
                        variant={
                          rule.severity === "critical"
                            ? "error"
                            : rule.severity === "warning"
                              ? "warning"
                              : "neutral"
                        }
                      >
                        {rule.severity}
                      </Badge>
                    </td>
                    <td className="px-4 py-4">
                      <StatusBadge
                        status={rule.enabled ? rule.state : "muted"}
                      />
                    </td>
                    <td className="px-4 py-4 text-xs text-fog">
                      {formatDate(rule.last_evaluated_at)}
                    </td>
                    <td className="px-4 py-4 font-mono text-xs text-mist">
                      {rule.for_seconds ? `${rule.for_seconds}s` : "Immediate"}
                    </td>
                    <td className="px-4 py-4 text-right">
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon"
                        aria-label={`Delete ${rule.name}`}
                        onClick={() => {
                          if (
                            window.confirm(
                              `Delete the alert rule “${rule.name}”?`,
                            )
                          )
                            remove.mutate(rule.id);
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
          {remove.error ? (
            <p
              className="border-t border-graphite p-4 text-sm text-coral-red"
              role="alert"
            >
              {errorMessage(remove.error)}
            </p>
          ) : null}
        </div>
      ) : null}
      <section className="overflow-hidden rounded-xl border border-graphite bg-carbon">
        <div className="border-b border-graphite px-4 py-4">
          <h2 className="text-sm font-medium text-paper">
            Recent alert history
          </h2>
          <p className="mt-1 text-sm text-fog">
            Firing and resolved events remain available after their rule is
            deleted.
          </p>
        </div>
        {history.isPending ? <LoadingState rows={3} /> : null}
        {history.error ? (
          <div className="p-4" role="alert">
            <ErrorState
              title="Could not load alert history"
              error={history.error}
              retry={() => history.refetch()}
            />
          </div>
        ) : null}
        {history.data && !historyItems.length ? (
          <p className="p-6 text-sm text-fog">No alert history recorded yet.</p>
        ) : null}
        {historyItems.length ? (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[760px] text-left text-sm">
              <thead className="border-b border-graphite bg-white/[0.02] text-xs uppercase tracking-[0.1em] text-fog">
                <tr>
                  <th className="px-4 py-3 font-medium">Rule</th>
                  <th className="px-4 py-3 font-medium">Severity</th>
                  <th className="px-4 py-3 font-medium">State</th>
                  <th className="px-4 py-3 font-medium">Source</th>
                  <th className="px-4 py-3 font-medium">Occurred</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-graphite">
                {historyItems.map((event) => (
                  <tr
                    key={event.id}
                    className="align-top hover:bg-white/[0.025]"
                  >
                    <td className="max-w-[280px] px-4 py-4">
                      <p className="truncate text-mist" title={event.rule_name}>
                        {event.rule_name}
                      </p>
                      <p className="mt-1 font-mono text-[11px] text-fog">
                        {event.rule_kind}
                      </p>
                    </td>
                    <td className="px-4 py-4">
                      <Badge
                        variant={
                          event.severity === "critical"
                            ? "error"
                            : event.severity === "warning"
                              ? "warning"
                              : "neutral"
                        }
                      >
                        {event.severity}
                      </Badge>
                    </td>
                    <td className="px-4 py-4">
                      <StatusBadge status={event.state} />
                    </td>
                    <td className="px-4 py-4 text-xs text-fog">
                      {event.source_rule_exists
                        ? "Active rule"
                        : "Deleted rule"}
                    </td>
                    <td className="px-4 py-4 text-xs text-fog">
                      {formatDate(event.occurred_at)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : null}
        {history.hasNextPage ? (
          <div className="border-t border-graphite p-4">
            <Button
              type="button"
              variant="secondary"
              size="sm"
              onClick={() => history.fetchNextPage()}
              disabled={history.isFetchingNextPage}
            >
              {history.isFetchingNextPage ? "Loading older…" : "Load older"}
            </Button>
          </div>
        ) : null}
      </section>
    </AdminShell>
  );
}
