"use client";
import {
  keepPreviousData,
  useInfiniteQuery,
  useQuery,
} from "@tanstack/react-query";
import { api, cancellableQuery, unwrap } from "@/api/client";
import { queryKeys } from "@/api/query-keys";

export function usePublicStatusPage() {
  return useQuery({
    queryKey: queryKeys.publicStatusPage,
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/status-page", { signal }),
    ),
    retry: false,
  });
}
export function useAdminMonitors(
  query: { limit?: number } = {},
  options?: { refetchInterval?: number | false },
) {
  return useQuery({
    queryKey: [...queryKeys.adminMonitors, query],
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/admin/monitors", { params: { query }, signal }),
    ),
    placeholderData: keepPreviousData,
    refetchInterval: options?.refetchInterval,
  });
}

export function useAdminMonitor(
  monitorId: string | undefined,
  options?: { refetchInterval?: number | false },
) {
  return useQuery({
    queryKey: queryKeys.adminMonitor(monitorId ?? ""),
    enabled: Boolean(monitorId),
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/admin/monitors/{monitorID}", {
        params: { path: { monitorID: monitorId! } },
        signal,
      }),
    ),
    refetchInterval: options?.refetchInterval,
  });
}

export function useAdminAlerts(
  query: { limit?: number } = {},
  options?: { refetchInterval?: number | false },
) {
  return useQuery({
    queryKey: [...queryKeys.adminAlerts, query],
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/admin/alerts", { params: { query }, signal }),
    ),
    placeholderData: keepPreviousData,
    refetchInterval: options?.refetchInterval,
  });
}

export function useAdminAlertEvents(
  query: { from?: string; to?: string; limit?: number } = {},
  options?: { refetchInterval?: number | false },
) {
  const pageQuery = { ...query };
  return useInfiniteQuery({
    queryKey: [...queryKeys.adminAlertEvents, pageQuery],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ signal, pageParam }) => {
      const result = await api.GET("/v1/admin/alert-events", {
        params: { query: { ...pageQuery, cursor: pageParam } },
        signal,
      });
      return unwrap(result);
    },
    getNextPageParam: (lastPage) => lastPage?.next_cursor ?? undefined,
    refetchInterval: options?.refetchInterval,
  });
}

export function useAdminNotificationChannels(
  query: { limit?: number } = {},
  options?: { refetchInterval?: number | false },
) {
  return useQuery({
    queryKey: [...queryKeys.adminNotifications, query],
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/admin/notifications", { params: { query }, signal }),
    ),
    placeholderData: keepPreviousData,
    refetchInterval: options?.refetchInterval,
  });
}

export function useAdminAlert(
  alertRuleId: string | undefined,
  options?: { refetchInterval?: number | false },
) {
  return useQuery({
    queryKey: queryKeys.adminAlert(alertRuleId ?? ""),
    enabled: Boolean(alertRuleId),
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/admin/alerts/{alertRuleID}", {
        params: { path: { alertRuleID: alertRuleId! } },
        signal,
      }),
    ),
    refetchInterval: options?.refetchInterval,
  });
}

export function useAdminIncidents(
  query: { limit?: number } = {},
  options?: { refetchInterval?: number | false },
) {
  return useQuery({
    queryKey: [...queryKeys.adminIncidents, query],
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/admin/incidents", { params: { query }, signal }),
    ),
    placeholderData: keepPreviousData,
    refetchInterval: options?.refetchInterval,
  });
}

export function useAdminIncident(
  incidentId: string | undefined,
  options?: { refetchInterval?: number | false },
) {
  return useQuery({
    queryKey: queryKeys.adminIncident(incidentId ?? ""),
    enabled: Boolean(incidentId),
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/admin/incidents/{incidentID}", {
        params: { path: { incidentID: incidentId! } },
        signal,
      }),
    ),
    refetchInterval: options?.refetchInterval,
  });
}

export function useAdminDashboards(
  query: { limit?: number } = {},
  options?: { refetchInterval?: number | false },
) {
  return useQuery({
    queryKey: [...queryKeys.adminDashboards, query],
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/admin/dashboards", { params: { query }, signal }),
    ),
    placeholderData: keepPreviousData,
    refetchInterval: options?.refetchInterval,
  });
}

export function useAdminDashboard(
  dashboardId: string | undefined,
  options?: { refetchInterval?: number | false },
) {
  return useQuery({
    queryKey: queryKeys.adminDashboard(dashboardId ?? ""),
    enabled: Boolean(dashboardId),
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/admin/dashboards/{dashboardID}", {
        params: { path: { dashboardID: dashboardId! } },
        signal,
      }),
    ),
    refetchInterval: options?.refetchInterval,
  });
}

export function useAdminStatusPage(options?: {
  refetchInterval?: number | false;
}) {
  return useQuery({
    queryKey: queryKeys.adminStatusPage,
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/admin/status-page", { signal }),
    ),
    refetchInterval: options?.refetchInterval,
  });
}
