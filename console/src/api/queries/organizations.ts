"use client";

import {
  useInfiniteQuery,
  useQuery,
  type InfiniteData,
} from "@tanstack/react-query";
import { api, cancellableQuery } from "@/api/client";
import type { components } from "@/api/generated/schema";
import { queryKeys } from "@/api/query-keys";

/** One page of organizations. The API paginates with a cursor. */
const ORGANIZATIONS_LIMIT = 50;

/** One page of projects for the selected organization. */
const PROJECTS_LIMIT = 50;

/** One page of members for the selected organization. */
const MEMBERSHIPS_LIMIT = 50;

/**
 * Organizations the authenticated account can see, read page by page.
 *
 * This is the only list the user console can build a landing view from: the API
 * has no top-level project collection, so projects are always read through an
 * organization. It pages like the project list does — an account in more than
 * fifty organizations could not otherwise reach the rest of them.
 */
export function useOrganizations() {
  return useInfiniteQuery({
    queryKey: queryKeys.organizations,
    initialPageParam: undefined as string | undefined,
    queryFn: (context) => {
      const fetchPage = cancellableQuery((signal) =>
        api.GET("/v1/organizations", {
          params: {
            query: {
              limit: ORGANIZATIONS_LIMIT,
              cursor: context.pageParam,
            },
            signal,
          },
        }),
      );
      return fetchPage(context);
    },
    getNextPageParam: (last) => last?.pagination.next_cursor,
  });
}

/** Every organization loaded so far, in the API's order. */
export function organizationItems(
  data:
    | InfiniteData<components["schemas"]["OrganizationsPage"] | undefined>
    | undefined,
): components["schemas"]["Organization"][] {
  return data?.pages.flatMap((page) => page?.organizations ?? []) ?? [];
}

/**
 * Projects scoped to one organization, read page by page.
 *
 * The first page is large because an organization is browsed rather than paged
 * through, but the cursor is kept: asking for one page and dropping
 * `next_cursor` made every project past the fiftieth disappear without a trace.
 * `can_manage` rides on each page because it describes the caller, not the page.
 * Disabled until an organization is known, so the view never requests a list it
 * cannot attribute.
 */
export function useOrganizationProjects(organizationId: string | undefined) {
  return useInfiniteQuery({
    queryKey: queryKeys.projects(organizationId ?? ""),
    enabled: Boolean(organizationId),
    initialPageParam: undefined as string | undefined,
    queryFn: (context) => {
      const fetchPage = cancellableQuery((signal) =>
        api.GET("/v1/organizations/{organizationID}/projects", {
          params: {
            path: { organizationID: organizationId as string },
            query: { limit: PROJECTS_LIMIT, cursor: context.pageParam },
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
 * The organization's plan and live usage. Drives the plan badge and the real
 * usage bar, so neither is a hardcoded label.
 */
export function useOrganizationPlan(organizationId: string | undefined) {
  return useQuery({
    queryKey: queryKeys.organizationPlan(organizationId),
    enabled: Boolean(organizationId),
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/organizations/{organizationID}/plan", {
        params: { path: { organizationID: organizationId as string }, signal },
      }),
    ),
  });
}

/**
 * Members of one organization. Used for a real member count on the landing
 * view; the list itself belongs to the organization settings surface.
 */
export function useOrganizationMemberships(organizationId: string | undefined) {
  return useQuery({
    queryKey: queryKeys.memberships(organizationId ?? ""),
    enabled: Boolean(organizationId),
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/organizations/{organizationID}/memberships", {
        params: {
          path: { organizationID: organizationId as string },
          query: { limit: MEMBERSHIPS_LIMIT },
          signal,
        },
      }),
    ),
  });
}
