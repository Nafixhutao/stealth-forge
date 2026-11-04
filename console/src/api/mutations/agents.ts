"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/api/client";
import { invalidate } from "@/api/cache-coherence";
import type { components } from "@/api/generated/schema";

/**
 * Queues a prompt for an agent.
 *
 * The API only accepts the run into its durable queue; a trusted worker owns
 * execution. So this resolves as soon as the run exists, and the page renders
 * the returned status instead of pretending an answer is on its way.
 */
export function useCreateAgentRun(agentId: string | undefined) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (prompt: string) =>
      unwrap(
        await api.POST("/v1/agents/{agentID}/runs", {
          params: { path: { agentID: agentId as string } },
          body: {
            prompt,
          } satisfies components["schemas"]["CreateAgentRunRequest"],
        }),
      ),
    onSuccess: () => invalidate(queryClient, [{ kind: "agent-run", agentId }]),
  });
}
