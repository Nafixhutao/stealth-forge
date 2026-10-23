import { queryKeys } from "@/api/query-keys";
import { addCacheKey, type CacheQueryKey } from "./keys";

/**
 * An Admin control-room change. Admin events resolve through the same
 * invalidation vocabulary as project changes, so a realtime adapter for
 * `/v1/admin/realtime` and the Admin mutations cannot grow separate key lists.
 *
 * `resourceId` narrows the change to one resource's detail query. `forget`
 * marks a deletion: the detail query is dropped instead of refetched, because
 * a deleted row can only answer with 404.
 */
export type AdminCacheChange =
  | { kind: "admin-monitor"; resourceId?: string; forget?: boolean }
  | { kind: "admin-alert"; resourceId?: string; forget?: boolean }
  | { kind: "admin-notification" }
  | { kind: "admin-incident"; resourceId?: string; forget?: boolean }
  | { kind: "admin-dashboard"; resourceId?: string; forget?: boolean }
  | { kind: "admin-status-page" }
  | { kind: "admin-error-group" }
  | { kind: "admin-overview" }
  | { kind: "admin-account" }
  | { kind: "admin-oauth-provider" }
  | { kind: "admin-operation" };

export function adminInvalidationKeysFor(
  change: AdminCacheChange,
): CacheQueryKey[] {
  const keys: CacheQueryKey[] = [queryKeys.adminAuditEventsScope];

  switch (change.kind) {
    case "admin-monitor":
      addCacheKey(keys, queryKeys.adminMonitors);
      if (change.resourceId)
        addCacheKey(keys, queryKeys.adminMonitor(change.resourceId));
      return keys;

    case "admin-alert":
      addCacheKey(keys, queryKeys.adminAlerts);
      addCacheKey(keys, queryKeys.adminAlertEvents);
      if (change.resourceId)
        addCacheKey(keys, queryKeys.adminAlert(change.resourceId));
      return keys;

    case "admin-notification":
      addCacheKey(keys, queryKeys.adminNotifications);
      return keys;

    case "admin-incident":
      addCacheKey(keys, queryKeys.adminIncidents);
      if (change.resourceId)
        addCacheKey(keys, queryKeys.adminIncident(change.resourceId));
      return keys;

    case "admin-dashboard":
      addCacheKey(keys, queryKeys.adminDashboards);
      if (change.resourceId)
        addCacheKey(keys, queryKeys.adminDashboard(change.resourceId));
      return keys;

    case "admin-status-page":
      addCacheKey(keys, queryKeys.adminStatusPage);
      return keys;

    case "admin-error-group":
      addCacheKey(keys, queryKeys.adminTelemetrySignal("errors"));
      return keys;

    case "admin-overview":
      // Unknown Admin events still invalidate the bounded control-room overview
      // without forcing every telemetry query to refetch.
      addCacheKey(keys, queryKeys.adminOverview);
      return keys;

    case "admin-account":
      addCacheKey(keys, queryKeys.adminAccounts);
      return keys;

    case "admin-oauth-provider":
      addCacheKey(keys, queryKeys.adminOAuthProviders);
      return keys;

    case "admin-operation":
      addCacheKey(keys, queryKeys.adminOperations);
      return keys;
  }
}

export function adminForgottenKeysFor(
  change: AdminCacheChange,
): CacheQueryKey[] {
  if (!("forget" in change) || !change.forget || !change.resourceId) {
    return [];
  }
  switch (change.kind) {
    case "admin-monitor":
      return [queryKeys.adminMonitor(change.resourceId)];
    case "admin-alert":
      return [queryKeys.adminAlert(change.resourceId)];
    case "admin-incident":
      return [queryKeys.adminIncident(change.resourceId)];
    case "admin-dashboard":
      return [queryKeys.adminDashboard(change.resourceId)];
    default:
      return [];
  }
}
