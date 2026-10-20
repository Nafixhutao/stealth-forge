# Console server-side caching: design proposal

Date: 2026-10-15
Status: PROPOSAL — no implementation. Requires an owner decision.
Scope: Whether and how to adopt Next.js 16 Cache Components (`cacheComponents`,
`use cache`, `cacheLife`, `cacheTag`) in the Stealth Console.
Related: [Console caching and data-layer audit](console-caching-audit-2026-10-15.md) (finding C-8)

## Why this note exists

The audit recorded C-8: the Console does not use any Next.js server-side
caching, so every route pays a client waterfall — HTML, then JS, then hydrate,
then fetch. Next.js 16 stabilizes `cacheLife` and `cacheTag`, which makes
server-side caching newly available without preview flags.

That framing is accurate but incomplete. Investigating the change surfaced a
structural constraint that decides the whole question: **the Console's data is
authorized by a session cookie, and a `use cache` scope cannot read cookies.**
This note records the constraint, the options, and a recommendation. It does not
propose a change to ship now.

## Current state

Verified in source.

| Fact | Evidence |
| --- | --- |
| All product data is fetched client-side | 36 `useQuery` call sites in `console/src/api/queries/*` |
| The API client sends the session cookie | `console/src/api/client.ts:26` — `credentials: "include"` |
| Pages are thin server shells rendering client features | `console/src/app/admin/page.tsx` returns `<AdminOverview />` |
| Authorization is enforced by the Go API per request | no Console middleware exists; `AdminGate` is a client boundary |
| Only two server-side fetches exist | `src/app/setup/page.tsx:34`, `src/features/auth/authenticated-redirect.server.ts:35` |
| Both bypass the cache | `cache: "no-store"` in each |
| One route opts out of prerendering | `src/app/setup/page.tsx:14` — `export const dynamic = "force-dynamic"` |
| No route uses `loading.tsx` | 0 files; `Suspense` appears only on auth and setup routes |
| `next.config.ts` sets no caching options | only `output`, `poweredByHeader`, `reactStrictMode`, `allowedDevOrigins` |

The existing server-side pattern is already established in
`authenticated-redirect.server.ts`: it reads the cookie with `cookies()`,
forwards it as a `cookie` header to the internal API
(`STEALTH_API_INTERNAL_URL`, default `http://stealth-api:8080`), and treats the
response as uncacheable. That file is marked `server-only`.

## What Next.js 16 Cache Components provides

Grounded in the bundled documentation at
`console/node_modules/next/dist/docs/`. The version installed is Next.js 16.4.0.

### Enabling it is an app-wide change

`cacheComponents: true` must be paired with an explicit `partialPrefetching`
setting, or the build logs a warning
(`01-app/03-api-reference/05-config/01-next-config-js/cacheComponents.md:16-29`).

Three consequences follow from the single flag:

1. **Partial Prerendering becomes the default.** `experimental.ppr` and the
   `experimental_ppr` segment config are removed
   (`cacheComponents.md:41`).
2. **Navigation semantics change.** Next.js uses React's `<Activity>` to keep
   the previous route mounted with mode `"hidden"` instead of unmounting it.
   Component state is preserved across navigation, and the docs warn that
   "some UI patterns behave differently when components stay mounted instead of
   unmounting", pointing at dropdowns, dialogs, and testing
   (`cacheComponents.md:47-61`).
3. **Node.js runtime is required.** Routes exporting `runtime = 'edge'` must be
   migrated (`cacheComponents.md:31`).

This is the part that matters most for Stealth: the flag is not scoped to one
route. Enabling it to cache a handful of reads also changes how every route in
the Console behaves on navigation.

### Cached scopes cannot read request data

A `use cache` function or component **cannot** call `cookies()`, `headers()`, or
`searchParams`, and the restriction follows the call stack
(`use-cache.md:241`). The documented failure mode is important for a self-hosted
product:

> On a dynamically rendered route this surfaces when the route runs, so it can
> pass `next build` and fail under `next start`.

The error is `next-request-in-use-cache`. A Console that builds green in CI and
fails on an operator's host is the worst possible outcome for Stealth.

### Caching a session-derived read requires a value you can pass

