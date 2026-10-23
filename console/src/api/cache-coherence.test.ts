import { describe, expect, it, vi } from "vitest";
import {
  adminInvalidationKeysFor,
  invalidate,
  invalidationKeysFor,
  planCacheChange,
} from "@/api/cache-coherence";
import { queryKeys } from "@/api/query-keys";

describe("cache coherence", () => {
  it("maps an Agent run change to its list and detail keys", () => {
    expect(
      invalidationKeysFor({
        kind: "agent-run",
        projectId: "project-1",
        agentId: "agent-1",
        runId: "run-1",
      }),
    ).toEqual([
      ["agent-runs", "agent-1"],
      ["agent-run", "agent-1", "run-1"],
    ]);
  });

  it("maps a function deployment change to the affected resource keys", () => {
    expect(
      invalidationKeysFor({
        kind: "function-deployment",
        projectId: "project-1",
        functionId: "function-1",
        deploymentId: "deployment-1",
      }),
    ).toEqual([
      ["function", "project-1", "function-1"],
      ["function-deployments", "project-1", "function-1"],
      ["function-deployment", "project-1", "function-1", "deployment-1"],
    ]);
  });

  it("invalidates every deployment list when the parent function id is missing", () => {
    expect(
      invalidationKeysFor({
        kind: "function-deployment",
        projectId: "project-1",
        deploymentId: "deployment-1",
      }),
    ).toEqual([["function-deployments", "project-1"]]);
  });

  it("invalidates every deployment list when the parent site id is missing", () => {
    expect(
      invalidationKeysFor({
        kind: "site-deployment",
        projectId: "project-1",
        deploymentId: "deployment-1",
      }),
    ).toEqual([["site-deployments", "project-1"]]);
  });

  it("maps App desired-state changes to the list and detail queries", () => {
    expect(
      invalidationKeysFor({
        kind: "app",
        projectId: "project-1",
        appId: "app-1",
      }),
    ).toEqual([
      ["apps", "project-1"],
      ["app", "project-1", "app-1"],
      ["app-diagnostics", "project-1", "app-1"],
    ]);
  });

  it("refreshes App state and variable metadata after an environment change", () => {
    expect(
      invalidationKeysFor({
        kind: "app-environment-variable",
        projectId: "project-1",
        appId: "app-1",
      }),
    ).toEqual([
      ["app", "project-1", "app-1"],
      ["app-diagnostics", "project-1", "app-1"],
      ["app-environment-variables", "project-1", "app-1"],
    ]);
  });

  it("refreshes diagnostics after a deployment selection or rollback", () => {
    expect(
      invalidationKeysFor({
        kind: "app-deployment",
        projectId: "project-1",
        appId: "app-1",
        deploymentId: "deployment-3",
      }),
    ).toEqual([
      ["apps", "project-1"],
      ["app", "project-1", "app-1"],
      ["app-diagnostics", "project-1", "app-1"],
      ["app-deployments", "project-1", "app-1"],
      ["app-deployment", "project-1", "app-1", "deployment-3"],
    ]);
  });

  it("refreshes the parent Agent when a run can change its derived status", () => {
    expect(
      invalidationKeysFor({
        kind: "agent-run",
        projectId: "project-1",
        agentId: "agent-1",
        includeAgent: true,
        includeAgentDetail: true,
      }),
    ).toEqual([
      ["agent-runs", "agent-1"],
      ["agents", "project-1"],
      ["agent", "agent-1"],
    ]);
  });

  it("maps an authenticated account refresh to all affected global keys", () => {
    expect(
      invalidationKeysFor({
        kind: "account",
        includeOrganizations: true,
        includeBootstrapStatus: true,
      }),
    ).toEqual([["account"], ["organizations"], ["bootstrap-status"]]);
  });

  it("keeps database restore invalidation policy in one module", () => {
    expect(
      invalidationKeysFor({
        kind: "database-backup",
        projectId: "project-1",
        databaseId: "database-1",
        restore: true,
      }),
    ).toEqual([
      ["rows", "project-1", "database-1"],
      ["row", "project-1", "database-1"],
      ["table", "project-1", "database-1"],
      ["columns", "project-1", "database-1"],
      ["indexes", "project-1", "database-1"],
      ["database", "project-1", "database-1"],
      ["tables", "project-1", "database-1"],
      ["database-backups", "project-1", "database-1"],
    ]);
  });

  it("refreshes indexes when a table schema changes", () => {
    expect(
      invalidationKeysFor({
        kind: "database-table-schema",
        projectId: "project-1",
        databaseId: "database-1",
        tableId: "table-1",
      }),
    ).toEqual([
      ["columns", "project-1", "database-1", "table-1"],
      ["rows", "project-1", "database-1", "table-1"],
      ["row", "project-1", "database-1", "table-1"],
      ["indexes", "project-1", "database-1", "table-1"],
    ]);
  });

  it("maps storage file changes without leaking query keys to callers", () => {
    expect(
      invalidationKeysFor({
        kind: "storage-file",
        projectId: "project-1",
        bucketId: "bucket-1",
        operation: "rename",
      }),
    ).toEqual([
      ["files", "project-1", "bucket-1"],
      ["file", "project-1", "bucket-1"],
    ]);
  });

  it("applies every unique invalidation key exactly once", async () => {
    const invalidateQueries = vi.fn().mockResolvedValue(undefined);
    const removeQueries = vi.fn().mockResolvedValue(undefined);
    const queryClient = { invalidateQueries, removeQueries };
    const change = {
      kind: "database" as const,
      projectId: "project-1",
    };

    await invalidate(queryClient, [change, change]);

    expect(invalidateQueries).toHaveBeenCalledOnce();
    expect(invalidateQueries).toHaveBeenCalledWith({
      queryKey: ["databases", "project-1"],
    });
    expect(removeQueries).not.toHaveBeenCalled();
  });

  it("forgets a deleted resource instead of refetching it", async () => {
    const invalidateQueries = vi.fn().mockResolvedValue(undefined);
    const removeQueries = vi.fn().mockResolvedValue(undefined);
    const queryClient = { invalidateQueries, removeQueries };

    await invalidate(queryClient, [
      { kind: "admin-monitor", resourceId: "monitor-1", forget: true },
    ]);

    // The detail query is dropped; the list and audit keys are refreshed.
    expect(removeQueries).toHaveBeenCalledOnce();
    expect(removeQueries).toHaveBeenCalledWith({
      queryKey: ["admin", "monitor", "monitor-1"],
    });
    expect(invalidateQueries).toHaveBeenCalledTimes(2);
    expect(invalidateQueries).toHaveBeenCalledWith({
      queryKey: ["admin", "audit-events"],
    });
    expect(invalidateQueries).toHaveBeenCalledWith({
      queryKey: ["admin", "monitors"],
    });
  });

  it("never both forgets and refetches the same key", async () => {
    const invalidateQueries = vi.fn().mockResolvedValue(undefined);
    const removeQueries = vi.fn().mockResolvedValue(undefined);
    const queryClient = { invalidateQueries, removeQueries };

    await invalidate(queryClient, [
      { kind: "admin-alert", resourceId: "alert-1", forget: true },
    ]);

    const refreshed = invalidateQueries.mock.calls.map(
      ([call]) => JSON.stringify(call.queryKey),
    );
    expect(refreshed).not.toContain(JSON.stringify(["admin", "alert", "alert-1"]));
    expect(removeQueries).toHaveBeenCalledWith({
      queryKey: ["admin", "alert", "alert-1"],
    });
  });

  it("forgets the account when a password reset discards it", async () => {
    const invalidateQueries = vi.fn().mockResolvedValue(undefined);
    const removeQueries = vi.fn().mockResolvedValue(undefined);
    const queryClient = { invalidateQueries, removeQueries };

    await invalidate(queryClient, [{ kind: "account", forget: true }]);

    expect(removeQueries).toHaveBeenCalledWith({ queryKey: ["account"] });
    expect(invalidateQueries).not.toHaveBeenCalled();
  });

  it("refreshes the usage snapshot through the policy, not a raw key", async () => {
    const invalidateQueries = vi.fn().mockResolvedValue(undefined);
    const removeQueries = vi.fn().mockResolvedValue(undefined);
    const queryClient = { invalidateQueries, removeQueries };

    await invalidate(queryClient, [
      { kind: "site", projectId: "project-1", siteId: "site-1" },
      { kind: "usage", projectId: "project-1" },
    ]);

    expect(invalidateQueries).toHaveBeenCalledWith({
      queryKey: ["usage", "project-1"],
    });
  });

  it("resolves both policies through one plan", () => {
    const admin = planCacheChange({ kind: "admin-account" });
    expect(admin.forget).toEqual([]);
    expect(admin.invalidate).toEqual([
      ["admin", "audit-events"],
      ["admin", "accounts"],
    ]);

    const project = planCacheChange({
      kind: "project",
      projectId: "project-1",
    });
    expect(project.forget).toEqual([]);
    expect(project.invalidate).toEqual([["project", "project-1"]]);
  });

  it("invalidates the project key alone, since it already covers its audit key", () => {
    // `["project", id]` is a prefix of `["project", id, "audit"]`, so TanStack
    // Query's prefix matching refreshes the audit list from one key.
    expect(
      invalidationKeysFor({ kind: "project", projectId: "project-1" }),
    ).toEqual([["project", "project-1"]]);
  });

  it("maps an Admin monitor change to the audit scope and monitor keys", () => {
    expect(
      adminInvalidationKeysFor({
        kind: "admin-monitor",
        resourceId: "monitor-1",
      }),
    ).toEqual([
      ["admin", "audit-events"],
      ["admin", "monitors"],
      ["admin", "monitor", "monitor-1"],
    ]);
  });

  it("maps an Admin error-group change to the telemetry error prefix", () => {
    expect(adminInvalidationKeysFor({ kind: "admin-error-group" })).toEqual([
      ["admin", "audit-events"],
      ["admin", "telemetry", "errors"],
    ]);
  });

  it("keeps Admin scope keys aligned with the full key they prefix", () => {
    expect(queryKeys.adminAuditEventsScope).toEqual(
      queryKeys.adminAuditEvents({}).slice(0, 2),
    );
    expect(queryKeys.adminTelemetrySignal("errors")).toEqual(
      queryKeys.adminTelemetry("errors", {}).slice(0, 3),
    );
  });

  it("keeps deployment scope keys aligned with the full key they prefix", () => {
    expect(queryKeys.functionDeploymentsScope("project-1")).toEqual(
      queryKeys.functionDeployments("project-1", "function-1").slice(0, 2),
    );
    expect(queryKeys.siteDeploymentsScope("project-1")).toEqual(
      queryKeys.siteDeployments("project-1", "site-1").slice(0, 2),
    );
  });
});
