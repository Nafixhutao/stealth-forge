"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/api/client";
import { invalidate } from "@/api/cache-coherence";
import type { components } from "@/api/generated/schema";

/**
 * Creates an organization.
 *
 * The API makes the caller its owner and provisions the plan, so the only cache
 * that goes stale is the organization list. Invalidating it through the shared
 * policy means the switcher and the empty state both see the new record without
 * the caller naming a query key.
 */
export function useCreateOrganization() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (
      body: components["schemas"]["CreateOrganizationRequest"],
    ) =>
      unwrap(
        await api.POST("/v1/organizations", {
          body,
        }),
      ),
    onSuccess: () => invalidate(queryClient, [{ kind: "organizations" }]),
  });
}
