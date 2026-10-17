/**
 * Shared primitives for cache invalidation. These helpers carry no domain
 * knowledge: the project and Admin policies in `./project` and `./admin` build
 * their key lists with them.
 */

export type CacheQueryKey = readonly unknown[];

/**
 * Appends a query key unless an equivalent key is already present. Query keys
 * are compared structurally so different arrays with the same content collapse
 * into a single invalidation target.
 */
export function addCacheKey(keys: CacheQueryKey[], key: CacheQueryKey) {
  const serialized = JSON.stringify(key);
  if (!keys.some((candidate) => JSON.stringify(candidate) === serialized)) {
    keys.push(key);
  }
}

/** Removes duplicate query keys while preserving first-seen order. */
export function dedupeCacheKeys(
  keys: readonly CacheQueryKey[],
): CacheQueryKey[] {
  // Serialize each key once and dedupe through a Set, so this stays linear
  // rather than re-stringifying every candidate per insert.
  const seen = new Set<string>();
  const unique: CacheQueryKey[] = [];
  for (const key of keys) {
    const serialized = JSON.stringify(key);
    if (seen.has(serialized)) continue;
    seen.add(serialized);
    unique.push(key);
  }
  return unique;
}
