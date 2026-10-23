import type { QueryClient } from "@tanstack/react-query";
import {
  adminForgottenKeysFor,
  adminInvalidationKeysFor,
  type AdminCacheChange,
} from "./admin";
import { dedupeCacheKeys, withoutCacheKeys, type CacheQueryKey } from "./keys";
import {
  forgottenKeysFor,
  invalidationKeysFor,
  type CacheChange,
} from "./project";

/**
 * Any change either policy understands. Mutations and a realtime adapter pass
 * one of these to {@link invalidate}; neither names a query key itself.
 */
export type CacheOperation = CacheChange | AdminCacheChange;

type Invalidator = Pick<QueryClient, "invalidateQueries" | "removeQueries">;

function isAdminChange(change: CacheOperation): change is AdminCacheChange {
  return change.kind.startsWith("admin-");
}

/**
 * Resolves one change into the queries to refetch and the queries to forget.
 * Exported for tests: production callers use {@link invalidate}, which applies
 * the plan, and never need the key lists themselves.
 */
export function planCacheChange(change: CacheOperation): {
  invalidate: CacheQueryKey[];
  forget: CacheQueryKey[];
} {
  if (isAdminChange(change)) {
    const forget = adminForgottenKeysFor(change);
    return {
      invalidate: withoutCacheKeys(adminInvalidationKeysFor(change), forget),
      forget,
    };
  }
  const forget = forgottenKeysFor(change);
  return {
    invalidate: withoutCacheKeys(invalidationKeysFor(change), forget),
    forget,
  };
}

/**
 * The only entry point for cache invalidation. Every change in the list is
 * resolved through one shared vocabulary, deduplicated, and applied in two
 * phases: forget first, then refetch.
 *
 * The order matters. A forgotten detail key is usually a prefix of the list
 * key that replaces it, so invalidating first would leave the deleted resource
 * in the cache long enough for an observer to refetch it and receive a 404.
 */
export async function invalidate(
  queryClient: Invalidator,
  changes: readonly CacheOperation[],
): Promise<void> {
  const plans = changes.map(planCacheChange);
  const forget = dedupeCacheKeys(plans.flatMap((plan) => plan.forget));
  const refetch = dedupeCacheKeys(plans.flatMap((plan) => plan.invalidate));

  await Promise.all(
    forget.map((queryKey) => queryClient.removeQueries({ queryKey })),
  );
  await Promise.all(
    refetch.map((queryKey) => queryClient.invalidateQueries({ queryKey })),
  );
}
