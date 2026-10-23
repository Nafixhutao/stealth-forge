"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/api/client";
import { invalidate } from "@/api/cache-coherence";
import type { components } from "@/api/generated/schema";

/**
 * The Admin control room's writes.
 *
 * Each mutation states what changed and lets the cache-coherence module decide
 * which queries go stale. Deleting a resource marks the change `forget`, so its
 * detail query is dropped rather than refetched into a 404.
 */

export function useCreateAdminMonitor() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (
      body: components["schemas"]["CreateAdminMonitorRequest"],
    ) => unwrap(await api.POST("/v1/admin/monitors", { body })),
    onSuccess: () => invalidate(queryClient, [{ kind: "admin-monitor" }]),
  });
}

export function useUpdateAdminMonitor(monitorId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (
      body: components["schemas"]["UpdateAdminMonitorRequest"],
    ) =>
      unwrap(
        await api.PUT("/v1/admin/monitors/{monitorID}", {
          params: { path: { monitorID: monitorId } },
          body,
        }),
      ),
    onSuccess: () =>
      invalidate(queryClient, [
        { kind: "admin-monitor", resourceId: monitorId },
      ]),
  });
}

export function useDeleteAdminMonitor() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (monitorId: string) =>
      unwrap(
        await api.DELETE("/v1/admin/monitors/{monitorID}", {
          params: { path: { monitorID: monitorId } },
        }),
      ),
    onSuccess: (_, monitorId) =>
      invalidate(queryClient, [
        { kind: "admin-monitor", resourceId: monitorId, forget: true },
      ]),
  });
}

export function useCreateAdminAlert() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (
      body: components["schemas"]["CreateAdminAlertRuleRequest"],
    ) => unwrap(await api.POST("/v1/admin/alerts", { body })),
    onSuccess: () => invalidate(queryClient, [{ kind: "admin-alert" }]),
  });
}

export function useDeleteAdminAlert() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (alertRuleId: string) =>
      unwrap(
        await api.DELETE("/v1/admin/alerts/{alertRuleID}", {
          params: { path: { alertRuleID: alertRuleId } },
        }),
      ),
    onSuccess: (_, alertRuleId) =>
      invalidate(queryClient, [
        { kind: "admin-alert", resourceId: alertRuleId, forget: true },
      ]),
  });
}

export function useUpdateAdminErrorStatus() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      fingerprint,
      status,
    }: {
      fingerprint: string;
      status: components["schemas"]["UpdateAdminErrorGroupStatusRequest"]["status"];
    }) =>
      unwrap(
        await api.PATCH("/v1/admin/telemetry/errors/{fingerprint}", {
          params: { path: { fingerprint } },
          body: { status },
        }),
      ),
    onSuccess: () => invalidate(queryClient, [{ kind: "admin-error-group" }]),
  });
}

export function useCreateAdminNotificationChannel() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (
      body: components["schemas"]["CreateAdminNotificationChannelRequest"],
    ) => unwrap(await api.POST("/v1/admin/notifications", { body })),
    onSuccess: () => invalidate(queryClient, [{ kind: "admin-notification" }]),
  });
}

export function useDeleteAdminNotificationChannel() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (channelId: string) =>
      unwrap(
        await api.DELETE("/v1/admin/notifications/{channelID}", {
          params: { path: { channelID: channelId } },
        }),
      ),
    onSuccess: () => invalidate(queryClient, [{ kind: "admin-notification" }]),
  });
}

export function useTestAdminNotificationChannel() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (channelId: string) =>
      unwrap(
        await api.POST("/v1/admin/notifications/{channelID}/test", {
          params: { path: { channelID: channelId } },
        }),
      ),
    onSuccess: () => invalidate(queryClient, [{ kind: "admin-notification" }]),
  });
}

export function useCreateAdminIncident() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (
      body: components["schemas"]["CreateAdminIncidentRequest"],
    ) => unwrap(await api.POST("/v1/admin/incidents", { body })),
    onSuccess: () => invalidate(queryClient, [{ kind: "admin-incident" }]),
  });
}

export function useUpdateAdminIncident(incidentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (
      body: components["schemas"]["UpdateAdminIncidentRequest"],
    ) =>
      unwrap(
        await api.PATCH("/v1/admin/incidents/{incidentID}", {
          params: { path: { incidentID: incidentId } },
          body,
        }),
      ),
    onSuccess: () =>
      invalidate(queryClient, [
        { kind: "admin-incident", resourceId: incidentId },
      ]),
  });
}

export function useAddAdminIncidentEvent(incidentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (
      body: components["schemas"]["AddAdminIncidentEventRequest"],
    ) =>
      unwrap(
        await api.POST("/v1/admin/incidents/{incidentID}/events", {
          params: { path: { incidentID: incidentId } },
          body,
        }),
      ),
    onSuccess: () =>
      invalidate(queryClient, [
        { kind: "admin-incident", resourceId: incidentId },
      ]),
  });
}

export function useCreateAdminDashboard() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (
      body: components["schemas"]["CreateAdminDashboardRequest"],
    ) => unwrap(await api.POST("/v1/admin/dashboards", { body })),
    onSuccess: () => invalidate(queryClient, [{ kind: "admin-dashboard" }]),
  });
}

export function useUpdateAdminDashboard(dashboardId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (
      body: components["schemas"]["CreateAdminDashboardRequest"],
    ) =>
      unwrap(
        await api.PUT("/v1/admin/dashboards/{dashboardID}", {
          params: { path: { dashboardID: dashboardId } },
          body,
        }),
      ),
    onSuccess: () =>
      invalidate(queryClient, [
        { kind: "admin-dashboard", resourceId: dashboardId },
      ]),
  });
}

export function useDeleteAdminDashboard() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (dashboardId: string) =>
      unwrap(
        await api.DELETE("/v1/admin/dashboards/{dashboardID}", {
          params: { path: { dashboardID: dashboardId } },
        }),
      ),
    onSuccess: (_, dashboardId) =>
      invalidate(queryClient, [
        { kind: "admin-dashboard", resourceId: dashboardId, forget: true },
      ]),
  });
}

export function useUpdateAdminStatusPage() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (
      body: components["schemas"]["UpdateAdminStatusPageRequest"],
    ) => unwrap(await api.PUT("/v1/admin/status-page", { body })),
    onSuccess: () => invalidate(queryClient, [{ kind: "admin-status-page" }]),
  });
}

export function useUpdateAdminAccountPlan() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      accountID,
      plan,
    }: {
      accountID: string;
      plan: components["schemas"]["UpdateAdminAccountPlanRequest"]["plan"];
    }) =>
      unwrap(
        await api.PUT("/v1/admin/accounts/{accountID}/plan", {
          params: { path: { accountID } },
          body: { plan },
        }),
      ),
    onSuccess: () => invalidate(queryClient, [{ kind: "admin-account" }]),
  });
}
