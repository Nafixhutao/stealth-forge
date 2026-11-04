"use client";

import { useQuery } from "@tanstack/react-query";
import { api, cancellableQuery } from "@/api/client";
import { queryKeys } from "@/api/query-keys";

/**
 * The project's hosted MySQL databases.
 *
 * The free tier returns at most one row, so this is a plain query rather than a
 * cursor list. Credentials are not part of the response; they are fetched by a
 * separate audited reveal call.
 */
export function useManagedDatabases(projectId: string | undefined) {
  return useQuery({
    queryKey: queryKeys.managedDatabases(projectId ?? ""),
    enabled: Boolean(projectId),
    // Usage is read from MySQL on each request; a minute keeps the free-tier
    // cards current without turning the Console into a polling dashboard.
    refetchInterval: 60_000,
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/projects/{projectID}/managed-databases", {
        params: { path: { projectID: projectId as string } },
        signal,
      }),
    ),
  });
}
