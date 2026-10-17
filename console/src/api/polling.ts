/**
 * Polling cadence and freshness are one decision, so they are derived together.
 *
 * A query that refetches every N ms must not claim to be fresh for longer than
 * N ms. The Console sets a global `staleTime` of 15s, which would otherwise
 * outlive a 5s poller: the data would read as fresh while the poller is already
 * replacing it, so the two policies would contradict each other.
 *
 * The caller's `requested` interval wins when present, including `false` to
 * disable polling. `fallback` applies only when the caller is silent.
 *
 * When no interval applies, `staleTime` is omitted rather than set to
 * `undefined`. TanStack Query spreads query options over the global defaults,
 * so an explicit `undefined` would override the default instead of inheriting
 * it.
 */
export function polledQuery(
  requested: number | false | undefined,
  fallback?: number,
): { refetchInterval: number | false | undefined; staleTime?: number } {
  const refetchInterval = requested ?? fallback;
  return typeof refetchInterval === "number"
    ? { refetchInterval, staleTime: refetchInterval }
    : { refetchInterval };
}

/** In-process host, database, and Redis samples behind the Overview cards. */
export const POLL_FAST_MS = 5_000;

/** API-level counters, one step behind the in-process samples. */
export const POLL_MEDIUM_MS = 15_000;

/** Overview, accounts, and incident boards. */
export const POLL_SLOW_MS = 30_000;
