import { queryKeys } from "@/api/query-keys";
import { addCacheKey, type CacheQueryKey } from "./keys";

/**
 * An Admin control-room change. Admin events resolve through the same
 * invalidation vocabulary as project changes, so a realtime adapter for
 * `/v1/admin/realtime` and the Admin mutations cannot grow separate key lists.
 */
export type AdminCacheChange =
  | { kind: "admin-monitor"; resourceId?: string }
  | { kind: "admin-alert"; resourceId?: string }
  | { kind: "admin-notification" }
  | { kind: "admin-incident"; resourceId?: string }
  | { kind: "admin-dashboard"; resourceId?: string }
  | { kind: "admin-status-page" }
  | { kind: "admin-error-group" }
  | { kind: "admin-overview" };

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
  }
}
