"use client";

import { useInfiniteQuery, useQueries, useQuery } from "@tanstack/react-query";
import { useMemo } from "react";
import { api, cancellableQuery } from "@/api/client";
import { queryKeys } from "@/api/query-keys";

/**
 * The project surface's reads.
 *
 * Every read here is scoped by the API itself: the repository resolves the
 * caller's organization role for the project and answers 404 for a project the
 * account cannot see, so the pages need one not-found state rather than a
 * separate permission probe. Resource lists additionally return `can_manage`,
 * which is what gates a create or delete control.
 */

/** One project by id. Identity for the header, and the page's 404 path. */
export function useProject(projectId: string | undefined) {
  return useQuery({
    queryKey: queryKeys.project(projectId ?? ""),
    enabled: Boolean(projectId),
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/projects/{projectID}", {
        params: { path: { projectID: projectId as string } },
        signal,
      }),
    ),
  });
}

/**
 * The project's usage snapshot: resource counts, storage and artifact bytes,
 * quotas, and the rolling 30-day counters the Overview summarises. Kept apart
 * from the project itself because the snapshot is recomputed on read.
 */
export function useProjectUsage(projectId: string | undefined) {
  return useQuery({
    queryKey: queryKeys.usage(projectId ?? ""),
    enabled: Boolean(projectId),
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/projects/{projectID}/usage", {
        params: { path: { projectID: projectId as string } },
        signal,
      }),
    ),
  });
}

/**
 * The 30-day request count for each project on screen, keyed by project id.
 *
 * The organization home lists projects but the list itself carries no usage, so
 * the number has to come from each project's snapshot. One query per project is
 * the only shape the API offers; they share the `usage` key with the project
 * page, so opening a project reuses what the card already fetched instead of
 * asking again. A project whose read fails is reported as `undefined` rather
 * than as zero, because "no data" and "no traffic" are different claims.
 */
export function useProjectRequestCounts(projectIds: readonly string[]) {
  const results = useQueries({
    queries: projectIds.map((projectId) => ({
      queryKey: queryKeys.usage(projectId),
      queryFn: cancellableQuery((signal) =>
        api.GET("/v1/projects/{projectID}/usage", {
          params: { path: { projectID: projectId }, signal },
        }),
      ),
    })),
  });

  return useMemo(() => {
    const counts = new Map<string, number | undefined>();
    projectIds.forEach((projectId, index) => {
      const usage = results[index]?.data?.usage;
      counts.set(projectId, usage?.api_request_count_30d);
    });
    return counts;
  }, [projectIds, results]);
}

/**
 * The daily metering series behind the Overview chart. The API defaults to the
 * window the chart draws (30 days ending today), so no range is sent: the
 * default is the contract, not a client choice.
 *
 * The operation answers JSON or CSV, which the generated client types as one
 * union; the request above asks for JSON, so the CSV arm is dropped here rather
 * than left for every caller to narrow.
 */
export function useProjectUsageMetering(projectId: string | undefined) {
  return useQuery({
    queryKey: queryKeys.usageMetering(projectId ?? ""),
    enabled: Boolean(projectId),
    queryFn: async (context) => {
      const fetchMetering = cancellableQuery((signal) =>
        api.GET("/v1/projects/{projectID}/usage/metering", {
          params: { path: { projectID: projectId as string } },
          signal,
        }),
      );
      const payload = await fetchMetering(context);
      // The endpoint also answers CSV, which the generated client folds into
      // the same union as the JSON payload. This request always asks for JSON,
      // so the CSV arm is dropped once here instead of by every consumer.
      return typeof payload === "string" ? undefined : payload?.metering;
    },
  });
}

/**
 * A section list, read page by page.
 *
 * The console asks for a large first page — a section is browsed, not paged
 * through — and offers the rest only when the API says there is more, so a
 * project with more resources than one page never silently looks complete.
 * `can_manage` rides on the first page: it describes the caller's role, not the
 * page.
 */
const RESOURCE_PAGE_SIZE = 100;

export function useProjectDatabases(projectId: string | undefined) {
  return useInfiniteQuery({
    queryKey: queryKeys.databases(projectId ?? ""),
    enabled: Boolean(projectId),
    initialPageParam: undefined as string | undefined,
    queryFn: (context) => {
      const fetchPage = cancellableQuery((signal) =>
        api.GET("/v1/projects/{projectID}/databases", {
          params: {
            path: { projectID: projectId as string },
            query: { limit: RESOURCE_PAGE_SIZE, cursor: context.pageParam },
          },
          signal,
        }),
      );
      return fetchPage(context);
    },
    getNextPageParam: (last) => last?.pagination.next_cursor,
  });
}

export function useProjectBuckets(projectId: string | undefined) {
  return useInfiniteQuery({
    queryKey: queryKeys.buckets(projectId ?? ""),
    enabled: Boolean(projectId),
    initialPageParam: undefined as string | undefined,
    queryFn: (context) => {
      const fetchPage = cancellableQuery((signal) =>
        api.GET("/v1/projects/{projectID}/storage/buckets", {
          params: {
            path: { projectID: projectId as string },
            query: { limit: RESOURCE_PAGE_SIZE, cursor: context.pageParam },
          },
          signal,
        }),
      );
      return fetchPage(context);
    },
    getNextPageParam: (last) => last?.pagination.next_cursor,
  });
}

export function useProjectFunctions(projectId: string | undefined) {
  return useInfiniteQuery({
    queryKey: queryKeys.functions(projectId ?? ""),
    enabled: Boolean(projectId),
    initialPageParam: undefined as string | undefined,
    queryFn: (context) => {
      const fetchPage = cancellableQuery((signal) =>
        api.GET("/v1/projects/{projectID}/functions", {
          params: {
            path: { projectID: projectId as string },
            query: { limit: RESOURCE_PAGE_SIZE, cursor: context.pageParam },
          },
          signal,
        }),
      );
      return fetchPage(context);
    },
    getNextPageParam: (last) => last?.pagination.next_cursor,
  });
}

export function useProjectSites(projectId: string | undefined) {
  return useInfiniteQuery({
    queryKey: queryKeys.sites(projectId ?? ""),
    enabled: Boolean(projectId),
    initialPageParam: undefined as string | undefined,
    queryFn: (context) => {
      const fetchPage = cancellableQuery((signal) =>
        api.GET("/v1/projects/{projectID}/sites", {
          params: {
            path: { projectID: projectId as string },
            query: { limit: RESOURCE_PAGE_SIZE, cursor: context.pageParam },
          },
          signal,
        }),
      );
      return fetchPage(context);
    },
    getNextPageParam: (last) => last?.pagination.next_cursor,
  });
}
