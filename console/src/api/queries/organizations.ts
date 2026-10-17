"use client";

import { useQuery } from "@tanstack/react-query";
import { api, cancellableQuery } from "@/api/client";
import { queryKeys } from "@/api/query-keys";

/** One page of organizations. The API paginates with a cursor. */
const ORGANIZATIONS_LIMIT = 50;

/** One page of projects for the selected organization. */
const PROJECTS_LIMIT = 50;

/** One page of members for the selected organization. */
const MEMBERSHIPS_LIMIT = 50;

/**
 * Organizations the authenticated account can see. This is the only list the
 * user console can build a landing view from: the API has no top-level project
 * collection, so projects are always read through an organization.
 */
export function useOrganizations() {
  return useQuery({
    queryKey: queryKeys.organizations,
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/organizations", {
        params: { query: { limit: ORGANIZATIONS_LIMIT } },
        signal,
      }),
    ),
  });
}

/**
 * Projects scoped to one organization. Disabled until an organization is
 * known, so the view never requests a project list it cannot attribute.
 */
export function useOrganizationProjects(organizationId: string | undefined) {
  return useQuery({
    queryKey: queryKeys.projects(organizationId ?? ""),
    enabled: Boolean(organizationId),
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/organizations/{organizationID}/projects", {
        params: {
          path: { organizationID: organizationId as string },
          query: { limit: PROJECTS_LIMIT },
          signal,
        },
      }),
    ),
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
