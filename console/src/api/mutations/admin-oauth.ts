"use client";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/api/client";
import { queryKeys } from "@/api/query-keys";
import type {
  components,
  PathsV1AdminOauthProvidersProviderDeleteParametersPathProvider,
  PathsV1AdminOauthProvidersProviderPutParametersPathProvider,
} from "@/api/generated/schema";

/** Provider names the instance supports for external sign-in. */
export type OAuthProviderName = "github" | "google";

/** Save one provider's client credentials. The secret is write-only. */
export function useUpdateAdminOAuthProvider() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      provider,
      body,
    }: {
      provider: OAuthProviderName;
      body: components["schemas"]["UpdateAdminOAuthProviderRequest"];
    }) =>
      unwrap(
        await api.PUT("/v1/admin/oauth-providers/{provider}", {
          params: {
            path: {
              provider:
                provider as PathsV1AdminOauthProvidersProviderPutParametersPathProvider,
            },
          },
          body,
        }),
      ),
    onSuccess: () =>
      queryClient.invalidateQueries({
        queryKey: queryKeys.adminOAuthProviders,
      }),
  });
}

/** Remove one provider's stored credentials. */
export function useDeleteAdminOAuthProvider() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (provider: OAuthProviderName) =>
      unwrap(
        await api.DELETE("/v1/admin/oauth-providers/{provider}", {
          params: {
            path: {
              provider:
                provider as PathsV1AdminOauthProvidersProviderDeleteParametersPathProvider,
            },
          },
        }),
      ),
    onSuccess: () =>
      queryClient.invalidateQueries({
        queryKey: queryKeys.adminOAuthProviders,
      }),
  });
}
