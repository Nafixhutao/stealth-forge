"use client";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { api, apiUrl, cancellableQuery } from "@/api/client";
import type { PathsV1AdminInfrastructureMetricsGetParametersQueryScope } from "@/api/generated/schema";
import { queryKeys } from "@/api/query-keys";
import { useEffect, useMemo, useState } from "react";

export type AdminTelemetryQuery = {
  from?: string;
  to?: string;
  service?: string;
  level?: string;
  query?: string;
  trace_id?: string;
  min_duration_ms?: number;
  name?: string;
  limit?: number;
};

export type AdminTailLog = {
  timestamp: string;
  trace_id?: string;
  span_id?: string;
  level?: string;
  service: string;
  message: string;
  attributes?: Record<string, string>;
  resource_attributes?: Record<string, string>;
};

export function useAdminLogTail(
  query: Pick<
    AdminTelemetryQuery,
    "from" | "to" | "service" | "level" | "query"
  > & { limit?: number },
  enabled: boolean,
) {
  const params = useMemo(() => {
    const search = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) {
      if (value !== undefined && value !== "") search.set(key, String(value));
    }
    return search.toString();
  }, [query]);
  const streamKey = useMemo(() => ({ enabled, params }), [enabled, params]);
  const [stream, setStream] = useState<{
    key: typeof streamKey | null;
    items: AdminTailLog[];
    error: string | null;
    connected: boolean;
  }>({ key: null, items: [], error: null, connected: false });

  useEffect(() => {
    if (!enabled || typeof EventSource === "undefined") {
      return;
    }
    // A query/filter change starts a fresh bounded stream. Reconnects on the
    // same EventSource retain the current items while the browser resumes
    // from the last SSE event id.
    const source = new EventSource(
      apiUrl(`/v1/admin/telemetry/logs/tail?${params}`),
      { withCredentials: true },
    );
    // A busy tail can emit many events per frame. Buffer them in the effect
    // closure and flush at most once per animation frame so each message does
    // not trigger a re-render (and a full re-dedupe of the visible list).
    const items: AdminTailLog[] = [];
    const seen = new Set<string>();
    const keyOf = (item: AdminTailLog) =>
      `${item.timestamp}\u0000${item.trace_id ?? ""}\u0000${item.span_id ?? ""}\u0000${item.service}\u0000${item.message}`;
    let frame: number | null = null;
    const flush = () => {
      frame = null;
      setStream((current) => {
        const base =
          current.key === streamKey
            ? current
            : {
                key: streamKey,
                items: [],
                error: null,
                connected: false,
              };
        return { ...base, items: items.slice(), error: null };
      });
    };
    const schedule = () => {
      if (frame !== null) return;
      frame = window.requestAnimationFrame(flush);
    };
    const onLog = (event: Event) => {
      try {
        const item = JSON.parse((event as MessageEvent).data) as AdminTailLog;
        const candidateKey = keyOf(item);
        if (seen.has(candidateKey)) return;
        seen.add(candidateKey);
        items.unshift(item);
        if (items.length > 250) {
          for (const dropped of items.splice(250)) {
            seen.delete(keyOf(dropped));
          }
        }
        schedule();
      } catch {
        setStream((current) => ({
          ...(current.key === streamKey
            ? current
            : { key: streamKey, items: [], connected: false }),
          error: "The live log stream returned an invalid event.",
        }));
      }
    };
    const onStreamError = (event: Event) => {
      try {
        const payload = JSON.parse((event as MessageEvent).data) as {
          message?: string;
        };
        setStream((current) => ({
          ...(current.key === streamKey
            ? current
            : { key: streamKey, items: [], connected: false }),
          error: payload.message ?? "Live log stream is unavailable.",
        }));
      } catch {
        setStream((current) => ({
          ...(current.key === streamKey
            ? current
            : { key: streamKey, items: [], connected: false }),
          error: "Live log stream is unavailable.",
        }));
      }
    };
    const onConnectionError = () => {
      setStream((current) => ({
        ...(current.key === streamKey
          ? current
          : { key: streamKey, items: [], connected: false }),
        connected: false,
        error: "Live log stream disconnected. The browser will retry.",
      }));
    };
    source.addEventListener("log", onLog);
    source.addEventListener("stream_error", onStreamError);
    source.onerror = onConnectionError;
    source.onopen = () => {
      setStream((current) => ({
        ...(current.key === streamKey
          ? current
          : { key: streamKey, items: [], connected: false }),
        connected: true,
        error: null,
      }));
    };
    return () => {
      if (frame !== null) window.cancelAnimationFrame(frame);
      source.removeEventListener("log", onLog);
      source.removeEventListener("stream_error", onStreamError);
      source.onerror = null;
      source.close();
    };
  }, [enabled, params, streamKey]);

  const isCurrentStream = stream.key === streamKey && enabled;

  return {
    items: isCurrentStream ? stream.items : [],
    error: isCurrentStream ? stream.error : null,
    connected: isCurrentStream && stream.connected,
  };
}

