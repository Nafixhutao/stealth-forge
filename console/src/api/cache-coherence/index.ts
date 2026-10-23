/**
 * Cache invalidation policy. One operation, one vocabulary:
 *
 * - `./keys` — domain-free primitives (`CacheQueryKey`, `addCacheKey`,
 *   `dedupeCacheKeys`, `withoutCacheKeys`).
 * - `./project` — project-scoped changes (`CacheChange`, `invalidationKeysFor`,
 *   `forgottenKeysFor`).
 * - `./admin` — instance-Admin control-room changes (`AdminCacheChange`,
 *   `adminInvalidationKeysFor`, `adminForgottenKeysFor`).
 * - `./invalidate` — `invalidate(queryClient, changes)`, the only entry point.
 *
 * Both policies resolve to one shared vocabulary, so mutation and realtime
 * adapters cannot grow different key lists. Callers describe *what changed*
 * and this module decides *what goes stale*; nothing outside this directory
 * names a query key for invalidation.
 */

export { addCacheKey, dedupeCacheKeys, type CacheQueryKey } from "./keys";
export {
  forgottenKeysFor,
  invalidationKeysFor,
  type CacheChange,
} from "./project";
export {
  adminForgottenKeysFor,
  adminInvalidationKeysFor,
  type AdminCacheChange,
} from "./admin";
export { invalidate, planCacheChange, type CacheOperation } from "./invalidate";
