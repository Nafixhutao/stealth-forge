/**
 * Cache invalidation policy, split by domain so a reviewer reads one policy at
 * a time:
 *
 * - `./keys` — domain-free primitives (`CacheQueryKey`, `addCacheKey`,
 *   `dedupeCacheKeys`).
 * - `./project` — project-scoped changes (`CacheChange`,
 *   `invalidationKeysFor`, `applyCacheChanges`).
 * - `./admin` — instance-Admin control-room changes (`AdminCacheChange`,
 *   `adminInvalidationKeysFor`).
 *
 * Both policies resolve to one shared vocabulary, so realtime and mutation
 * adapters cannot quietly grow different key lists. Import from this module
 * rather than the files above.
 */

export { addCacheKey, dedupeCacheKeys, type CacheQueryKey } from "./keys";
export {
  applyCacheChanges,
  invalidationKeysFor,
  type CacheChange,
} from "./project";
export { adminInvalidationKeysFor, type AdminCacheChange } from "./admin";
