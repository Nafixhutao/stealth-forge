import "server-only";

// The setup API and the console share the internal alias `stealth-api` in both
// the production and setup Compose projects, so one default covers both.
const INTERNAL_API_URL = (
  process.env.STEALTH_API_INTERNAL_URL ?? "http://stealth-api:8080"
).replace(/\/+$/, "");

const PROBE_TIMEOUT_MS = 2_000;

export type SessionResolution =
  | { status: "authenticated"; role: string | null }
  | { status: "anonymous" }
  /** The API could not answer, so the session is unknown rather than absent. */
  | { status: "unavailable" };

/**
 * Resolves the Console session on the server from the request's cookie header.
 *
 * Callers must treat `unavailable` differently from `anonymous`. An outage is
 * not evidence of a signed-out visitor, and acting on it would strand a
 * signed-in operator on the sign-in page — which itself bounces authenticated
 * visitors away, producing a redirect loop.
 *
 * Only the request's cookies are forwarded to the API — no other request
 * headers. The Console origin may hold unrelated cookies, which the API is
 * free to ignore; the point is that the probe never widens the request.
 */
export async function resolveSession(
  cookieHeader: string | null,
): Promise<SessionResolution> {
  if (!cookieHeader) return { status: "anonymous" };

  let response: Response;
  try {
    response = await fetch(`${INTERNAL_API_URL}/v1/account`, {
      headers: { cookie: cookieHeader },
      cache: "no-store",
      signal: AbortSignal.timeout(PROBE_TIMEOUT_MS),
    });
  } catch {
    return { status: "unavailable" };
  }
  if (response.status === 401) return { status: "anonymous" };
  if (!response.ok) return { status: "unavailable" };

  try {
    const body = (await response.json()) as {
      account?: { instance_role?: string | null };
    };
    return {
      status: "authenticated",
      role: body.account?.instance_role ?? null,
    };
  } catch {
    return { status: "unavailable" };
  }
}

// The proxy runs on every request matching /admin or /user — including RSC
// prefetches, which arrive in bursts while a visitor browses. Each hits the
// account API, so a page load can cost several identical probes. A tiny
// per-cookie cache absorbs those bursts without removing the check: results
// live for a few seconds, and the client SessionGate/AdminGate still re-check
// against the API on load, so a session revoked mid-window falls back correct.
const PROXY_CACHE_TTL_MS = 5_000;
const PROXY_CACHE_LIMIT = 1_000;

const proxyCache = new Map<string, { at: number; value: SessionResolution }>();

/**
 * `resolveSession`, deduplicated for the proxy's per-request traffic.
 *
 * Keyed by the exact cookie header, so two visitors never share a result, and
 * capped so arbitrary visitor cookies cannot grow it without bound (oldest
 * entry evicted). A cache hit may be up to TTL old, which is the only new
 * staleness this adds; revocation within that window is still caught by the
 * client-side gates.
 */
export async function resolveSessionCached(
  cookieHeader: string | null,
): Promise<SessionResolution> {
  const key = cookieHeader ?? "";
  const hit = proxyCache.get(key);
  if (hit && Date.now() - hit.at < PROXY_CACHE_TTL_MS) return hit.value;

  const value = await resolveSession(cookieHeader);
  // Re-insertion acts as the refresh: expired entries move to the newest slot
  // instead of squatting on their original position.
  proxyCache.delete(key);
  proxyCache.set(key, { at: Date.now(), value });
  if (proxyCache.size > PROXY_CACHE_LIMIT) {
    const oldest = proxyCache.keys().next().value;
    if (oldest !== undefined) proxyCache.delete(oldest);
  }
  return value;
}
