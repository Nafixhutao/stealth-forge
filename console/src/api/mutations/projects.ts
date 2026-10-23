"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/api/client";
import { invalidate } from "@/api/cache-coherence";
import type { components } from "@/api/generated/schema";

/**
 * Creates a project in an organization.
 *
 * Both the project list and the plan are refreshed through the shared
 * invalidation policy, so a new project cannot leave a stale usage bar behind.
 */
export function useCreateProject(organizationId: string | undefined) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: components["schemas"]["CreateProjectRequest"]) =>
      unwrap(
        await api.POST("/v1/organizations/{organizationID}/projects", {
          params: { path: { organizationID: organizationId as string } },
          body,
        }),
      ),
    onSuccess: () =>
      invalidate(queryClient, [
        {
          kind: "organization-projects",
          organizationId: organizationId as string,
        },
        { kind: "organization-plan", organizationId: organizationId as string },
      ]),
  });
}
