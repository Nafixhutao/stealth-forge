"use client";

import { usePathname } from "next/navigation";
import { useProject } from "@/api/queries/projects";
import { useCurrentOrganizationId as useScopeOrganizationId } from "@/features/user/organization-scope";

/** `/projects/<uuid>` or any of its sections. */
const PROJECT_ROUTE = /^\/projects\/([0-9a-fA-F-]{36})(?:\/|$)/;

/**
 * The organization the current surface belongs to.
 *
 * On a project route that is the project's organization — the authoritative
 * answer, and already in the query cache because the project shell fetched the
 * same project. Everywhere else it is the organization the user console is
 * showing, which the home page publishes when its switcher moves.
 */
export function useCurrentOrganizationId(): string | null {
  const pathname = usePathname();
  const projectId = PROJECT_ROUTE.exec(pathname)?.[1];
  // Both hooks run on every route: a hook cannot be skipped on a path change.
  const project = useProject(projectId);
  const scopeOrganizationId = useScopeOrganizationId();

  if (!projectId) return scopeOrganizationId;
  return project.data?.project.organization_id ?? null;
}
