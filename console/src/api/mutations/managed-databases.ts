"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/api/client";
import { invalidate } from "@/api/cache-coherence";
import type { components } from "@/api/generated/schema";

/**
 * Hosted MySQL writes.
 *
 * Create and delete change the list; revealing or rotating a password changes
 * no list state, so those calls deliberately do not invalidate anything. The
 * returned password is held by the caller and never cached by React Query.
 */
export function useCreateManagedDatabase(projectId: string | undefined) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (
      body: components["schemas"]["CreateManagedDatabaseRequest"],
    ) =>
      unwrap(
        await api.POST("/v1/projects/{projectID}/managed-databases", {
          params: { path: { projectID: projectId as string } },
          body,
        }),
      ),
    onSuccess: async () => {
      await invalidate(queryClient, [
        { kind: "managed-database", projectId: projectId as string },
      ]);
    },
  });
}

export function useDeleteManagedDatabase(projectId: string | undefined) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (databaseId: string) =>
      unwrap(
        await api.DELETE(
          "/v1/projects/{projectID}/managed-databases/{databaseID}",
          {
            params: {
              path: { projectID: projectId as string, databaseID: databaseId },
            },
          },
        ),
      ),
    onSuccess: async () => {
      await invalidate(queryClient, [
        { kind: "managed-database", projectId: projectId as string },
      ]);
    },
  });
}

export function useRevealManagedDatabasePassword(
  projectId: string | undefined,
) {
  return useMutation({
    mutationFn: async (databaseId: string) =>
      unwrap(
        await api.POST(
          "/v1/projects/{projectID}/managed-databases/{databaseID}/password",
          {
            params: {
              path: { projectID: projectId as string, databaseID: databaseId },
            },
          },
        ),
      ),
  });
}

export function useRotateManagedDatabasePassword(
  projectId: string | undefined,
) {
  return useMutation({
    mutationFn: async (databaseId: string) =>
      unwrap(
        await api.POST(
          "/v1/projects/{projectID}/managed-databases/{databaseID}/password/rotate",
          {
            params: {
              path: { projectID: projectId as string, databaseID: databaseId },
            },
          },
        ),
      ),
  });
}
