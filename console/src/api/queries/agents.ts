"use client";

import { useQuery } from "@tanstack/react-query";
import { api, cancellableQuery } from "@/api/client";
import type { components } from "@/api/generated/schema";
import { queryKeys } from "@/api/query-keys";

type AgentRunsPage = components["schemas"]["AgentRunsPage"];

/**
 * The Agent surface's reads.
 *
 * Every read is scoped by the API: an agent is visible only through a project
 * the account belongs to, and a run only through its agent. The page therefore
 * needs one empty state, not a permission probe.
 */

/** Runs in flight are the only state worth polling; a queued run waits on a
 * worker that this installation may not have, and the page says so instead of
 * re-asking forever. */
const RUN_POLL_MS = 4_000;

/** Every agent the signed-in account can reach, newest first. */
export function useAgents() {
  return useQuery({
    queryKey: queryKeys.allAgents,
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/agents", { params: { query: { limit: 50 } }, signal }),
    ),
  });
}

export function useAgentRuns(agentId: string | undefined) {
  return useQuery<AgentRunsPage | undefined>({
    queryKey: queryKeys.agentRuns(agentId ?? ""),
    enabled: Boolean(agentId),
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/agents/{agentID}/runs", {
        params: { path: { agentID: agentId as string }, query: { limit: 50 } },
        signal,
      }),
    ),
    // A run that is executing can finish without the user acting, so the
    // history is re-read while one is running and left alone otherwise.
    refetchInterval: (query) =>
      query.state.data?.runs?.some((run) => run.status === "running")
        ? RUN_POLL_MS
        : false,
    staleTime: RUN_POLL_MS,
  });
}

/** Providers, models, roles, tools, and whether this installation executes
 * runs or only queues them. */
export function useAgentCatalog() {
  return useQuery({
    queryKey: queryKeys.agentCatalog,
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/agent-catalog", { signal }),
    ),
    staleTime: 5 * 60_000,
  });
}
