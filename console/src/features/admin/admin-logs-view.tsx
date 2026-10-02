"use client";

import { useMemo, useState } from "react";
import { useSearchParams } from "next/navigation";
import { Download, History, Search, X } from "lucide-react";
import { useAdminLogTail, useAdminLogs } from "@/api/queries";
import { ErrorState } from "@/components/feedback/error-state";
import { LoadingState } from "@/components/feedback/loading-state";
import { PageHeader } from "@/components/page-header";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { AdminShell } from "./admin-shell";
import { formatAdminLogQuery, parseAdminLogQuery } from "./admin-log-query";
import { AdminQueryEditorLoader } from "./admin-query-editor-loader";
import { AdminTimeRange, useAdminTimeRange } from "./admin-time-range";
import { downloadLogs, VirtualizedLogTable } from "./admin-log-table";

const logQueryHistoryKey = "stealth.admin.log-query-history";

function readLogQueryHistory(): string[] {
  if (typeof window === "undefined") return [];
  try {
    const parsed = JSON.parse(
      window.localStorage.getItem(logQueryHistoryKey) ?? "[]",
    );
    if (!Array.isArray(parsed)) return [];
    return parsed
      .filter((item): item is string => typeof item === "string")
      .slice(0, 8);
  } catch {
    return [];
  }
}

export function AdminLogTailStatus({
  connected,
  error,
}: {
  connected: boolean;
  error: string | null;
}) {
  return (
    <div
      className="mb-4 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-fog"
      role="status"
      aria-live="polite"
      aria-atomic="true"
    >
      <span>
        {connected ? "Streaming new records" : "Connecting to live stream…"}
      </span>
      {error ? <span className="text-coral-red">{error}</span> : null}
    </div>
  );
}

