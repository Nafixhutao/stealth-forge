"use client";

import { useMemo, useState } from "react";
import { LayoutDashboard, Plus, Trash2 } from "lucide-react";
import {
  useDeleteAdminDashboard,
  useUpdateAdminDashboard,
} from "@/api/mutations";
import { useAdminDashboard, useAdminDashboards } from "@/api/queries";
import { ErrorState, errorMessage } from "@/components/feedback/error-state";
import { LoadingState } from "@/components/feedback/loading-state";
import { PageHeader } from "@/components/page-header";
import { Button } from "@/components/ui/button";
import { Dialog, DialogTrigger } from "@/components/ui/dialog";
import { formatDate } from "@/lib/format";
import {
  AddPanelDialog,
  CreateDashboardDialog,
} from "./admin-dashboard-dialogs";
import type { Panel } from "./admin-dashboard-model";
import { DashboardPanel } from "./admin-dashboard-panels";
import { AdminShell } from "./admin-shell";
import { AdminEmptyState } from "./admin-empty-state";
import { AdminTimeRange, useAdminTimeRange } from "./admin-time-range";

export { TelemetryPanelError } from "./admin-dashboard-panels";

export function AdminDashboardsView() {
  const timeRange = useAdminTimeRange();
  const dashboards = useAdminDashboards({ limit: 100 });
  const [dialogOpen, setDialogOpen] = useState(false);
  const [panelDialogOpen, setPanelDialogOpen] = useState(false);
  const [selectedId, setSelectedId] = useState<string>();
  const remove = useDeleteAdminDashboard();
  const update = useUpdateAdminDashboard(selectedId ?? "");
  const detail = useAdminDashboard(selectedId);
  const selectedPanels = useMemo(
    () => panelsFromDefinition(detail.data?.dashboard.definition),
    [detail.data?.dashboard.definition],
  );

  function addPanel(panel: Panel) {
    if (!selectedId || !detail.data) return;
    update.mutate(
      {
        name: detail.data.dashboard.name,
        description: detail.data.dashboard.description,
        definition: {
          ...detail.data.dashboard.definition,
          panels: [...selectedPanels, panel],
        },
      },
      { onSuccess: () => setPanelDialogOpen(false) },
    );
  }

  return (
    <AdminShell>
      <PageHeader
        eyebrow="Admin / Telemetry / Dashboards"
        title="Dashboards"
        description="Saved, owner-defined views backed by bounded telemetry queries. A panel never sends raw SQL to ClickHouse."
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
                  <Plus className="size-3.5" aria-hidden="true" /> New dashboard
                </Button>
              </DialogTrigger>
              <CreateDashboardDialog
                onCreated={(id) => {
                  setDialogOpen(false);
                  setSelectedId(id);
                }}
              />
            </Dialog>
          </div>
        }
      />
      <div className="grid gap-4 lg:grid-cols-[280px_minmax(0,1fr)]">
        <div className="overflow-hidden rounded-xl border border-graphite bg-carbon">
          <div className="border-b border-graphite px-4 py-3">
            <p className="text-xs uppercase tracking-[0.1em] text-fog">
              Saved views
            </p>
          </div>
          {dashboards.isPending ? (
            <div className="p-4">
              <LoadingState rows={3} />
            </div>
          ) : null}
          {dashboards.error ? (
            <div className="p-4">
              <ErrorState
                error={dashboards.error}
                retry={() => dashboards.refetch()}
              />
            </div>
          ) : null}
          {dashboards.data?.items.length ? (
            <div className="divide-y divide-graphite">
              {dashboards.data.items.map((dashboard) => (
                <button
                  key={dashboard.id}
                  type="button"
                  onClick={() => setSelectedId(dashboard.id)}
                  className={`flex w-full items-start gap-3 px-4 py-4 text-left transition-colors duration-150 hover:bg-white/[0.025] ${selectedId === dashboard.id ? "bg-white/[0.05]" : ""}`}
                >
                  <LayoutDashboard
                    className="mt-0.5 size-4 shrink-0 text-fog"
                    aria-hidden="true"
                  />
                  <span className="min-w-0">
                    <span
                      className="block truncate text-sm text-mist"
                      title={dashboard.name}
                    >
                      {dashboard.name}
                    </span>
                    <span
                      className="mt-1 block truncate text-xs text-fog"
                      title={`Updated ${formatDate(dashboard.updated_at)}`}
                    >
                      Updated {formatDate(dashboard.updated_at)}
                    </span>
                  </span>
                </button>
              ))}
            </div>
          ) : null}
          {dashboards.data && !dashboards.data.items.length ? (
            <p className="p-5 text-sm leading-6 text-fog">
              No saved dashboards yet.
            </p>
          ) : null}
        </div>
        <div className="min-w-0">
          {!selectedId ? (
            <AdminEmptyState
              icon={LayoutDashboard}
              title="Choose a saved dashboard"
              description="Dashboards query the authenticated telemetry API and show an explicit empty state when no signal has arrived."
              actionLabel="New dashboard"
              onAction={() => setDialogOpen(true)}
            />
          ) : null}
          {selectedId && detail.isPending ? <LoadingState rows={3} /> : null}
          {selectedId && detail.error ? (
            <ErrorState
              title="Could not load dashboard"
              error={detail.error}
              retry={() => detail.refetch()}
            />
          ) : null}
          {detail.data ? (
            <div className="space-y-4">
              <div className="flex flex-wrap items-start justify-between gap-4">
                <div>
                  <p className="text-xs uppercase tracking-[0.1em] text-fog">
                    Dashboard
                  </p>
                  <h2 className="mt-2 text-xl tracking-[-0.012em] text-paper">
                    {detail.data.dashboard.name}
                  </h2>
                  {detail.data.dashboard.description ? (
                    <p className="mt-1 text-sm text-fog">
                      {detail.data.dashboard.description}
                    </p>
                  ) : null}
                </div>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  disabled={remove.isPending}
                  onClick={() => {
                    if (
                      window.confirm(
                        `Delete the dashboard “${detail.data?.dashboard.name}”?`,
                      )
                    ) {
                      remove.mutate(detail.data!.dashboard.id, {
                        onSuccess: () => setSelectedId(undefined),
                      });
                    }
                  }}
                >
                  <Trash2 className="size-3.5" aria-hidden="true" /> Delete
                </Button>
              </div>
              <div className="flex justify-end">
                <Dialog
                  open={panelDialogOpen}
                  onOpenChange={setPanelDialogOpen}
                >
                  <DialogTrigger asChild>
                    <Button
                      type="button"
                      size="sm"
                      variant="secondary"
                      disabled={update.isPending}
                    >
                      <Plus className="size-3.5" aria-hidden="true" /> Add panel
                    </Button>
                  </DialogTrigger>
                  <AddPanelDialog onAdd={addPanel} />
                </Dialog>
              </div>
              {selectedPanels.length ? (
                <div className="grid gap-4 md:grid-cols-2">
                  {selectedPanels.map((panel) => (
                    <DashboardPanel
                      key={panel.id ?? `${panel.type}-${panel.title}`}
                      panel={panel}
                      query={timeRange.query}
                      refreshInterval={timeRange.refreshInterval}
                    />
                  ))}
                </div>
              ) : (
                <div className="rounded-xl border border-dashed border-graphite p-8 text-center text-sm text-fog">
                  This dashboard has no panels.
                </div>
              )}
              {remove.error ? (
                <p className="text-sm text-coral-red" role="alert">
                  {errorMessage(remove.error)}
                </p>
              ) : null}
              {update.error ? (
                <p className="text-sm text-coral-red" role="alert">
                  {errorMessage(update.error)}
                </p>
              ) : null}
            </div>
          ) : null}
        </div>
      </div>
    </AdminShell>
  );
}

function panelsFromDefinition(
  definition: Record<string, unknown> | undefined,
): Panel[] {
  if (!definition || !Array.isArray(definition.panels)) return [];
  return definition.panels.filter((value): value is Panel =>
    Boolean(
      value &&
      typeof value === "object" &&
      "type" in value &&
      typeof value.type === "string",
    ),
  );
}
