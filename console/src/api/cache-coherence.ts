import type { QueryClient } from "@tanstack/react-query";
import { queryKeys } from "@/api/query-keys";

export type CacheQueryKey = readonly unknown[];

/**
 * A domain change that can be translated into the queries whose source of
 * truth is now stale. Keeping this semantic input shared means realtime and
 * mutation adapters cannot quietly grow different invalidation policies.
 */
export type CacheChange =
  | { kind: "organizations" }
  | { kind: "organization-projects"; organizationId: string }
  | { kind: "organization-memberships"; organizationId: string }
  | { kind: "organization-invitations"; organizationId: string }
  | {
      kind: "account";
      includeOrganizations?: boolean;
      includeBootstrapStatus?: boolean;
    }
  | { kind: "account-sessions" }
  | { kind: "auth-settings"; projectId: string }
  | { kind: "service-layout"; projectId: string }
  | { kind: "agent"; projectId: string; agentId?: string }
  | {
      kind: "agent-run";
      projectId?: string;
      agentId?: string;
      runId?: string;
      includeAgent?: boolean;
      includeAgentDetail?: boolean;
    }
  | { kind: "function"; projectId: string; functionId?: string }
  | {
      kind: "function-variable";
      projectId: string;
      functionId: string;
    }
  | {
      kind: "function-deployment";
      projectId: string;
      functionId: string;
      deploymentId?: string;
    }
  | {
      kind: "function-execution";
      projectId: string;
      functionId?: string;
      executionId?: string;
    }
  | { kind: "site"; projectId: string; siteId?: string }
  | { kind: "app"; projectId: string; appId?: string }
  | { kind: "app-environment-variable"; projectId: string; appId: string }
  | {
      kind: "app-deployment";
      projectId: string;
      appId: string;
      deploymentId?: string;
    }
  | {
      kind: "site-deployment";
      projectId: string;
      siteId: string;
      deploymentId?: string;
      includeScope?: boolean;
    }
  | {
      kind: "webhook";
      projectId: string;
      webhookId?: string;
      includeDetail?: boolean;
      delivery?: boolean;
    }
  | { kind: "messaging"; projectId: string }
  | { kind: "database"; projectId: string; databaseId?: string }
  | {
      kind: "database-table";
      projectId: string;
      databaseId: string;
      tableId?: string;
    }
  | {
      kind: "database-table-schema";
      projectId: string;
      databaseId: string;
      tableId: string;
    }
  | {
      kind: "database-table-rows";
      projectId: string;
      databaseId: string;
      tableId: string;
    }
  | {
      kind: "database-backup";
      projectId: string;
      databaseId: string;
      restore?: boolean;
    }
  | {
      kind: "storage-bucket";
      projectId: string;
      bucketId?: string;
      includeDetail?: boolean;
    }
  | {
      kind: "storage-file";
      projectId: string;
      bucketId: string;
      fileId?: string;
      operation: "upload" | "rename" | "delete";
    }
  | { kind: "project-user"; projectId: string; userId?: string }
  | { kind: "api-key"; projectId: string; keyId?: string }
  | { kind: "project"; projectId: string };

/**
 * Appends a query key unless an equivalent key is already present. Query keys
 * are compared structurally so different arrays with the same content collapse
 * into a single invalidation target.
 */
export function addCacheKey(keys: CacheQueryKey[], key: CacheQueryKey) {
  if (
    !keys.some((candidate) => JSON.stringify(candidate) === JSON.stringify(key))
  ) {
    keys.push(key);
  }
}

/** Removes duplicate query keys while preserving first-seen order. */
export function dedupeCacheKeys(
  keys: readonly CacheQueryKey[],
): CacheQueryKey[] {
  const unique: CacheQueryKey[] = [];
  for (const key of keys) addCacheKey(unique, key);
  return unique;
}

