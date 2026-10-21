"use client";

import {
  keepPreviousData,
  useQuery,
  type UseQueryResult,
} from "@tanstack/react-query";
import { api, cancellableQuery } from "@/api/client";
import { useAdminLogTail } from "@/api/queries/admin-telemetry";
import { polledQuery } from "@/api/polling";
import { queryKeys } from "@/api/query-keys";
import { useMemo } from "react";
import type { LogEntry } from "../../types/logs";
import type { Trace } from "../../types/traces";
import type { Incident } from "../../types/incidents";
import {
  cpuHistoryFromMetrics,
  executionQueueFromOperations,
  hostFromMetrics,
  logFromApi,
  providersFromCatalog,
  resourceSeriesFromMetrics,
  runFromOperation,
  statusFromApi,
  toIncident,
  tracesFromSpans,
  usageFromOperations,
} from "./adapters";
import type {
  AdminAgentCatalog,
  AdminErrorGroup,
  AdminHostMetricsSnapshot,
  AdminOperationSummary,
  AdminReadState,
  AdminReadStatus,
  AdminStatusPage,
  AdminTimeRange,
  ExecutionQueueProjection,
  ProviderReadModel,
  RunProjection,
  StatusProjection,
  UsageProjection,
} from "./types";

export type {
  AdminErrorGroup,
  AdminOperation,
  AdminOperationSummary,
  AdminReadState,
  AdminReadStatus,
  AdminTimeRange,
  ExecutionQueueProjection,
  ProviderProjection,
  ProviderReadModel,
  RunProjection,
  StatusProjection,
  UsageProjection,
} from "./types";
export { toIncident } from "./adapters";

const RANGE_MS: Record<AdminTimeRange, number> = {
  "1h": 60 * 60 * 1000,
  "6h": 6 * 60 * 60 * 1000,
  "24h": 24 * 60 * 60 * 1000,
  "7d": 7 * 24 * 60 * 60 * 1000,
};

function windowFor(range: AdminTimeRange) {
  const to = new Date();
  const from = new Date(to.getTime() - RANGE_MS[range]);
  return { from: from.toISOString(), to: to.toISOString() };
}

function errorMessage(error: unknown): string {
  if (error instanceof Error && error.message) return error.message;
  return "The Admin API request failed.";
}

function readState<T>(
  queries: UseQueryResult[],
  data: T,
  refetch: () => void,
): AdminReadState<T> {
  const failed = queries.find((query) => query.isError);
  const status: AdminReadStatus = failed
    ? "error"
    : queries.some((query) => query.isPending)
      ? "pending"
      : "ready";
  return {
    status,
    data,
    error: failed ? errorMessage(failed.error) : null,
    isFetching: queries.some((query) => query.isFetching),
    refetch,
  };
}

function refetchAll(queries: UseQueryResult[]) {
  return () => {
    for (const query of queries) void query.refetch();
  };
}

function useAdminOverviewQuery() {
  return useQuery({
    queryKey: queryKeys.adminOverview,
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/admin/overview", { signal }),
    ),
    ...polledQuery(30_000),
  });
}

function useOperationsQuery(
  range: AdminTimeRange,
  limit: number,
  enabled = true,
) {
  return useQuery({
    queryKey: [...queryKeys.adminOperations, range, limit],
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/admin/operations", {
        params: { query: { ...windowFor(range), limit } },
        signal,
      }),
    ),
    placeholderData: keepPreviousData,
    enabled,
    ...polledQuery(15_000),
  });
}

export function useInfrastructureReadModel(
  range: AdminTimeRange = "1h",
): AdminReadState<{
  host: ReturnType<typeof hostFromMetrics>;
  series: ReturnType<typeof resourceSeriesFromMetrics>;
  cpuHistory: number[];
  snapshot: AdminHostMetricsSnapshot | undefined;
  operations: AdminOperationSummary | undefined;
}> {
  const host = useQuery({
    queryKey: [...queryKeys.adminHostMetrics, range],
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/admin/host-metrics", {
        params: {
          query: { window_minutes: Math.round(RANGE_MS[range] / 60_000) },
        },
        signal,
      }),
    ),
    ...polledQuery(5_000),
  });
  const overview = useAdminOverviewQuery();
  const snapshot = host.data;
  const operations = overview.data?.operations;
  const queries = [host, overview];
  const data = {
    host: hostFromMetrics(snapshot, operations?.running_jobs),
    series: resourceSeriesFromMetrics(snapshot),
    cpuHistory: cpuHistoryFromMetrics(snapshot),
    snapshot,
    operations,
  };
  return readState(queries, data, refetchAll(queries));
}

export function useExecutionQueueReadModel(): AdminReadState<ExecutionQueueProjection> {
  const operations = useOperationsQuery("24h", 100);
  const overview = useAdminOverviewQuery();
  const queries = [operations, overview];
  const data = executionQueueFromOperations(
    operations.data?.items ?? [],
    overview.data?.operations ?? null,
  );
  return readState(queries, data, refetchAll(queries));
}

export function useRunsReadModel(
  range: AdminTimeRange = "7d",
): AdminReadState<RunProjection[]> {
  const operations = useOperationsQuery(range, 100);
  const data = useMemo(
    () =>
      (operations.data?.items ?? [])
        .filter((item) => item.kind === "agent_run")
        .map(runFromOperation),
    [operations.data],
  );
  return readState([operations], data, refetchAll([operations]));
}

