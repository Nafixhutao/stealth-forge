# Console caching and data-layer audit

Date: 2026-10-15
Mode: FRESH
Scope: Stealth Console data layer — TanStack Query keys, invalidation
coherence, staleness policy, realtime integration, and Next.js server caching
Branch: `feat/console-redesign`
Audited source: `d9e93ec61948c3bfa15dffa543d33a9f938f77d4`
Remediation: applied for C-2, C-3, C-4, C-5, C-6; C-8 has a design proposal; see [Changes applied](#changes-applied)

## Audit basis

A read-only review of the Console data layer after the console redesign. The
relevant guides in `node_modules/next/dist/docs/` were consulted for Next.js 16
behavior, and current library documentation for TanStack Query v5 and Tailwind
CSS v4 was consulted through Context7.

The Console fetches all data client-side through TanStack Query v5 over
`openapi-fetch`. There is no server-side data fetching except
`console/src/app/setup/page.tsx`, and no Server Actions.

### Surfaces reviewed

| Surface | Path | Size |
| --- | --- | --- |
| Query key registry | `console/src/api/query-keys.ts` | 158 lines |
| Invalidation policy | `console/src/api/cache-coherence.ts` | 585 lines (now split; see C-6) |
| Query hooks | `console/src/api/queries/*.ts` | 5 files |
| Mutation hooks | `console/src/api/mutations/*.ts` | 4 files |
| Global defaults | `console/src/app/providers.tsx` | 44 lines |
| Policy tests | `console/src/api/cache-coherence.test.ts` | 13 tests |

### Scale

- 36 `useQuery` call sites, 41 `useMutation` call sites.
- 32 `enabled:` guards, 10 `placeholderData: keepPreviousData` uses.
- 0 `onMutate` (no optimistic updates).
- 2 realtime `EventSource` consumers (`admin-telemetry.ts:60`,
  `browser-setup-flow.ts:188`), 0 `WebSocket`.

## What is already sound

These were verified in source, not assumed.

1. **Single key registry.** Every query key resolves through the `queryKeys`
   factory. No component builds a key literal inline.
2. **Semantic invalidation layer.** Callers describe a domain change
   (`{ kind: "agent-run", ... }`) and `cache-coherence/` translates it to
   keys. Realtime and mutation paths therefore cannot drift apart by accident.
3. **Sensible global defaults.** `staleTime: 15s`, `refetchOnWindowFocus: false`,
   and retry limited to network errors and 5xx, capped at 2 attempts.
4. **Paged admin tables keep previous data**, so pagination does not flash.
5. **Every query receives an abort signal** through `cancellableQuery`.
6. **Setup polling terminates itself.** `useSetupStatus` returns `false` for
   `refetchInterval` unless the install is in an in-flight phase, so a
   background tab stops polling.
7. **The log tail is correctly outside the query cache.** `useAdminLogTail`
   uses `EventSource` with `requestAnimationFrame` batching, a dedupe set, and
   a 250-item cap. This is the right call for a high-frequency stream.
8. **13 tests pin the invalidation policy** for the trickiest domains.

## Findings

### C-1. The Admin invalidation table is unexercised groundwork, not dead code

- Status: OPEN (revised)
- Priority: HIGH
- Area: `console/src/api/cache-coherence/` (at the audited revision,
  `cache-coherence.ts:526-585`)
- Evidence:
  - `adminInvalidationKeysFor` (`cache-coherence/admin.ts`) has **zero callers**
    in the repository. `AdminCacheChange` is referenced only by it. The
    admin half of the module has no test.
  - A first pass concluded this was removable dead code. That conclusion is
    **wrong**, and the correction matters:
    - `openapi/openapi.yaml:21` declares `/v1/admin/realtime` and references
      `./paths/admin-core.yaml#/paths/~1v1~1admin~1realtime`.
    - The Go backend implements it end to end —
      `internal/httpapi/routes_admin.go:9` (`r.With(s.requireInstanceAdmin).Get("/admin/realtime", s.adminRealtime)`),
      `internal/httpapi/server_admin_realtime.go`, and
      `internal/httpapi/admin_realtime_integration_test.go`.
    - Traefik routing is configured in `internal/installengine/traefik.go:23`
      (`stealth-admin-realtime`, rule `Path('/v1/admin/realtime')`).
    - `docs/realtime.md:8-21` documents the stream, its bounded
      `admin_realtime_events` outbox, and its `Last-Event-ID` cursor contract.
  - `adminInvalidationKeysFor` is the translation layer that a Console
    subscriber for that endpoint is meant to call. The function is not dead;
    the consumer is missing.
- Impact: the declarative admin policy and the 23 hand-written admin
  invalidations (`mutations/admin.ts` 21, `mutations/admin-oauth.ts` 2) can
  diverge without any test failing. Because the table is unexercised, a wrong
  entry would not surface until the subscriber lands.
- Recommended remediation: keep the table and make the missing consumer an
  explicit, tracked decision. Either wire a Console subscriber for
  `/v1/admin/realtime` (a feature; propose before implementing) or record that
  admin invalidation is intentionally imperative and remove the table. Do not
  treat the function as dead code in the meantime.
- Progress in this pass: the table is now covered by tests, so its policy is
  pinned even before the consumer exists.

### C-2. Four invalidation keys were built from literals instead of the factory

- Status: FIXED
- Priority: MEDIUM
- Area: `console/src/api/cache-coherence/`
- Evidence:
  - `[queryKeys.adminAuditEventsScope]` was a literal `[["admin", "audit-events"]]`
    in `cache-coherence/admin.ts`, duplicating the prefix of
    `queryKeys.adminAuditEvents`.
  - `["admin", "telemetry", "errors"]` duplicated the prefix of
    `queryKeys.adminTelemetry("errors", …)`.
  - `["function-deployments", projectId]` and `["site-deployments", projectId]`
    in `cache-coherence/project.ts` had no factory at all.
- Impact: a literal and its factory can be renamed independently. If that
  happens, invalidation stops matching silently — no type error, no failing
  test, just a stale screen.
- Fix applied: added four factories to `query-keys.ts` —
  `functionDeploymentsScope`, `siteDeploymentsScope`, `adminAuditEventsScope`,
  and `adminTelemetrySignal` — and replaced all four literals with them.

### C-3. `project` change invalidated a key it already covered

- Status: FIXED
- Priority: LOW
- Area: `console/src/api/cache-coherence/project.ts` (`case "project"`)
- Evidence: the case added both `queryKeys.project(projectId)`
  (`["project", id]`) and `queryKeys.audit("project", projectId)`
  (`["project", id, "audit"]`). TanStack Query `invalidateQueries` matches by
  key prefix, so the first already covered the second.
- Impact: no functional bug, but the second line implied a distinction that
  does not exist.
- Fix applied: removed the redundant key and left a comment recording why the
  single key is sufficient.

### C-4. Admin key parameters were typed `unknown`

- Status: FIXED
- Priority: LOW
- Area: `console/src/api/query-keys.ts`
- Evidence: `adminAuditEvents`, `adminTelemetry`, and `adminInfrastructure`
  took `query: unknown`, while every other factory in the file used concrete
  parameter types.
- Impact: `unknown` disabled the type safety the registry exists to provide. A
  caller could pass a non-serializable value, and the key would fail to match
  on the next render — a permanent cache miss rather than an error.
- Fix applied: added `AdminTelemetryQueryParams`, `AdminAuditEventsQueryParams`,
  and `AdminInfrastructureQueryParams` next to the factories and used them.
  `admin-telemetry.ts` now aliases its exported `AdminTelemetryQuery` to the
  shared type instead of redefining it, so the key and its parameters cannot
  drift.

### C-5. Poll intervals disagreed with the global `staleTime`

- Status: FIXED
- Priority: MEDIUM
- Area: `console/src/api/queries/admin-telemetry.ts`,
  `console/src/api/queries/admin-control.ts`,
  `console/src/app/providers.tsx:15`
- Evidence:
  - Global default was `staleTime: 15_000`.
  - `useAdminHostMetrics`, `useAdminDBMetrics`, and `useAdminRedisMetrics`
    polled every **5s**; `useAdminAPIMetrics` every 15s; `useAdminOverview` and
    `useAdminAccounts` every 30s.
  - None of them overrode `staleTime`, so they inherited 15s while polling on a
    different cadence. A 5s poller read as fresh for 15s, and a 30s poller went
    stale at 15s with nothing due to refresh it until 30s.
  - Callers passed intervals too (`admin-overview.tsx:74` and
    `incidents-page.tsx:34` both request 30s), which the hooks could not reflect
    in staleness.
  - `refetchOnWindowFocus` is globally `false`, so a page that does not poll
    never refreshes while it stays mounted.
- Impact: the freshness contract was ambiguous — the numbers contradicted each
  other. Not a live defect, because `refetchInterval` refetches regardless of
  staleness, but it was fragile and hard to reason about.
- Fix applied: added `console/src/api/polling.ts` with a `polledQuery` helper
  that derives `staleTime` from the effective interval, plus named cadence
  constants `POLL_FAST_MS` (5s), `POLL_MEDIUM_MS` (15s), and `POLL_SLOW_MS`
  (30s). All 27 polling hooks across the two admin query modules now derive both
  values from one source, so they agree by construction.
  - The caller's interval wins, including `false` to disable polling.
  - When no interval applies, `staleTime` is omitted rather than set to
    `undefined`. TanStack Query spreads query options over the global defaults
    (`queryClient.js`, `defaultQueryOptions`), so an explicit `undefined` would
    override the default instead of inheriting it. This is the reason the helper
    returns two different object shapes.
  - `useSetupStatus` in `queries/setup.ts` already paired `staleTime: 2_000`
    with a 2s poll and was left unchanged.
- Behavior change: staleness now moves with the interval. The 5s pollers become
  stale sooner (5s instead of 15s) and the 30s pollers later (30s instead of
  15s). Polling itself is unaffected — `refetchInterval` fires on the same
  cadence as before. The only difference is whether a remount or refetch within
  the window hits the cache or the network.

### C-6. `cache-coherence.ts` mixed two domains at 585 lines

- Status: FIXED
- Priority: LOW
- Area: `console/src/api/cache-coherence/`
- Evidence: `console/AGENTS.md` states that 300–600 lines "merits a
  responsibility review". The file held project domain changes, admin domain
  changes, and shared key-dedupe helpers in one 585-line module.
- Impact: navigability. Reviewers had to scan admin and project policy together
  to change either.
- Fix applied: split into a directory with a barrel, matching the existing
  `src/api/queries.ts` and `src/api/mutations.ts` convention:

  | File | Lines | Contents |
  | --- | --- | --- |
  | `cache-coherence/keys.ts` | 36 | `CacheQueryKey`, `addCacheKey`, `dedupeCacheKeys` |
  | `cache-coherence/project.ts` | 490 | `CacheChange`, `invalidationKeysFor`, `applyCacheChanges` |
  | `cache-coherence/admin.ts` | 68 | `AdminCacheChange`, `adminInvalidationKeysFor` |
  | `cache-coherence/index.ts` | 23 | Barrel; the only import surface |

  The public API is unchanged: the same 8 symbols are exported, and the two
  existing importers (`mutations/auth.ts`, `cache-coherence.test.ts`) resolve
  through the barrel without edits. `project.ts` remains large because
  `invalidationKeysFor` is a single 27-case switch; splitting the switch itself
  is a separate change with no current driver.

### C-7. No optimistic updates anywhere

- Status: OPEN (trade-off)
- Priority: LOW
- Area: all 41 mutation hooks
- Evidence: zero `onMutate` and zero `setQueryData` outside
  `mutations/setup.ts:17`.
- Impact: every write waits for a server round trip before the UI reflects it.
  For toggle and rename interactions in an operations console, that is
  avoidable perceived latency.
- Recommended remediation: not required. Apply optimistic updates only where a
  rollback is cheap and well understood, and leave the rest as-is.

### C-8. Next.js 16 server caching is unused

- Status: OPEN (design proposal written)
- Priority: MEDIUM
- Area: `console/next.config.ts`, `console/src/app/**`
- Evidence:
  - No `cacheComponents`, `use cache`, `cacheLife`, `cacheTag`,
    `revalidateTag`, or Server Actions anywhere in `console/src`.
  - `next.config.ts` sets only `output: "standalone"`, `poweredByHeader`,
    `reactStrictMode`, and `allowedDevOrigins`.
  - The single server-side fetch, `src/app/setup/page.tsx:34`, uses
    `cache: "no-store"`.
  - Next.js 16 stabilizes `cacheLife` and `cacheTag` (no `unstable_` prefix),
    which makes them available without preview flags.
- Impact: every route pays a client waterfall — HTML, then JS, then hydrate,
  then fetch. Data that is stable per user or per project cannot be rendered
  on the server or reused across navigations.
- Design proposal: [Console server-side caching](console-server-caching-design-2026-10-15.md).
  Investigating the change found a structural blocker rather than a tuning
  decision: the Console's data is authorized by a session cookie, and a
  `use cache` scope cannot call `cookies()`. Only one surface
  (`/v1/status-page`, declared `security: [{}]`) is cacheable without an
  authentication change, which does not justify enabling an app-wide flag that
  also changes navigation semantics and requires the Node.js runtime.
- Recommended remediation: follow the design proposal — address the waterfall
  with `loading.tsx` and streaming boundaries, and treat server-side caching of
  authorized data as a separate security proposal that must land first.

### C-9. `realtime.md` describes an Admin Console subscriber that does not exist

- Status: OPEN
- Priority: MEDIUM
- Area: `docs/realtime.md:8-21`, `console/src`
- Evidence:
  - `docs/realtime.md:8` states in the present tense: *"Instance-owner/admin
    Console sessions also subscribe to `GET /v1/admin/realtime`."*
  - The Console contains no subscriber for that endpoint. The only
    `EventSource` uses are `admin-telemetry.ts:60` (admin log tail) and
    `browser-setup-flow.ts:188` (setup install events).
  - The backend endpoint, contract entry, integration test, and Traefik route
    all exist (see C-1).
- Impact: readers cannot tell whether Admin Console sessions receive live
  invalidation or not. If the subscriber is genuinely missing, Admin views rely
  only on bounded polling and manual refresh; if it exists elsewhere, the doc
  is pointing at the wrong place. Either way the documentation currently
  overstates the shipped behavior.
- Recommended remediation: choose one and make it true — either implement the
  Admin Console subscriber (which would also exercise C-1's table), or reword
  `realtime.md` to describe the endpoint as available to subscribers that have
  not yet been implemented in the Console.

## Changes applied

Scoped to C-2, C-3, C-4, and C-5. For C-2 through C-4 no behavior change is
intended: every key produced after this pass is byte-for-byte the same as before
it. C-5 intentionally changes staleness, described below.

| File | Change |
| --- | --- |
| `console/src/api/query-keys.ts` | Added `AdminTelemetryQueryParams`, `AdminAuditEventsQueryParams`, `AdminInfrastructureQueryParams`; added `functionDeploymentsScope`, `siteDeploymentsScope`, `adminAuditEventsScope`, `adminTelemetrySignal`; typed the three Admin factories |
| `console/src/api/cache-coherence/keys.ts` | New: `CacheQueryKey`, `addCacheKey`, `dedupeCacheKeys`, moved out of the old single file |
| `console/src/api/cache-coherence/project.ts` | New: `CacheChange`, `invalidationKeysFor`, `applyCacheChanges`; replaced 2 key literals with the new factories; removed the redundant `audit` key from `case "project"` |
| `console/src/api/cache-coherence/admin.ts` | New: `AdminCacheChange`, `adminInvalidationKeysFor`; replaced 2 key literals with the new factories |
| `console/src/api/cache-coherence/index.ts` | New: barrel exporting the same 8 public symbols as the removed `cache-coherence.ts` |
| `console/src/api/queries/admin-telemetry.ts` | `AdminTelemetryQuery` is now an alias of `AdminTelemetryQueryParams` instead of a duplicate definition; all 16 polling hooks derive cadence and staleness through `polledQuery` |
| `console/src/api/queries/admin-control.ts` | All 11 polling hooks derive cadence and staleness through `polledQuery` |
| `console/src/api/polling.ts` | New: `polledQuery` helper and the `POLL_FAST_MS` / `POLL_MEDIUM_MS` / `POLL_SLOW_MS` cadence constants |
| `console/src/api/cache-coherence.test.ts` | Added 5 tests: Admin monitor mapping, Admin error-group mapping, the `project` single-key guarantee, and two scope-alignment drift guards |
| `console/src/api/polling.test.ts` | New: 6 tests pinning the cadence/staleness contract, including that polling disabled leaves `staleTime` unset |

The old `console/src/api/cache-coherence.ts` was deleted; its contents moved into
the directory above with no logic change.

### Behavior change in C-5

Staleness now follows the poll cadence. `refetchInterval` is unchanged, so
polling fires exactly as often as before; what changes is whether a remount or
refetch inside the window reads the cache or goes to the network.

| Hook | Interval | `staleTime` before | `staleTime` after |
| --- | --- | --- | --- |
| `useAdminHostMetrics`, `useAdminDBMetrics`, `useAdminRedisMetrics` | 5s | 15s | 5s |
| `useAdminAPIMetrics` | 15s | 15s | 15s (unchanged) |
| `useAdminOverview`, `useAdminAccounts` | 30s | 15s | 30s |
| `useAdminIncidents` (caller passes 30s) | 30s | 15s | 30s |
| Hooks with no interval | none | 15s | 15s (unchanged) |

### Validation

```text
npm run typecheck   # tsc --noEmit, clean
npm run lint        # eslint, clean
npm run test        # 16 files, 72 tests passed
```

The policy test file went from 13 to 18 tests; `polling.test.ts` adds 6.

## Summary

| ID | Priority | Status | Finding |
| --- | --- | --- | --- |
| C-1 | HIGH | OPEN (revised) | Admin invalidation table is unexercised groundwork for a missing subscriber, not dead code |
| C-2 | MEDIUM | FIXED | Four invalidation keys bypassed the key factory |
| C-5 | MEDIUM | FIXED | Poll intervals contradicted the global `staleTime` |
| C-8 | MEDIUM | OPEN (design proposal) | Next.js 16 server caching unused; blocked by cookie-authorized reads |
| C-9 | MEDIUM | OPEN | `realtime.md` claims an Admin Console subscriber that does not exist |
| C-3 | LOW | FIXED | Redundant `audit` invalidation in `project` |
| C-4 | LOW | FIXED | Admin key params typed `unknown` |
| C-6 | LOW | FIXED | `cache-coherence.ts` mixed two domains |
| C-7 | LOW | OPEN | No optimistic updates (trade-off) |

The remaining HIGH/MEDIUM items need a decision rather than a mechanical fix:
C-1 and C-9 are two halves of the same question — does the Admin Console
subscribe to `/v1/admin/realtime`? C-8 now has a design proposal recommending
against the change as scoped.

## Verification commands

Read-only inspection used for this audit:

```bash
# C-1 / C-9: the endpoint exists in backend, contract, and routing
grep -rn "admin/realtime" internal/httpapi openapi --include="*.go" --include="*.yaml"
grep -rn "adminInvalidationKeysFor\|AdminCacheChange" . --include="*.ts" --include="*.tsx"

# C-2: literal keys
grep -n 'addCacheKey(keys, \[' console/src/api/cache-coherence/*.ts

# C-5: staleness and polling policy
grep -rn "polledQuery" console/src/api/queries/*.ts
grep -rn "refetchInterval: options?.refetchInterval" console/src/api/queries/  # expect no matches

# C-8: server caching
grep -rn "use cache\|cacheComponents\|cacheTag\|cacheLife" console/src console/next.config.ts
```

## Remediation order

1. Decide the Admin realtime question (C-1 and C-9 together). Implementing the
   subscriber both closes the doc gap and exercises the now-tested policy table.
2. Decide C-8 against the [design proposal](console-server-caching-design-2026-10-15.md):
   adopt the `loading.tsx` route (Option B), or approve the API identity work
   (Option C) before any caching change.
3. C-7 remains optional and needs no decision until a specific interaction
   justifies it.