The authentication guide is explicit (`authentication-with-cache-components.md`):
a session read cannot be prerendered into the static shell. It must sit behind a
`<Suspense>` boundary and stream in. To cache session-derived data on the server,
the value has to be extracted and passed as an argument:

- Plain `use cache` and `use cache: remote` cannot call `cookies()`.
- `use cache: private` **can** read `cookies()` and `headers()`, but it "keeps
  the result in the browser only, never on the server" — so it does not reduce
  server work or API load (`authentication-with-cache-components.md:58-60`).

The guide's own example shows why a cookie-based session resists extraction:
"a session helper reads the cookie deep inside its own code, so there's nothing
to lift out" (`authentication-with-cache-components.md:55`).

### Runtime behavior on a self-hosted deployment

Stealth ships as self-hosted Docker Compose, so the self-hosted row applies:
cache entries persist across requests in an in-memory LRU, sized by
`cacheMaxMemorySize` (`use-cache.md:245-256`).

Four further properties constrain what caching can buy here:

| Property | Consequence |
| --- | --- |
| Cache key includes the Build ID | Entries do **not** carry across a deploy (`use-cache.md:83`, `:259`) |
| Client enforces a 30s minimum stale time | Config cannot make client reuse more aggressive than 30s (`use-cache.md:321`) |
| `cacheLife` should be set explicitly in every scope | Otherwise the `default` profile applies invisibly (`use-cache.md:336-353`) |
| A short-lived cache nested in one without explicit `cacheLife` fails the build | Composes badly with ad-hoc adoption (`use-cache.md:364`) |

### Migration is route-by-route

The guide provides an escape hatch: `export const instant = false` on a page or
layout lets it "keep blocking on the server" while patterns are adopted one
route at a time (`authentication-with-cache-components.md:45`).

## The blocking constraint

Stealth's Console data is **per-user and per-project, authorized by a session
cookie that only the Go API can validate.** The Console has no middleware and no
server-side authorization of its own; `AdminGate` is a client boundary, and the
API rejects unauthorized requests per call.

Combining that with the framework constraints:

- A cached server read cannot present the session cookie, because a `use cache`
  scope cannot call `cookies()`.
- The value that *could* be passed in is the session identity — but the Console
  cannot derive one. The Go API owns session validation, and the Console only
  holds an opaque cookie.
- `use cache: private` reads the cookie but caches in the browser only, so it
  changes nothing about server work or API load.
- The existing workaround — forwarding the raw cookie header, as
  `authenticated-redirect.server.ts` does — cannot be cached, because the cookie
  is request-time data and would leak one user's response to another.

So for the bulk of the Console, **server-side caching is not a tuning decision;
it is blocked on an authentication change.** Caching authorized reads requires
the Go API to accept a server-to-server identity that a cached scope can hold
and pass as an argument.

## Candidate surfaces

Sorting the Console's reads by whether they are user-specific and whether they
are cookie-authorized:

| Candidate | User-specific | Cookie-authorized | Cacheable today |
| --- | --- | --- | --- |
| `/v1/status-page` (public status page) | No | **No** — `security: [{}]` | **Yes** |
| Agent catalog | No | Yes | No — blocked |
| Project metadata, service layout | Yes (per project) | Yes | No — blocked |
| Admin telemetry, metrics, audit | Yes (admin only) | Yes | No — blocked |
| Account, organizations, memberships | Yes | Yes | No — blocked |

Exactly one surface is cacheable without an authentication change: the public
status page, declared with `security: [{}]` in
`openapi/paths/admin-monitors.yaml:78`. It is a single low-churn read on one
route. That is a thin return for enabling an app-wide flag.

## Options

### Option A — Enable Cache Components and cache the public status page

Turn on `cacheComponents` + `partialPrefetching`, wrap the status page read in
`use cache` with an explicit `cacheLife` and a `cacheTag`.

- Buys: one route stops paying a client waterfall.
- Costs: app-wide navigation semantics change through `<Activity>`; PPR becomes
  the default everywhere; every route that reads the session is flagged by
  instant-navigation validation; the Node.js runtime requirement becomes
  binding; the `next-request-in-use-cache` failure mode is introduced on a
  self-hosted product.
- Assessment: **the cost is disproportionate to the benefit.** Do not do this
  for one public read.

### Option B — Keep client-side fetching; remove the waterfall instead