export function useLogsReadModel(options: {
  range: AdminTimeRange;
  service?: string;
  level?: string;
  search?: string;
  live: boolean;
}): AdminReadState<LogEntry[]> {
  const { range, service, level, search, live } = options;
  const base = useQuery({
    queryKey: [
      ...queryKeys.adminTelemetrySignal("logs"),
      range,
      service ?? "",
      level ?? "",
      search ?? "",
    ],
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/admin/telemetry/logs", {
        params: {
          query: {
            ...windowFor(range),
            service: service || undefined,
            level: level || undefined,
            query: search || undefined,
            limit: 250,
          },
        },
        signal,
      }),
    ),
    placeholderData: keepPreviousData,
    enabled: !live,
    ...polledQuery(30_000),
  });
  const tailWindow = useMemo(() => windowFor(range), [range]);
  const tail = useAdminLogTail(
    {
      ...tailWindow,
      service: service || undefined,
      level: level || undefined,
      query: search || undefined,
      limit: 250,
    },
    live,
  );
  const data = useMemo(() => {
    const merged = new Map<string, LogEntry>();
    for (const item of tail.items) {
      const entry = logFromApi(item);
      merged.set(entry.id, entry);
    }
    for (const item of base.data?.items ?? []) {
      const entry = logFromApi(item);
      merged.set(entry.id, entry);
    }
    return [...merged.values()].sort((a, b) =>
      b.timestamp.localeCompare(a.timestamp),
    );
  }, [base.data, tail.items]);
  const tailError = tail.error ? new Error(tail.error) : null;
  return {
    status: tailError ? "error" : base.isPending && !live ? "pending" : "ready",
    data,
    error: tailError
      ? errorMessage(tailError)
      : base.error
        ? errorMessage(base.error)
        : null,
    isFetching: base.isFetching,
    refetch: () => void base.refetch(),
  };
}

export function useTracesReadModel(
  range: AdminTimeRange = "1h",
): AdminReadState<Trace[]> {
  const traces = useQuery({
    queryKey: [...queryKeys.adminTelemetrySignal("traces"), range],
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/admin/telemetry/traces", {
        params: { query: { ...windowFor(range), limit: 250 } },
        signal,
      }),
    ),
    placeholderData: keepPreviousData,
    ...polledQuery(15_000),
  });
  const overview = useAdminOverviewQuery();
  const data = useMemo(
    () => tracesFromSpans(traces.data?.items ?? []),
    [traces.data],
  );
  return readState([traces, overview], data, refetchAll([traces, overview]));
}

export function useTraceMetrics() {
  const overview = useAdminOverviewQuery();
  return {
    http: overview.data?.http,
    isPending: overview.isPending,
    error: overview.error,
  };
}

export function useErrorsReadModel(
  range: AdminTimeRange = "24h",
): AdminReadState<AdminErrorGroup[]> {
  const errors = useQuery({
    queryKey: [...queryKeys.adminTelemetrySignal("errors"), range],
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/admin/telemetry/errors", {
        params: { query: { ...windowFor(range), limit: 250 } },
        signal,
      }),
    ),
    placeholderData: keepPreviousData,
    ...polledQuery(30_000),
  });
  const data = useMemo(() => errors.data?.items ?? [], [errors.data]);
  return readState([errors], data, refetchAll([errors]));
}

export function useProvidersReadModel(): AdminReadState<ProviderReadModel> {
  const catalog = useQuery({
    queryKey: queryKeys.agentCatalog,
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/agent-catalog", { signal }),
    ),
  });
  const data = useMemo<ProviderReadModel>(() => {
    const response = catalog.data as AdminAgentCatalog | undefined;
    return {
      providers: providersFromCatalog(response),
      execution: response?.execution ?? null,
    };
  }, [catalog.data]);
  return readState([catalog], data, refetchAll([catalog]));
}

export function useUsageReadModel(
  range: AdminTimeRange = "7d",
): AdminReadState<UsageProjection> {
  const operations = useOperationsQuery(range, 100);
  const overview = useAdminOverviewQuery();
  const queries = [operations, overview];
  const data = usageFromOperations(
    operations.data?.items ?? [],
    overview.data?.operations ?? null,
  );
  return readState(queries, data, refetchAll(queries));
}

export function useStatusReadModel(): AdminReadState<StatusProjection> {
  const status = useQuery({
    queryKey: queryKeys.adminStatusPage,
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/admin/status-page", { signal }),
    ),
    ...polledQuery(30_000),
  });
  const incidents = useQuery({
    queryKey: [...queryKeys.adminIncidents, { limit: 20 }],
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/admin/incidents", {
        params: { query: { limit: 20 } },
        signal,
      }),
    ),
    ...polledQuery(30_000),
  });
  const data = useMemo(
    () =>
      statusFromApi(
        status.data as AdminStatusPage | undefined,
        (incidents.data?.items ?? []).map(toIncident),
      ),
    [incidents.data, status.data],
  );
  return readState([status, incidents], data, refetchAll([status, incidents]));
}

export function useIncidentsReadModel(): AdminReadState<Incident[]> {
  const incidents = useQuery({
    queryKey: [...queryKeys.adminIncidents, { limit: 100 }],
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/admin/incidents", {
        params: { query: { limit: 100 } },
        signal,
      }),
    ),
    placeholderData: keepPreviousData,
    ...polledQuery(30_000),
  });
  const data = useMemo(
    () => (incidents.data?.items ?? []).map(toIncident),
    [incidents.data],
  );
  return readState([incidents], data, refetchAll([incidents]));
}
