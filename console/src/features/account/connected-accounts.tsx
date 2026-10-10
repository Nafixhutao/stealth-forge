"use client";

import { useEffect } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Check, Link2, Unlink } from "lucide-react";
import { toast } from "sonner";
import { api, unwrap } from "@/api/client";
import { queryKeys } from "@/api/query-keys";
import { useAccountIdentities } from "@/api/queries/account";
import { AUTH_PROVIDERS } from "@/features/auth/auth-standalone";
import {
  AccountIdentitiesResponseProviders,
  PathsV1AccountIdentitiesProviderDeleteParametersPathProvider,
  PathsV1AccountIdentitiesProviderLinkGetParametersPathProvider,
} from "@/api/generated/schema";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

const PROVIDER_LABEL: Record<string, string> = {
  github: "GitHub",
  google: "Google",
};

/** Banner copy for the ?link= result the API redirects back with. */
const LINK_RESULT: Record<string, { text: string; ok: boolean }> = {
  ok: { text: "Sign-in provider linked", ok: true },
  taken: { text: "That account is already linked to another user", ok: false },
  mismatch: { text: "Link session did not match this account", ok: false },
  session: { text: "Sign in again, then retry linking", ok: false },
  error: { text: "Could not link that provider", ok: false },
};

/**
 * Connected accounts: link and unlink external sign-in providers. Linking
 * keeps the current session and only attaches the identity, so it can never
 * sign the browser into a different account.
 */
export function ConnectedAccounts() {
  const queryClient = useQueryClient();
  const query = useAccountIdentities();
  const linked = query.data?.items ?? [];
  const available = query.data?.providers ?? [];

  const link = useMutation({
    mutationFn: async (provider: string) =>
      unwrap(
        await api.GET("/v1/account/identities/{provider}/link", {
          params: {
            path: {
              provider:
                provider as PathsV1AccountIdentitiesProviderLinkGetParametersPathProvider,
            },
          },
        }),
      ),
    onSuccess: (result) => {
      if (result?.authorization_url) {
        window.location.assign(result.authorization_url);
      }
    },
    onError: () => toast.error("Could not start linking that provider"),
  });

  const unlink = useMutation({
    mutationFn: async (provider: string) => {
      const response = await api.DELETE("/v1/account/identities/{provider}", {
        params: {
          path: {
            provider:
              provider as PathsV1AccountIdentitiesProviderDeleteParametersPathProvider,
          },
        },
      });
      if (response.error) throw response.error;
    },
    onSuccess: () => {
      toast.success("Sign-in provider unlinked");
      void queryClient.invalidateQueries({
        queryKey: queryKeys.accountIdentities,
      });
      void queryClient.invalidateQueries({ queryKey: queryKeys.account });
    },
    onError: () => toast.error("The last sign-in method cannot be unlinked"),
  });

  // Surface the redirect result once, then drop the query param so a refresh
  // does not replay the toast.
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const result = params.get("link");
    if (!result) return;
    const message = LINK_RESULT[result];
    if (message) {
      if (message.ok) toast.success(message.text);
      else toast.error(message.text);
    }
    params.delete("link");
    params.delete("provider");
    const query = params.toString();
    window.history.replaceState(
      null,
      "",
      window.location.pathname + (query ? `?${query}` : ""),
    );
    void queryClient.invalidateQueries({
      queryKey: queryKeys.accountIdentities,
    });
    void queryClient.invalidateQueries({ queryKey: queryKeys.account });
  }, [queryClient]);

  return (
    <Card className="mt-5">
      <CardHeader>
        <CardTitle>Connected accounts</CardTitle>
        <p className="mt-1 text-xs text-slate-500">
          Link the sign-in providers you use. Unlinking only removes the
          identity; your account and password stay unchanged.
        </p>
      </CardHeader>
      <CardContent className="space-y-3">
        {AUTH_PROVIDERS.filter((provider) =>
          available.includes(provider.id as AccountIdentitiesResponseProviders),
        ).map((provider) => {
          const identity = linked.find((item) => item.provider === provider.id);
          return (
            <div
              key={provider.id}
              className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-stealth-border p-4"
            >
              <div className="flex min-w-0 items-center gap-3">
                <span className="flex size-9 shrink-0 items-center justify-center rounded-lg border border-stealth-border bg-white/[0.03]">
                  {provider.mark}
                </span>
                <div className="min-w-0">
                  <p className="m-0 flex items-center gap-2 text-sm font-medium text-white">
                    {PROVIDER_LABEL[provider.id] ?? provider.name}
                    {identity ? (
                      <Badge variant="success">
                        <Check size={11} strokeWidth={3} aria-hidden="true" />
                        Linked
                      </Badge>
                    ) : null}
                  </p>
                  <p className="m-0 mt-0.5 truncate text-xs text-slate-500">
                    {identity
                      ? (identity.provider_login ??
                        identity.display_name ??
                        "Connected")
                      : "Not connected"}
                  </p>
                </div>
              </div>
              {identity ? (
                <Button
                  variant="outline"
                  size="sm"
                  disabled={unlink.isPending}
                  onClick={() => unlink.mutate(provider.id)}
                >
                  <Unlink size={13} aria-hidden="true" /> Unlink
                </Button>
              ) : (
                <Button
                  variant="outline"
                  size="sm"
                  disabled={link.isPending}
                  onClick={() => link.mutate(provider.id)}
                >
                  <Link2 size={13} aria-hidden="true" /> Link
                </Button>
              )}
            </div>
          );
        })}
        {available.length === 0 ? (
          <p className="text-sm text-slate-500">
            No external sign-in providers are configured on this instance.
          </p>
        ) : null}
      </CardContent>
    </Card>
  );
}
