"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/api/client";
import { applyCacheChanges } from "@/api/cache-coherence";
import { queryKeys } from "@/api/query-keys";
import type { components } from "@/api/generated/schema";

/**
 * The Functions section's writes.
 *
 * Creating a function writes metadata only: a function serves nothing until a
 * deployment is uploaded, and this console has no deployment upload for
 * functions yet, so the section says so rather than implying the function is
 * live. Both writes refresh the usage snapshot, which counts functions.
 */

export function useCreateFunction(projectId: string | undefined) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: components["schemas"]["CreateFunctionRequest"]) =>
      unwrap(
        await api.POST("/v1/projects/{projectID}/functions", {
          params: { path: { projectID: projectId as string } },
          body,
        }),
      ),
    onSuccess: async () => {
      await applyCacheChanges(queryClient, [
        { kind: "function", projectId: projectId as string },
      ]);
      await queryClient.invalidateQueries({
        queryKey: queryKeys.usage(projectId as string),
      });
    },
  });
}

export function useDeleteFunction(projectId: string | undefined) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (functionId: string) =>
      unwrap(
        await api.DELETE("/v1/projects/{projectID}/functions/{functionID}", {
          params: {
            path: { projectID: projectId as string, functionID: functionId },
          },
        }),
      ),
    onSuccess: async (_result, functionId) => {
      await applyCacheChanges(queryClient, [
        { kind: "function", projectId: projectId as string, functionId },
      ]);
      await queryClient.invalidateQueries({
        queryKey: queryKeys.usage(projectId as string),
      });
    },
  });
}
