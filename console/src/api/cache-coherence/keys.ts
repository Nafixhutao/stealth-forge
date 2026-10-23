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

/**
 * Drops every key present in `remove`. Used when a change both refreshes a
 * list and forgets a detail query: the detail must not be refetched, so it is
 * subtracted from the invalidation set rather than added to it as well.
 */
export function withoutCacheKeys(
  keys: readonly CacheQueryKey[],
  remove: readonly CacheQueryKey[],
): CacheQueryKey[] {
  const removed = new Set(remove.map((key) => JSON.stringify(key)));
  return keys.filter((key) => !removed.has(JSON.stringify(key)));
}
