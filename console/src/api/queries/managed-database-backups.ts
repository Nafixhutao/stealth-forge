"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, execute, unwrap } from "@/api/client";
import { invalidate } from "@/api/cache-coherence";
import { queryKeys } from "@/api/query-keys";

/**
 * Completed snapshots plus the most recent restore for one database. Restore
 * progress is polled so a queued request visibly moves to succeeded/failed.
 */
export function useManagedDatabaseBackups(
  projectId: string | undefined,
  databaseId: string | undefined,
) {
  return useQuery({
    queryKey: queryKeys.managedDatabaseBackups(
      projectId ?? "",
      databaseId ?? "",
    ),
    enabled: Boolean(projectId && databaseId),
    queryFn: ({ signal }) =>
      execute(
        api.GET(
          "/v1/projects/{projectID}/managed-databases/{databaseID}/backups",
          {
            params: {
              path: {
                projectID: projectId as string,
                databaseID: databaseId as string,
              },
            },
            signal,
          },
        ),
      ),
    // A restore moves queued -> running -> succeeded asynchronously. Poll
    // while it is in a non-terminal state so the reply appears without reload.
    refetchInterval: (query) =>
      ["queued", "running"].includes(
        query.state.data?.latest_restore?.status ?? "",
      )
        ? 5_000
        : 30_000,
  });
}

export function useRestoreManagedDatabase(
  projectId: string | undefined,
  databaseId: string | undefined,
) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (backupId: string) =>
      unwrap(
        await api.POST(
          "/v1/projects/{projectID}/managed-databases/{databaseID}/backups/{backupID}/restore",
          {
            params: {
              path: {
                projectID: projectId as string,
                databaseID: databaseId as string,
                backupID: backupId,
              },
            },
          },
        ),
      ),
    onSuccess: async () => {
      await invalidate(queryClient, [
        {
          kind: "managed-database-backups",
          projectId: projectId as string,
          databaseId: databaseId as string,
        },
      ]);
    },
  });
}
