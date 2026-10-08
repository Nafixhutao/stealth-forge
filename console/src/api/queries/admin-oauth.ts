"use client";
import { useQuery } from "@tanstack/react-query";
import { api, cancellableQuery } from "@/api/client";
import { queryKeys } from "@/api/query-keys";

/**
 * External sign-in provider status for the Admin Console. The API never
 * returns client secrets, only the client id, callback URL, and source.
 */
export function useAdminOAuthProviders() {
  return useQuery({
    queryKey: queryKeys.adminOAuthProviders,
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/admin/oauth-providers", { signal }),
    ),
    retry: false,
    staleTime: 30_000,
  });
}
