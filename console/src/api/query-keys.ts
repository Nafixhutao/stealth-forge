import type { PathsV1AdminInfrastructureMetricsGetParametersQueryScope } from "@/api/generated/schema";

/**
 * Parameter shapes accepted by the Admin key factories. A query key embeds its
 * parameters, so the factory and its parameter type stay together here rather
 * than in the query hooks that call them. `admin-telemetry.ts` re-exports
 * `AdminTelemetryQuery` as an alias of {@link AdminTelemetryQueryParams}.
 */
export type AdminTelemetryQueryParams = {
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

export type AdminAuditEventsQueryParams = {
  before?: string;
  limit?: number;
};

export type AdminInfrastructureQueryParams = Pick<
  AdminTelemetryQueryParams,
  "from" | "to" | "limit"
> & {
  scope?: PathsV1AdminInfrastructureMetricsGetParametersQueryScope;
};

export const queryKeys = {
  bootstrapStatus: ["bootstrap-status"] as const,
  setupStatus: ["setup-status"] as const,
  setupPreflight: ["setup-preflight"] as const,
  setupCloudflareAccounts: ["setup-cloudflare-accounts"] as const,
  setupCloudflareZones: (accountId: string) =>
    ["setup-cloudflare-zones", accountId] as const,
  account: ["account"] as const,
  accountSessions: ["account-sessions"] as const,
  accountIdentities: ["account-identities"] as const,
  organizations: ["organizations"] as const,
  organization: (organizationId: string) =>
    ["organization", organizationId] as const,
  organizationPlan: (organizationId: string | undefined) =>
    ["organization-plan", organizationId] as const,
  memberships: (organizationId: string | undefined) =>
    ["memberships", organizationId] as const,
  invitations: (organizationId: string | undefined) =>
    ["invitations", organizationId] as const,
  projects: (organizationId: string) => ["projects", organizationId] as const,
  project: (projectId: string) => ["project", projectId] as const,
  usage: (projectId: string) => ["usage", projectId] as const,
  // Nested under `usage` on purpose: invalidating the snapshot must also
  // refresh the chart, which is derived from the same metering data.
  usageMetering: (projectId: string) =>
    ["usage", projectId, "metering"] as const,
  audit: (scope: string, id: string) => [scope, id, "audit"] as const,
  traces: (scope: string, id: string) => [scope, id, "traces"] as const,
  functions: (projectId: string) => ["functions", projectId] as const,
  function: (projectId: string, functionId: string) =>
    ["function", projectId, functionId] as const,
  functionDeployments: (projectId: string, functionId: string) =>
    ["function-deployments", projectId, functionId] as const,
  // Prefix form: invalidates every deployment list in a project when the
  // parent function id is not known.
  functionDeploymentsScope: (projectId: string) =>
    ["function-deployments", projectId] as const,
  functionExecutions: (projectId: string, functionId: string) =>
    ["function-executions", projectId, functionId] as const,
  functionExecution: (
    projectId: string,
    functionId: string,
    executionId: string,
  ) => ["function-execution", projectId, functionId, executionId] as const,
  functionVariables: (projectId: string, functionId: string) =>
    ["function-variables", projectId, functionId] as const,
  functionDeployment: (
    projectId: string,
    functionId: string,
    deploymentId: string,
  ) => ["function-deployment", projectId, functionId, deploymentId] as const,
  sites: (projectId: string) => ["sites", projectId] as const,
  site: (projectId: string, siteId: string) =>
    ["site", projectId, siteId] as const,
  apps: (projectId: string) => ["apps", projectId] as const,
  app: (projectId: string, appId: string) => ["app", projectId, appId] as const,
  appDiagnostics: (projectId: string, appId: string) =>
    ["app-diagnostics", projectId, appId] as const,
  appEnvironmentVariables: (projectId: string, appId: string) =>
    ["app-environment-variables", projectId, appId] as const,
  appDeployments: (projectId: string, appId: string) =>
    ["app-deployments", projectId, appId] as const,
  appDeployment: (projectId: string, appId: string, deploymentId: string) =>
    ["app-deployment", projectId, appId, deploymentId] as const,
  siteDeployments: (projectId: string, siteId: string) =>
    ["site-deployments", projectId, siteId] as const,
  // Prefix form: invalidates every deployment list in a project when the
  // parent site id is not known.
  siteDeploymentsScope: (projectId: string) =>
    ["site-deployments", projectId] as const,
  siteDeployment: (projectId: string, siteId: string, deploymentId: string) =>
    ["site-deployment", projectId, siteId, deploymentId] as const,
  siteDeploymentScope: (projectId: string, siteId: string) =>
    ["site-deployment", projectId, siteId] as const,
  // The build log and the file listing belong to one release, so they hang off
  // the deployment key: refreshing the release refreshes both.
  siteDeploymentLogs: (
    projectId: string,
    siteId: string,
    deploymentId: string,
  ) => ["site-deployment", projectId, siteId, deploymentId, "logs"] as const,
  siteDeploymentFiles: (
    projectId: string,
    siteId: string,
    deploymentId: string,
  ) => ["site-deployment", projectId, siteId, deploymentId, "files"] as const,
  databases: (projectId: string) => ["databases", projectId] as const,
  database: (projectId: string, databaseId: string) =>
    ["database", projectId, databaseId] as const,
  tables: (projectId: string, databaseId: string) =>
    ["tables", projectId, databaseId] as const,
  table: (projectId: string, databaseId: string, tableId: string) =>
    ["table", projectId, databaseId, tableId] as const,
  tableScope: (projectId: string, databaseId: string) =>
    ["table", projectId, databaseId] as const,
  columns: (projectId: string, databaseId: string, tableId: string) =>
    ["columns", projectId, databaseId, tableId] as const,
  columnsScope: (projectId: string, databaseId: string) =>
    ["columns", projectId, databaseId] as const,
  indexes: (projectId: string, databaseId: string, tableId: string) =>
    ["indexes", projectId, databaseId, tableId] as const,
  indexesScope: (projectId: string, databaseId: string) =>
    ["indexes", projectId, databaseId] as const,
  row: (
    projectId: string,
    databaseId: string,
    tableId: string,
    rowId: string,
  ) => ["row", projectId, databaseId, tableId, rowId] as const,
  rowScope: (projectId: string, databaseId: string, tableId: string) =>
    ["row", projectId, databaseId, tableId] as const,
  rowDatabaseScope: (projectId: string, databaseId: string) =>
    ["row", projectId, databaseId] as const,
  databaseBackups: (projectId: string, databaseId: string) =>
    ["database-backups", projectId, databaseId] as const,
  rows: (projectId: string, databaseId: string, tableId: string) =>
    ["rows", projectId, databaseId, tableId] as const,
  rowsScope: (projectId: string, databaseId: string) =>
    ["rows", projectId, databaseId] as const,
  buckets: (projectId: string) => ["storage", projectId] as const,
  bucket: (projectId: string, bucketId: string) =>
    ["bucket", projectId, bucketId] as const,
  files: (projectId: string, bucketId: string) =>
    ["files", projectId, bucketId] as const,
  file: (projectId: string, bucketId: string, fileId: string) =>
    ["file", projectId, bucketId, fileId] as const,
  fileScope: (projectId: string, bucketId: string) =>
    ["file", projectId, bucketId] as const,
  users: (projectId: string) => ["users", projectId] as const,
  projectUser: (projectId: string, userId: string) =>
    ["project-user", projectId, userId] as const,
  apiKeys: (projectId: string) => ["api-keys", projectId] as const,
  apiKey: (projectId: string, keyId: string) =>
    ["api-key", projectId, keyId] as const,
  webhooks: (projectId: string) => ["webhooks", projectId] as const,
  webhook: (projectId: string, webhookId: string) =>
    ["webhook", projectId, webhookId] as const,
  webhookDeliveries: (projectId: string, webhookId: string) =>
    ["webhook-deliveries", projectId, webhookId] as const,
  agents: (projectId: string) => ["agents", projectId] as const,
  agent: (agentId: string) => ["agent", agentId] as const,
  agentCatalog: ["agent-catalog"] as const,
  agentRuns: (agentId: string) => ["agent-runs", agentId] as const,
  agentRun: (agentId: string, runId: string) =>
    ["agent-run", agentId, runId] as const,
  messagingProviders: (projectId: string) =>
    ["messaging-providers", projectId] as const,
  messagingTopics: (projectId: string) =>
    ["messaging-topics", projectId] as const,
  messagingMessages: (projectId: string) =>
    ["messaging-messages", projectId] as const,
  authSettings: (projectId: string) => ["auth-settings", projectId] as const,
  serviceLayout: (projectId: string) => ["service-layout", projectId] as const,
  adminOverview: ["admin", "overview"] as const,
  adminOperations: ["admin", "operations"] as const,
  adminOAuthProviders: ["admin", "oauth-providers"] as const,
  adminAuditEventsScope: ["admin", "audit-events"] as const,
  adminAuditEvents: (query: AdminAuditEventsQueryParams) =>
    ["admin", "audit-events", query] as const,
  adminMonitors: ["admin", "monitors"] as const,
  adminMonitor: (monitorId: string) => ["admin", "monitor", monitorId] as const,
  adminAlerts: ["admin", "alerts"] as const,
  adminAlert: (alertRuleId: string) => ["admin", "alert", alertRuleId] as const,
  adminAlertEvents: ["admin", "alert-events"] as const,
  adminNotifications: ["admin", "notifications"] as const,
  adminIncidents: ["admin", "incidents"] as const,
  adminIncident: (incidentId: string) =>
    ["admin", "incident", incidentId] as const,
  adminDashboards: ["admin", "dashboards"] as const,
  adminDashboard: (dashboardId: string) =>
    ["admin", "dashboard", dashboardId] as const,
  adminStatusPage: ["admin", "status-page"] as const,
  adminHostMetrics: ["admin", "host-metrics"] as const,
  adminDBMetrics: ["admin", "db-metrics"] as const,
  adminRedisMetrics: ["admin", "redis-metrics"] as const,
  adminAPIMetrics: ["admin", "api-metrics"] as const,
  adminAccounts: ["admin", "accounts"] as const,
  publicStatusPage: ["public", "status-page"] as const,
  adminTelemetrySignal: (signal: string) =>
    ["admin", "telemetry", signal] as const,
  adminTelemetry: (signal: string, query: AdminTelemetryQueryParams) =>
    ["admin", "telemetry", signal, query] as const,
  adminInfrastructure: (query: AdminInfrastructureQueryParams) =>
    ["admin", "infrastructure", query] as const,
} as const;
