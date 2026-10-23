"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/api/client";
import { invalidate } from "@/api/cache-coherence";
import type { components } from "@/api/generated/schema";

/**
 * The Databases section's writes.
 *
 * Deleting a database drops its tables, indexes, and rows with it, so the
 * dialog that calls this asks the operator to name what they are deleting
 * first. Both writes refresh the usage snapshot as well: the section header
 * counts databases, tables, and rows from it, not from this list.
 */

export function useCreateDatabase(projectId: string | undefined) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: components["schemas"]["CreateDatabaseRequest"]) =>
      unwrap(
        await api.POST("/v1/projects/{projectID}/databases", {
          params: { path: { projectID: projectId as string } },
          body,
        }),
      ),
    onSuccess: async () => {
      await invalidate(queryClient, [
        { kind: "database", projectId: projectId as string },
      ]);
      await invalidate(queryClient, [
        { kind: "usage", projectId: projectId as string },
      ]);
    },
  });
}

export function useDeleteDatabase(projectId: string | undefined) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (databaseId: string) =>
      unwrap(
        await api.DELETE("/v1/projects/{projectID}/databases/{databaseID}", {
          params: {
            path: { projectID: projectId as string, databaseID: databaseId },
          },
        }),
      ),
    onSuccess: async (_result, databaseId) => {
      await invalidate(queryClient, [
        {
          kind: "database",
          projectId: projectId as string,
          databaseId,
        },
      ]);
      await invalidate(queryClient, [
        { kind: "usage", projectId: projectId as string },
      ]);
    },
  });
}
