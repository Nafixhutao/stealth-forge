"use client";

import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { api, cancellableQuery } from "@/api/client";
import { queryKeys } from "@/api/query-keys";

/**
 * The database browser's reads.
 *
 * Management metadata (the database, its tables, and a table's columns) is
 * console-only and cursor-paginated, so the page asks for a large first page
 * and follows `next_cursor` only when the API says there is more. Rows use a
 * smaller page because the grid is scrolled, not read end to end.
 */

const METADATA_PAGE_SIZE = 100;
const ROW_PAGE_SIZE = 50;

export function useDatabase(
  projectId: string | undefined,
  databaseId: string | undefined,
) {
  return useQuery({
    queryKey: queryKeys.database(projectId ?? "", databaseId ?? ""),
    enabled: Boolean(projectId && databaseId),
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/projects/{projectID}/databases/{databaseID}", {
        params: {
          path: {
            projectID: projectId as string,
            databaseID: databaseId as string,
          },
        },
        signal,
      }),
    ),
  });
}

export function useDatabaseTables(
  projectId: string | undefined,
  databaseId: string | undefined,
) {
  return useInfiniteQuery({
    queryKey: queryKeys.tables(projectId ?? "", databaseId ?? ""),
    enabled: Boolean(projectId && databaseId),
    initialPageParam: undefined as string | undefined,
    queryFn: (context) => {
      const fetchPage = cancellableQuery((signal) =>
        api.GET("/v1/projects/{projectID}/databases/{databaseID}/tables", {
          params: {
            path: {
              projectID: projectId as string,
              databaseID: databaseId as string,
            },
            query: { limit: METADATA_PAGE_SIZE, cursor: context.pageParam },
          },
          signal,
        }),
      );
      return fetchPage(context);
    },
    getNextPageParam: (last) => last?.pagination.next_cursor,
  });
}

export function useDatabaseColumns(
  projectId: string | undefined,
  databaseId: string | undefined,
  tableId: string | undefined,
) {
  return useInfiniteQuery({
    queryKey: queryKeys.columns(
      projectId ?? "",
      databaseId ?? "",
      tableId ?? "",
    ),
    enabled: Boolean(projectId && databaseId && tableId),
    initialPageParam: undefined as string | undefined,
    queryFn: (context) => {
      const fetchPage = cancellableQuery((signal) =>
        api.GET(
          "/v1/projects/{projectID}/databases/{databaseID}/tables/{tableID}/columns",
          {
            params: {
              path: {
                projectID: projectId as string,
                databaseID: databaseId as string,
                tableID: tableId as string,
              },
              query: { limit: METADATA_PAGE_SIZE, cursor: context.pageParam },
            },
            signal,
          },
        ),
      );
      return fetchPage(context);
    },
    getNextPageParam: (last) => last?.pagination.next_cursor,
  });
}

export function useDatabaseRows(
  projectId: string | undefined,
  databaseId: string | undefined,
  tableId: string | undefined,
) {
  return useInfiniteQuery({
    queryKey: [
      ...queryKeys.rows(projectId ?? "", databaseId ?? "", tableId ?? ""),
      "page",
    ],
    enabled: Boolean(projectId && databaseId && tableId),
    initialPageParam: undefined as string | undefined,
    queryFn: (context) => {
      const fetchPage = cancellableQuery((signal) =>
        api.GET(
          "/v1/projects/{projectID}/databases/{databaseID}/tables/{tableID}/rows",
          {
            params: {
              path: {
                projectID: projectId as string,
                databaseID: databaseId as string,
                tableID: tableId as string,
              },
              query: { limit: ROW_PAGE_SIZE, cursor: context.pageParam },
            },
            signal,
          },
        ),
      );
      return fetchPage(context);
    },
    getNextPageParam: (last) => last?.pagination.next_cursor,
  });
}