export function invalidationKeysFor(change: CacheChange): CacheQueryKey[] {
  const keys: CacheQueryKey[] = [];

  switch (change.kind) {
    case "organizations":
      addCacheKey(keys, queryKeys.organizations);
      return keys;

    case "organization-projects":
      addCacheKey(keys, queryKeys.projects(change.organizationId));
      return keys;

    case "organization-memberships":
      addCacheKey(keys, queryKeys.memberships(change.organizationId));
      return keys;

    case "organization-invitations":
      addCacheKey(keys, queryKeys.invitations(change.organizationId));
      return keys;

    case "account":
      addCacheKey(keys, queryKeys.account);
      if (change.includeOrganizations)
        addCacheKey(keys, queryKeys.organizations);
      if (change.includeBootstrapStatus)
        addCacheKey(keys, queryKeys.bootstrapStatus);
      return keys;

    case "account-sessions":
      addCacheKey(keys, queryKeys.accountSessions);
      return keys;

    case "auth-settings":
      addCacheKey(keys, queryKeys.authSettings(change.projectId));
      return keys;

    case "service-layout":
      addCacheKey(keys, queryKeys.serviceLayout(change.projectId));
      return keys;

    case "agent":
      addCacheKey(keys, queryKeys.agents(change.projectId));
      if (change.agentId) addCacheKey(keys, queryKeys.agent(change.agentId));
      return keys;

    case "agent-run":
      if (change.agentId)
        addCacheKey(keys, queryKeys.agentRuns(change.agentId));
      if (change.agentId && change.runId) {
        addCacheKey(keys, queryKeys.agentRun(change.agentId, change.runId));
      }
      if ((!change.agentId || change.includeAgent) && change.projectId) {
        addCacheKey(keys, queryKeys.agents(change.projectId));
      }
      if (change.agentId && change.includeAgentDetail) {
        addCacheKey(keys, queryKeys.agent(change.agentId));
      }
      return keys;

    case "function":
      addCacheKey(keys, queryKeys.functions(change.projectId));
      if (change.functionId) {
        addCacheKey(
          keys,
          queryKeys.function(change.projectId, change.functionId),
        );
      }
      return keys;

    case "function-variable":
      addCacheKey(
        keys,
        queryKeys.functionVariables(change.projectId, change.functionId),
      );
      return keys;

    case "function-deployment":
      addCacheKey(
        keys,
        queryKeys.function(change.projectId, change.functionId),
      );
      addCacheKey(
        keys,
        queryKeys.functionDeployments(change.projectId, change.functionId),
      );
      if (change.deploymentId) {
        addCacheKey(
          keys,
          queryKeys.functionDeployment(
            change.projectId,
            change.functionId,
            change.deploymentId,
          ),
        );
      }
      return keys;

    case "function-execution":
      if (change.functionId) {
        addCacheKey(
          keys,
          queryKeys.functionExecutions(change.projectId, change.functionId),
        );
      }
      if (change.functionId && change.executionId) {
        addCacheKey(
          keys,
          queryKeys.functionExecution(
            change.projectId,
            change.functionId,
            change.executionId,
          ),
        );
      }
      return keys;

    case "site":
      addCacheKey(keys, queryKeys.sites(change.projectId));
      if (change.siteId) {
        addCacheKey(keys, queryKeys.site(change.projectId, change.siteId));
      }
      return keys;

    case "app":
      addCacheKey(keys, queryKeys.apps(change.projectId));
      if (change.appId) {
        addCacheKey(keys, queryKeys.app(change.projectId, change.appId));
        addCacheKey(
          keys,
          queryKeys.appDiagnostics(change.projectId, change.appId),
        );
      }
      return keys;

    case "app-deployment":
      addCacheKey(keys, queryKeys.apps(change.projectId));
      addCacheKey(keys, queryKeys.app(change.projectId, change.appId));
      addCacheKey(
        keys,
        queryKeys.appDiagnostics(change.projectId, change.appId),
      );
      addCacheKey(
        keys,
        queryKeys.appDeployments(change.projectId, change.appId),
      );
      if (change.deploymentId) {
        addCacheKey(
          keys,
          queryKeys.appDeployment(
            change.projectId,
            change.appId,
            change.deploymentId,
          ),
        );
      }
      return keys;

    case "app-environment-variable":
      addCacheKey(keys, queryKeys.app(change.projectId, change.appId));
      addCacheKey(
        keys,
        queryKeys.appDiagnostics(change.projectId, change.appId),
      );
      addCacheKey(
        keys,
        queryKeys.appEnvironmentVariables(change.projectId, change.appId),
      );
      return keys;

    case "site-deployment":
      addCacheKey(keys, queryKeys.site(change.projectId, change.siteId));
      addCacheKey(
        keys,
        queryKeys.siteDeployments(change.projectId, change.siteId),
      );
      if (change.deploymentId) {
        addCacheKey(
          keys,
          queryKeys.siteDeployment(
            change.projectId,
            change.siteId,
            change.deploymentId,
          ),
        );
      }
      if (change.includeScope || !change.deploymentId) {
        addCacheKey(
          keys,
          queryKeys.siteDeploymentScope(change.projectId, change.siteId),
        );
      }
      return keys;

    case "webhook":
      if (change.delivery && change.webhookId) {
        addCacheKey(
          keys,
          queryKeys.webhookDeliveries(change.projectId, change.webhookId),
        );
      }
      addCacheKey(keys, queryKeys.webhooks(change.projectId));
      if (change.includeDetail && change.webhookId) {
        addCacheKey(
          keys,
          queryKeys.webhook(change.projectId, change.webhookId),
        );
      }
      return keys;

    case "messaging":
      addCacheKey(keys, queryKeys.messagingProviders(change.projectId));
      addCacheKey(keys, queryKeys.messagingTopics(change.projectId));
      addCacheKey(keys, queryKeys.messagingMessages(change.projectId));
      return keys;

    case "database":
      addCacheKey(keys, queryKeys.databases(change.projectId));
      if (change.databaseId) {
        addCacheKey(
          keys,
          queryKeys.database(change.projectId, change.databaseId),
        );
      }
      return keys;

    case "database-table":
      addCacheKey(keys, queryKeys.tables(change.projectId, change.databaseId));
      if (change.tableId) {
        addCacheKey(
          keys,
          queryKeys.table(change.projectId, change.databaseId, change.tableId),
        );
      }
      return keys;

    case "database-table-schema":
      addCacheKey(
        keys,
        queryKeys.columns(change.projectId, change.databaseId, change.tableId),
      );
      addCacheKey(
        keys,
        queryKeys.rows(change.projectId, change.databaseId, change.tableId),
      );
      addCacheKey(
        keys,
        queryKeys.rowScope(change.projectId, change.databaseId, change.tableId),
      );
      addCacheKey(
        keys,
        queryKeys.indexes(change.projectId, change.databaseId, change.tableId),
      );
      return keys;

    case "database-table-rows":
      addCacheKey(
        keys,
        queryKeys.rows(change.projectId, change.databaseId, change.tableId),
      );
      addCacheKey(
        keys,
        queryKeys.rowScope(change.projectId, change.databaseId, change.tableId),
      );
      return keys;

    case "database-backup":
      if (change.restore) {
        addCacheKey(
          keys,
          queryKeys.rowsScope(change.projectId, change.databaseId),
        );
        addCacheKey(
          keys,
          queryKeys.rowDatabaseScope(change.projectId, change.databaseId),
        );
        addCacheKey(
          keys,
          queryKeys.tableScope(change.projectId, change.databaseId),
        );
        addCacheKey(
          keys,
          queryKeys.columnsScope(change.projectId, change.databaseId),
        );
        addCacheKey(
          keys,
          queryKeys.indexesScope(change.projectId, change.databaseId),
        );
        addCacheKey(
          keys,
          queryKeys.database(change.projectId, change.databaseId),
        );
        addCacheKey(
          keys,
          queryKeys.tables(change.projectId, change.databaseId),
        );
      }
      addCacheKey(
        keys,
        queryKeys.databaseBackups(change.projectId, change.databaseId),
      );
      return keys;

    case "storage-bucket":
      addCacheKey(keys, queryKeys.buckets(change.projectId));
      if (change.includeDetail && change.bucketId) {
        addCacheKey(keys, queryKeys.bucket(change.projectId, change.bucketId));
      }
      return keys;

    case "storage-file":
      addCacheKey(keys, queryKeys.files(change.projectId, change.bucketId));
      if (change.fileId) {
        addCacheKey(
          keys,
          queryKeys.file(change.projectId, change.bucketId, change.fileId),
        );
      }
      if (change.operation === "rename") {
        addCacheKey(
          keys,
          queryKeys.fileScope(change.projectId, change.bucketId),
        );
      } else {
        addCacheKey(keys, queryKeys.bucket(change.projectId, change.bucketId));
        addCacheKey(keys, queryKeys.buckets(change.projectId));
      }
      return keys;

    case "project-user":
      addCacheKey(keys, queryKeys.users(change.projectId));
      if (change.userId) {
        addCacheKey(
          keys,
          queryKeys.projectUser(change.projectId, change.userId),
        );
      }
      return keys;

    case "api-key":
      addCacheKey(keys, queryKeys.apiKeys(change.projectId));
      if (change.keyId) {
        addCacheKey(keys, queryKeys.apiKey(change.projectId, change.keyId));
      }
      return keys;

    case "project":
      addCacheKey(keys, queryKeys.project(change.projectId));
      addCacheKey(keys, queryKeys.audit("project", change.projectId));
      return keys;
  }
}

export async function applyCacheChanges(
  queryClient: Pick<QueryClient, "invalidateQueries">,
  changes: readonly CacheChange[],
) {
  const uniqueKeys = dedupeCacheKeys(changes.flatMap(invalidationKeysFor));

  await Promise.all(
    uniqueKeys.map((queryKey) => queryClient.invalidateQueries({ queryKey })),
  );
}

/**
 * An Admin control-room change. Admin events share this module with project
 * changes so both realtime adapters resolve to one invalidation policy instead
 * of growing separate key lists.
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
  const keys: CacheQueryKey[] = [["admin", "audit-events"]];

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
      addCacheKey(keys, ["admin", "telemetry", "errors"]);
      return keys;

    case "admin-overview":
      // Unknown Admin events still invalidate the bounded control-room overview
      // without forcing every telemetry query to refetch.
      addCacheKey(keys, queryKeys.adminOverview);
      return keys;
  }
}