export function AdminLogsView() {
  const timeRange = useAdminTimeRange();
  const searchParams = useSearchParams();
  const traceIDParam = searchParams.get("trace_id") ?? "";
  const initialQuery = useMemo(
    () =>
      formatAdminLogQuery({
        trace_id: traceIDParam,
      }),
    [traceIDParam],
  );
  const [draftQuery, setDraftQuery] = useState(initialQuery);
  const [activeQuery, setActiveQuery] = useState(initialQuery);
  const [queryHistory, setQueryHistory] = useState(readLogQueryHistory);
  const [tailEnabled, setTailEnabled] = useState(false);
  // The route can be reused with a different trace_id (for example when
  // following a "View linked logs" link) without remounting this view. Adjust
  // the applied query during render when the URL-derived query changes (the
  // React-recommended alternative to syncing state from an effect).
  const [syncedInitialQuery, setSyncedInitialQuery] = useState(initialQuery);
  if (initialQuery !== syncedInitialQuery) {
    setSyncedInitialQuery(initialQuery);
    setDraftQuery(initialQuery);
    setActiveQuery(initialQuery);
  }
  const draftParse = useMemo(
    () => parseAdminLogQuery(draftQuery),
    [draftQuery],
  );
  const activeParse = useMemo(
    () => parseAdminLogQuery(activeQuery),
    [activeQuery],
  );
  const query = useMemo(
    () => ({
      ...timeRange.query,
      ...activeParse.filters,
      limit: 100,
    }),
    [activeParse.filters, timeRange.query],
  );
  const logs = useAdminLogs(query, {
    enabled: activeParse.valid,
    refetchInterval: timeRange.refreshInterval,
  });
  const tail = useAdminLogTail(query, tailEnabled && activeParse.valid);
  const displayItems = tailEnabled
    ? tail.items.length
      ? tail.items
      : (logs.data?.items ?? [])
    : (logs.data?.items ?? []);

  return (
    <AdminShell>
      <PageHeader
        eyebrow="Admin / Telemetry"
        title="Logs"
        description="Search structured logs collected from the Stealth runtime and its Docker services."
        actions={
          <div className="flex flex-wrap items-center justify-end gap-2">
            <Button
              type="button"
              size="sm"
              variant={tailEnabled ? "secondary" : "ghost"}
              aria-pressed={tailEnabled}
              onClick={() => setTailEnabled((enabled) => !enabled)}
            >
              <span
                className={`size-1.5 rounded-full ${tail.connected ? "bg-pulse-green" : "bg-ash"}`}
                aria-hidden="true"
              />
              {tailEnabled ? "Live tail on" : "Live tail"}
            </Button>
            <AdminTimeRange
              rangeKey={timeRange.rangeKey}
              refreshKey={timeRange.refreshKey}
              customRange={timeRange.customRange}
              onRangeChange={timeRange.setRange}
              onRefreshChange={timeRange.setRefresh}
              onCustomRangeChange={timeRange.setCustomRange}
            />
          </div>
        }
      />
      {tailEnabled ? (
        <AdminLogTailStatus connected={tail.connected} error={tail.error} />
      ) : null}
      <Card className="mb-4">
        <CardContent className="space-y-3 p-4">
          <div className="flex items-start gap-3">
            <Search
              className="mt-3 size-4 shrink-0 text-ash"
              aria-hidden="true"
            />
            <div className="min-w-0 flex-1">
              <label className="mb-2 block text-xs uppercase tracking-[0.1em] text-fog">
                Structured query
              </label>
              <AdminQueryEditorLoader
                value={draftQuery}
                onChange={setDraftQuery}
                onSubmit={() => applyLogQuery(draftQuery, draftParse.valid)}
                invalid={!draftParse.valid}
              />
            </div>
            <Button
              type="button"
              size="sm"
              variant="default"
              onClick={() => applyLogQuery(draftQuery, draftParse.valid)}
              disabled={!draftParse.valid}
            >
              Apply
            </Button>
          </div>
          <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-fog">
            <p>
              Use{" "}
              <code className="font-mono text-mist">
                service:api level:ERROR
              </code>
              , <code className="font-mono text-mist">trace_id:…</code>, or{" "}
              <code className="font-mono text-mist">
                message:&quot;timeout&quot;
              </code>
              . Press Ctrl/Cmd+Enter to apply.
            </p>
            <div className="flex items-center gap-2">
              {queryHistory.length ? (
                <label className="flex items-center gap-1.5">
                  <History className="size-3.5" aria-hidden="true" />
                  <span className="sr-only">Recent log queries</span>
                  <select
                    value=""
                    onChange={(event) => {
                      const next = event.target.value;
                      if (next) setDraftQuery(next);
                    }}
                    className="min-h-11 max-w-[220px] rounded-md border border-control-border bg-carbon px-2 py-1.5 text-xs text-mist focus-visible:border-acid-lime/70 focus-visible:outline-none"
                  >
                    <option value="">History</option>
                    {queryHistory.map((item) => (
                      <option key={item} value={item}>
                        {item}
                      </option>
                    ))}
                  </select>
                </label>
              ) : null}
              <Button
                type="button"
                size="sm"
                variant="ghost"
                onClick={() => {
                  setDraftQuery("");
                  applyLogQuery("", true);
                }}
              >
                <X className="size-3.5" aria-hidden="true" />
                Clear
              </Button>
            </div>
          </div>
          {draftParse.diagnostics.map((diagnostic) => (
            <p
              key={`${diagnostic.from}-${diagnostic.message}`}
              className="text-xs text-coral-red"
            >
              {diagnostic.message}
            </p>
          ))}
        </CardContent>
      </Card>
      {logs.data?.items.length ? (
        <div className="mb-3 flex justify-end">
          <Button
            type="button"
            size="sm"
            variant="ghost"
            onClick={() => downloadLogs(logs.data?.items ?? [])}
          >
            <Download className="size-3.5" aria-hidden="true" />
            Download current result
          </Button>
        </div>
      ) : null}
      {logs.isPending ? <LoadingState rows={6} /> : null}
      {logs.error ? (
        <ErrorState
          title="Could not load logs"
          error={logs.error}
          retry={() => logs.refetch()}
        />
      ) : null}
      {logs.data && !displayItems.length ? (
        <Card>
          <CardContent className="p-8 text-center text-sm text-fog">
            No logs matched the selected filters.
          </CardContent>
        </Card>
      ) : null}
      {displayItems.length ? (
        <Card className="overflow-hidden">
          <VirtualizedLogTable items={displayItems} />
        </Card>
      ) : null}
    </AdminShell>
  );

  function applyLogQuery(next: string, valid: boolean) {
    if (!valid) return;
    setActiveQuery(next);
    const trimmed = next.trim();
    if (!trimmed) return;
    setQueryHistory((current) => {
      const nextHistory = [
        trimmed,
        ...current.filter((item) => item !== trimmed),
      ].slice(0, 8);
      try {
        window.localStorage.setItem(
          logQueryHistoryKey,
          JSON.stringify(nextHistory),
        );
      } catch {
        // History is optional and must not block log exploration.
      }
      return nextHistory;
    });
  }
}

export { VirtualizedLogTable } from "./admin-log-table";
