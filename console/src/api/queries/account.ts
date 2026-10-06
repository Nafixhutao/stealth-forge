"use client";
import { useQuery } from "@tanstack/react-query";
import { api, cancellableQuery, unwrap } from "@/api/client";
import { queryKeys } from "@/api/query-keys";

export function useCurrentAccount() {
  return useQuery({
    queryKey: queryKeys.account,
    queryFn: cancellableQuery((signal) => api.GET("/v1/account", { signal })),
    retry: false,
    staleTime: 60_000,
  });
}

/** Imperative account read for flows that run outside the app shell (for
 * example deciding the post-login destination from the instance role). */
export async function fetchCurrentAccount() {
  return unwrap(await api.GET("/v1/account"));
}

export function useAccountSessions() {
  return useQuery({
    queryKey: queryKeys.accountSessions,
    queryFn: cancellableQuery((signal) =>
      api.GET("/v1/account/sessions", { signal }),
    ),
    retry: false,
  });
}
