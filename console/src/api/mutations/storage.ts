"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/api/client";
import { invalidate } from "@/api/cache-coherence";
import type { components } from "@/api/generated/schema";

/**
 * The Storage section's writes.
 *
 * A bucket holds files, so deleting one is irreversible in the same way a
 * database delete is; the dialog confirms by name. Both writes refresh the
 * usage snapshot, because the header counts files and bytes from it.
 */

export function useCreateBucket(projectId: string | undefined) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (
      body: components["schemas"]["CreateStorageBucketRequest"],
    ) =>
      unwrap(
        await api.POST("/v1/projects/{projectID}/storage/buckets", {
          params: { path: { projectID: projectId as string } },
          body,
        }),
      ),
    onSuccess: async () => {
      await invalidate(queryClient, [
        { kind: "storage-bucket", projectId: projectId as string },
      ]);
      await invalidate(queryClient, [
        { kind: "usage", projectId: projectId as string },
      ]);
    },
  });
}

export function useDeleteBucket(projectId: string | undefined) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (bucketId: string) =>
      unwrap(
        await api.DELETE(
          "/v1/projects/{projectID}/storage/buckets/{bucketID}",
          {
            params: {
              path: { projectID: projectId as string, bucketID: bucketId },
            },
          },
        ),
      ),
    onSuccess: async (_result, bucketId) => {
      await invalidate(queryClient, [
        { kind: "storage-bucket", projectId: projectId as string, bucketId },
      ]);
      await invalidate(queryClient, [
        { kind: "usage", projectId: projectId as string },
      ]);
    },
  });
}