export function useAdminOverview(options?: {
  query?: Pick<AdminTelemetryQuery, "from" | "to">;
  refetchInterval?: number | false;
}) {
  return useQuery({
    queryKey: [...queryKeys.adminOverview, options?.query ?? {}],
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/admin/overview", {
        params: { query: options?.query ?? {} },
        signal,
      }),
    ),
    refetchInterval: options?.refetchInterval ?? 30_000,
  });
}
export function useAdminOperations(
  query: Pick<AdminTelemetryQuery, "from" | "to" | "limit"> = {},
  options?: { refetchInterval?: number | false },
) {
  return useQuery({
    queryKey: [...queryKeys.adminOperations, query],
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/admin/operations", { params: { query }, signal }),
    ),
    placeholderData: keepPreviousData,
    refetchInterval: options?.refetchInterval,
  });
}

// Host resource metrics are sampled in-process by the API (no telemetry
// backend). Poll a little faster than the heavier admin queries so the Overview
// resource cards and chart stay close to live. `windowMinutes` narrows the
// history; the server computes the cutoff, so render stays pure.
export function useAdminHostMetrics(
  windowMinutes?: number,
  options?: { refetchInterval?: number | false },
) {
  return useQuery({
    queryKey: [...queryKeys.adminHostMetrics, windowMinutes ?? "all"],
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/admin/host-metrics", {
        params: {
          query: windowMinutes ? { window_minutes: windowMinutes } : {},
        },
        signal,
      }),
    ),
    refetchInterval: options?.refetchInterval ?? 5_000,
  });
}

export function useAdminAuditEvents(
  query: { before?: string; limit?: number } = {},
  options?: { refetchInterval?: number | false },
) {
  return useQuery({
    queryKey: queryKeys.adminAuditEvents(query),
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/admin/audit-events", { params: { query }, signal }),
    ),
    placeholderData: keepPreviousData,
    refetchInterval: options?.refetchInterval,
  });
}

export function useAdminLogs(
  query: AdminTelemetryQuery = {},
  options?: { enabled?: boolean; refetchInterval?: number | false },
) {
  return useQuery({
    queryKey: queryKeys.adminTelemetry("logs", query),
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/admin/telemetry/logs", {
        params: { query },
        signal,
      }),
    ),
    placeholderData: keepPreviousData,
    enabled: options?.enabled ?? true,
    refetchInterval: options?.refetchInterval,
  });
}

export function useAdminLogVolume(
  query: AdminTelemetryQuery = {},
  options?: { refetchInterval?: number | false },
) {
  return useQuery({
    queryKey: queryKeys.adminTelemetry("log-volume", query),
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/admin/telemetry/log-volume", { params: { query }, signal }),
    ),
    placeholderData: keepPreviousData,
    refetchInterval: options?.refetchInterval,
  });
}

export function useAdminTraces(
  query: AdminTelemetryQuery = {},
  options?: { refetchInterval?: number | false },
) {
  return useQuery({
    queryKey: queryKeys.adminTelemetry("traces", query),
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/admin/telemetry/traces", {
        params: { query },
        signal,
      }),
    ),
    placeholderData: keepPreviousData,
    refetchInterval: options?.refetchInterval,
  });
}

export function useAdminErrors(
  query: Pick<
    AdminTelemetryQuery,
    "from" | "to" | "service" | "query" | "limit"
  > = {},
  options?: { refetchInterval?: number | false },
) {
  return useQuery({
    queryKey: queryKeys.adminTelemetry("errors", query),
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/admin/telemetry/errors", { params: { query }, signal }),
    ),
    placeholderData: keepPreviousData,
    refetchInterval: options?.refetchInterval,
  });
}

export function useAdminServiceMap(
  query: Pick<AdminTelemetryQuery, "from" | "to" | "limit"> = {},
  options?: { refetchInterval?: number | false },
) {
  return useQuery({
    queryKey: queryKeys.adminTelemetry("services", query),
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/admin/telemetry/services", { params: { query }, signal }),
    ),
    placeholderData: keepPreviousData,
    refetchInterval: options?.refetchInterval,
  });
}

export function useAdminInfrastructure(
  query: Pick<AdminTelemetryQuery, "from" | "to" | "limit"> & {
    scope?: PathsV1AdminInfrastructureMetricsGetParametersQueryScope;
  } = {},
  options?: { refetchInterval?: number | false },
) {
  return useQuery({
    queryKey: queryKeys.adminInfrastructure(query),
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/admin/infrastructure/metrics", {
        params: { query },
        signal,
      }),
    ),
    placeholderData: keepPreviousData,
    refetchInterval: options?.refetchInterval,
  });
}

export function useAdminMetrics(
  query: AdminTelemetryQuery = {},
  options?: { refetchInterval?: number | false },
) {
  return useQuery({
    queryKey: queryKeys.adminTelemetry("metrics", query),
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/admin/telemetry/metrics", {
        params: { query },
        signal,
      }),
    ),
    placeholderData: keepPreviousData,
    refetchInterval: options?.refetchInterval,
  });
}

export function useAdminSources(
  query: Pick<AdminTelemetryQuery, "from" | "to" | "limit"> = {},
  options?: { refetchInterval?: number | false },
) {
  return useQuery({
    queryKey: queryKeys.adminTelemetry("sources", query),
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/admin/telemetry/sources", {
        params: { query },
        signal,
      }),
    ),
    placeholderData: keepPreviousData,
    refetchInterval: options?.refetchInterval,
  });
}
