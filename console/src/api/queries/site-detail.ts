"use client";

import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { api, cancellableQuery } from "@/api/client";
import { queryKeys } from "@/api/query-keys";

/**
 * The Site detail surface's reads.
 *
 * A Site is three things at once: its own metadata, the releases published to it,
 * and the files inside one release. They are separate reads because they change
 * at different rates — a build moves the release list without touching the site
 * row, and the file list only exists once a build succeeds.
 */

/** The release history is browsed, not paged through; one large page suffices. */
const DEPLOYMENTS_PAGE_SIZE = 50;

export function useSite(projectId: string | undefined, siteId: string | undefined) {
  return useQuery({
    queryKey: queryKeys.site(projectId ?? "", siteId ?? ""),
    enabled: Boolean(projectId && siteId),
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/projects/{projectID}/sites/{siteID}", {
        params: {
          path: { projectID: projectId as string, siteID: siteId as string },
          signal,
        },
      }),
    ),
  });
}

export function useSiteDeployments(
  projectId: string | undefined,
  siteId: string | undefined,
) {
  return useInfiniteQuery({
    queryKey: queryKeys.siteDeployments(projectId ?? "", siteId ?? ""),
    enabled: Boolean(projectId && siteId),
    initialPageParam: undefined as string | undefined,
    queryFn: (context) => {
      const fetchPage = cancellableQuery((signal) =>
        api.GET("/v1/projects/{projectID}/sites/{siteID}/deployments", {
          params: {
            path: { projectID: projectId as string, siteID: siteId as string },
            query: { limit: DEPLOYMENTS_PAGE_SIZE, cursor: context.pageParam },
            signal,
          },
        }),
      );
      return fetchPage(context);
    },
    getNextPageParam: (last) => last?.pagination.next_cursor,
  });
}

/**
 * The build log of one release.
 *
 * A release that is still building gains lines, so this polls while its
 * `build_status` is queued or running. `active` is the caller's signal that
 * polling is worth the requests: a finished release is read once.
 */
export function useSiteDeploymentLogs(
  projectId: string | undefined,
  siteId: string | undefined,
  deploymentId: string | undefined,
  active: boolean,
) {
  return useQuery({
    queryKey: queryKeys.siteDeploymentLogs(
      projectId ?? "",
      siteId ?? "",
      deploymentId ?? "",
    ),
    enabled: Boolean(projectId && siteId && deploymentId),
    refetchInterval: active ? 2000 : false,
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/projects/{projectID}/sites/{siteID}/deployments/{deploymentID}/logs", {
        params: {
          path: {
            projectID: projectId as string,
            siteID: siteId as string,
            deploymentID: deploymentId as string,
          },
          query: { limit: 200 },
          signal,
        },
      }),
    ),
  });
}

/**
 * The files inside one published release.
 *
 * The listing answers 409 while the release is still building, so the caller
 * only enables this once a deployment has succeeded.
 */
export function useSiteDeploymentFiles(
  projectId: string | undefined,
  siteId: string | undefined,
  deploymentId: string | undefined,
  enabled: boolean,
) {
  return useQuery({
    queryKey: queryKeys.siteDeploymentFiles(
      projectId ?? "",
      siteId ?? "",
      deploymentId ?? "",
    ),
    enabled: Boolean(projectId && siteId && deploymentId) && enabled,
    queryFn: cancellableQuery((signal) =>
      api.GET(
        "/v1/projects/{projectID}/sites/{siteID}/deployments/{deploymentID}/files",
        {
          params: {
            path: {
              projectID: projectId as string,
              siteID: siteId as string,
              deploymentID: deploymentId as string,
            },
            signal,
          },
        },
      ),
    ),
  });
}