Address the actual symptom — first paint waits on JS then a fetch — without new
caching semantics: add `loading.tsx` boundaries where routes currently have
none, stream the shell, and let the existing TanStack Query states render inside
it.

- Buys: a faster perceived first paint on every console route, with no new
  caching model and no change to authorization.
- Costs: layout work per route; no reduction in API calls.
- Assessment: **the lowest-risk way to capture most of the stated benefit.**

### Option C — Give the Go API a server-to-server identity, then cache

Add a credential the Console's server can use to call the API on a specific
user's behalf, so a cached scope can take an explicit identity argument instead
of a cookie.

- Buys: unlocks server-side caching for the real surfaces (project metadata,
  service layout, catalogs).
- Costs: a new authentication path in a security-sensitive product. It must be
  scoped, auditable, and must not become a way to bypass tenant isolation.
- Assessment: **the only path that makes C-8 broadly worthwhile**, and it is a
  security change first and a caching change second.

### Option D — Do nothing

Accept the waterfall as the cost of a client-side console.

- Assessment: defensible. The audit found no correctness defect here, only an
  unused opportunity.

## Recommendation

1. **Do not enable `cacheComponents` to serve C-8.** The single cacheable
   surface does not justify an app-wide navigation and rendering change, and the
   `next-request-in-use-cache` failure mode is a poor fit for a self-hosted
   product that must not fail on an operator's host.
2. **Pursue Option B.** Adding `loading.tsx` and streaming boundaries is the
   proportionate fix for the observed waterfall and carries no authorization
   risk.
3. **Treat Option C as a separate proposal** with its own security review. If
   the owner wants server-side caching of authorized data, that review comes
   first — the caching follows from it.
4. **Revisit this note** if the Go API gains a server-to-server identity, or if
   the Console gains routes whose data is genuinely public.

## Staged plan if Option C is approved

Do not start with the flag.

1. **Design the API identity path.** Define the credential, its scope, its audit
   events, and the tenant-isolation tests. Land it with no Console changes.
2. **Spike on one route.** Enable `cacheComponents` + `partialPrefetching` on a
   branch, adopt `instant = false` on every existing route to keep current
   behavior, and convert one read. Measure before and after.
3. **Verify the failure modes.** Confirm `next build` and `next start` agree on
   every route, exercise the `<Activity>` navigation changes against dropdowns
   and dialogs, and check `cacheMaxMemorySize` behavior under load.
4. **Expand route by route**, removing `instant = false` as each route is
   migrated.

## Risks to record

| Risk | Why it matters here |
| --- | --- |
| `next-request-in-use-cache` passes build, fails at runtime | Operators run `next start`; CI would not catch it |
| `<Activity>` changes navigation behavior app-wide | Stealth's Console has dropdowns and dialogs; the docs warn these differ |
| Entries are keyed by Build ID | Caching does not survive a deploy, so it cannot warm a release |
| `use cache: private` caches in the browser only | It cannot reduce API load, which is the stated motivation |
| Client 30s minimum stale time | Bounds how much freshness the config can buy |
| Nested short-lived caches fail the build | Ad-hoc adoption has sharp edges |

## What this note does not propose

- It does not propose enabling `cacheComponents` now.
- It does not propose caching any session-authorized read without the
  server-to-server identity path.
- It does not change the TanStack Query layer. The C-2, C-3, C-4, and C-5
  remediations in the audit stand on their own.

## References

Bundled Next.js 16.4.0 documentation:

- `console/node_modules/next/dist/docs/01-app/03-api-reference/05-config/01-next-config-js/cacheComponents.md`
- `console/node_modules/next/dist/docs/01-app/03-api-reference/01-directives/use-cache.md`
- `console/node_modules/next/dist/docs/01-app/03-api-reference/04-functions/cacheTag.md`
- `console/node_modules/next/dist/docs/01-app/03-api-reference/05-config/01-next-config-js/cacheLife.md`
- `console/node_modules/next/dist/docs/01-app/02-guides/authentication-with-cache-components.md`

Stealth sources:

- `console/src/api/client.ts`
- `console/src/app/setup/page.tsx`
- `console/src/features/auth/authenticated-redirect.server.ts`
- `console/next.config.ts`
- `openapi/paths/admin-monitors.yaml`
- `docs/realtime.md`
