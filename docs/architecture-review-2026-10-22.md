# Architecture review

Date: 2026-10-22
Audited revision: `fcc664834208d69f7f0f1a642e2baf3041d19805`
Branch: `feat/admin-read-model`

This is a read-only review of the modules that the recent commit history
touches most: `console/src` (976 file changes in the last 120 commits),
`internal/httpapi` (183), and `internal/repository` (121). No application
source was changed. The review records **deepening opportunities** — refactors
that turn shallow modules into deep ones — and does not propose interfaces yet.

Vocabulary used throughout: **module**, **interface**, **implementation**,
**depth**, **seam**, **adapter**, **leverage**, **locality**.

Each candidate was checked against the tree at the revision above. Counts and
line references are from that tree.

## Summary

| Candidate | Strength |
| --- | --- |
| 1. Collapse the Admin read model onto one query dialect | Strong |
| 2. One invalidation policy, not two dialects | Strong |
| 3. Narrow the 409-method Repository to capabilities | Strong |
| 4. Give persistence one error vocabulary | Strong |
| 5. Resolve one Actor at the seam | Worth exploring |
| 6. Make handler behaviour testable without PostgreSQL | Strong |

## 1. Collapse the Admin read model onto one query dialect

**Strength:** Strong
**Files**

- [`console/src/features/admin/data/admin-read-model/index.ts`](../console/src/features/admin/data/admin-read-model/index.ts)
- [`console/src/features/admin/data/admin-read-model/adapters.ts`](../console/src/features/admin/data/admin-read-model/adapters.ts)
- [`console/src/api/queries/admin-telemetry.ts`](../console/src/api/queries/admin-telemetry.ts)
- [`console/src/api/queries/admin-control.ts`](../console/src/api/queries/admin-control.ts)

**Problem.** The read model does not compose the existing admin query hooks; it
re-implements them. `index.ts` imports only `api`, `cancellableQuery`,
`polledQuery` and `queryKeys` (lines 8–11) and issues ten raw `api.GET` calls.
The same endpoints already have typed hooks in `admin-telemetry.ts` and
`admin-control.ts`, so one HTTP request ends up with two cache keys:

- [`index.ts:134`](../console/src/features/admin/data/admin-read-model/index.ts#L134) keys the overview
  `queryKeys.adminOverview`, while
  [`admin-telemetry.ts:176`](../console/src/api/queries/admin-telemetry.ts#L176) keys it
  `[...queryKeys.adminOverview, options?.query ?? {}]`.
- [`index.ts:171`](../console/src/features/admin/data/admin-read-model/index.ts#L171) keys host metrics
  `[...adminHostMetrics, range]` (a range label such as `"1h"`) but sends
  `window_minutes: 60`; [`admin-telemetry.ts:209`](../console/src/api/queries/admin-telemetry.ts#L209)
  keys the identical request `[...adminHostMetrics, windowMinutes]`.

Two keys for one request means two cache entries and two pollers. The second
symptom is dead interface surface: **21** hooks exported by those two query
modules have no consumer, while the read model issues its own calls. The
adapter module itself is sound and tested (`adapters.test.ts`); `index.ts` is
the shallow part, because its interface is eleven bespoke hooks that each carry
their own window arithmetic, poll cadence and key shape.

**Solution.** Make `admin-read-model` the only Admin data module. The typed
query hooks become its internal adapter, or are deleted. The narrow interface
stays as it is — hooks returning `AdminReadState<T>` — but its implementation
composes one query dialect, with cache keys derived from request parameters
(minutes, not range labels) in one place.

**Wins.** Locality: window arithmetic, poll cadence and key shape change in one
file. Leverage: a cache-key fix reaches every Admin page at once. Tests: the
hooks become reachable with `renderHook` and a mocked `api`, as the repo
already does for mutations. Deletes 21 unreachable hooks.

## 2. One invalidation policy, not two dialects

**Strength:** Strong
**Files**

- [`console/src/api/cache-coherence/admin.ts`](../console/src/api/cache-coherence/admin.ts)
- [`console/src/api/cache-coherence/project.ts`](../console/src/api/cache-coherence/project.ts)
- [`console/src/api/mutations/admin.ts`](../console/src/api/mutations/admin.ts)
- [`console/src/api/mutations/sites.ts`](../console/src/api/mutations/sites.ts)

**Problem.** [`cache-coherence/index.ts:10`](../console/src/api/cache-coherence/index.ts#L10)
states the intent: both policies resolve to one vocabulary so mutation and
realtime adapters cannot grow different key lists. In practice the seam leaks.

- `adminInvalidationKeysFor` has **no production caller**; it appears only in
  `cache-coherence.test.ts` and the barrel export.
- [`mutations/admin.ts:112`](../console/src/api/mutations/admin.ts#L112) hardcodes
  `["admin", "telemetry", "errors"]` — exactly the drift the module exists to
  prevent — and the file hand-rolls **21** `invalidateQueries` calls.
- `mutations/sites.ts` mixes both dialects: it calls `applyCacheChanges` and
  raw `queryClient.invalidateQueries` inside the same mutations.

Callers must also know key internals to use the policy: `CacheChange` is a
105-line union and the test asserts prefix relationships by slicing other
factories (`.slice(0, 2)`), which makes the relationship implicit.

**Solution.** Keep one invalidation module, but give it a semantic and complete
interface: one exported operation, `invalidate(queryClient, change)`. Forbid
direct `invalidateQueries` calls in `mutations/*` with a lint rule. Either wire
`adminInvalidationKeysFor` into the Admin mutations and a real realtime adapter,
or delete it.

**Wins.** Locality: one place answers “what goes stale when an incident
changes”. Leverage: a new Admin resource touches one switch arm instead of 21
call sites. Tests assert behaviour instead of string-slicing other modules'
keys.

## 3. Narrow the 409-method Repository to capabilities

**Strength:** Strong
**Files**

- [`internal/repository/repository.go`](../internal/repository/repository.go)
- `internal/repository/*.go` — 409 exported methods across 91 files
- [`internal/httpapi/server.go:44`](../internal/httpapi/server.go#L44)
- [`internal/repository/function_worker.go`](../internal/repository/function_worker.go) — the seam that already works

**Problem.** `Repository` is one struct with 409 exported methods. `httpapi`
calls 289 distinct methods on it, and **292** methods have at most one calling
file. The package declares exactly one consumer-side interface in its whole
surface (`CloudflareOAuthClient`), and no repository interface at all. The
interface is therefore nearly as wide as the implementation: a signature change
is a package-wide compile event, and the contract cannot be held in one's head.

The seam is proven elsewhere. Every worker package already depends on a narrow
capability: `agentrunner.Persistence` (4 methods), `adminnotification` (3),
`realtimepublisher` (5), `monitoring` (3), `webhookrunner` (5),
`messagingrunner` (6), `appbuilder` (7), and `functionrunner` splits build (8)
from execution (5). The seam simply stops at the process boundary and never
reaches HTTP, the largest consumer.

The deletion test says this is a real module — delete it and SQL spreads across
twelve packages — so the fix is to narrow the interface, not to remove the
module.

**Solution.** Define per-domain capability interfaces next to the handlers that
use them (`ProjectStore`, `SiteStore`, `FunctionStore`, `AdminStore`,
`TelemetryStore`), each carrying only the methods its handler group calls.
`Repository` remains the production adapter behind all of them. The 292
single-caller methods become implementation detail instead of entries in a
public contract.

**Wins.** Locality: each handler group names its own persistence contract in a
small file beside it. Leverage: a capability signature change stops at that
capability. Tests: capabilities become fakeable (see candidate 6).

## 4. Give persistence one error vocabulary

**Strength:** Strong
**Files**

- [`internal/repository/errors.go`](../internal/repository/errors.go) — 16 of 129 sentinels
- `internal/repository/*.go` — 36 files declare the rest
- [`internal/httpapi/helpers.go:109`](../internal/httpapi/helpers.go#L109) — six near-identical mappers at 109, 117, 126, 138, 150, 162
- [`internal/repository/plans.go:19`](../internal/repository/plans.go#L19) — `PlanLimitError`, the pattern that works

**Problem.** The persistence-to-HTTP contract is 129 sentinel errors declared
across 36 repository files, of which `errors.go` holds only 16; discovering the
contract means grepping the package. `httpapi` contains 269 `repository.Err*`
references across 61 files, and handlers choose status codes themselves.

The same sentinel carries different HTTP meaning at different call sites:
`ErrNotFound` is a 404 for a project in one handler, a 401 for a session in
another, and a 404 for a membership in a third. No central mapper can be
written until the repository contract distinguishes those cases.

`PlanLimitError` shows the better shape — a structured error with `Is`, mapped
once in `planLimitError` — but it is the only such type.

**Solution.** Introduce a domain-level error taxonomy (kind, code, message) and
one `httpapi` mapper whose interface is a single call, `respondError(w, err)`.
Repositories return typed domain errors; handlers stop importing `repository`
for error inspection. Ambiguity is resolved at the source with distinct kinds
(`ErrSessionInvalid`, `ErrProjectNotFound`) instead of per call site.

**Wins.** Locality: the status/code/message table is one file. Leverage: a new
error kind is one taxonomy entry, not edits across 61 files. Tests: the mapper
is a pure table test.

> This narrows the repository contract, which `AGENTS.md` treats as an
> architecture change. Propose it before implementing.

## 5. Resolve one Actor at the seam

**Strength:** Worth exploring
**Files**

- [`internal/httpapi/middleware.go`](../internal/httpapi/middleware.go)
- [`internal/httpapi/server_admin.go:61`](../internal/httpapi/server_admin.go#L61)
- [`internal/httpapi/server_databases.go`](../internal/httpapi/server_databases.go)
- [`internal/httpapi/server_storage.go`](../internal/httpapi/server_storage.go)

**Problem.** Actor resolution is spread across six middlewares, five actor
shapes and seven context keys declared in three files.

- `requireProjectManagement` (lines 30–67) and `requireFunctionExecutionActor`
  (lines 117–153) contain the same API-key block: diffing them yields five
  differing lines out of 38. A fix to key validation or touch semantics must be made
  twice.
- `rateLimitActorID` must know all five actor representations to derive one
  bucket key (`projectActor`, `projectDataActor`, `repository.StorageActor`,
  `domain.ApplicationUser`, `domain.Account`). A sixth actor kind silently
  falls through to an anonymous IP bucket instead of failing to compile.
- The context accessors are unchecked type assertions: **28** sites, none
  guarded with `, ok`. A middleware ordering mistake panics, and the recovery
  middleware reports it as a generic 500, hiding the wiring bug.
- Admin authorization is re-checked in four places with two predicates —
  account-only `IsInstanceAdmin` and the safer `IsInstanceAdminSession` — and
  the route middleware uses the weaker one.

**Solution.** One auth module whose interface is a single resolved value:
`actor(r)` returning a typed `Actor` (kind, subject, project scope, scopes),
produced by one resolution chain under one key. Rate limiting, handlers and the
database/storage actor derivation consume `Actor` instead of re-reading five
keys. API-key resolution becomes one helper. Accessors return `(Actor, bool)`
so wiring errors become 401s rather than panics.

**Wins.** Locality: “who is calling and what may they do” is one file.
Leverage: a new actor kind touches one union arm, and the rate limiter cannot
forget it. Tests: the existing probe router becomes a table-driven matrix over
one seam.

## 6. Make handler behaviour testable without PostgreSQL

**Strength:** Strong
**Files**

- [`internal/httpapi/testmain_test.go`](../internal/httpapi/testmain_test.go)
- `internal/httpapi/*_test.go` — 32 files require `TEST_DATABASE_URL`
- [`internal/repository/admin_alert_evaluator.go:23`](../internal/repository/admin_alert_evaluator.go#L23) — the tested kernel
- [`internal/repository/admin_alert_evaluator.go:82`](../internal/repository/admin_alert_evaluator.go#L82) — the untested wrapper

**Problem.** The handler layer has no substitute adapter, because the
dependency is 409 methods wide. Every handler behaviour test either exercises a
pure helper or stands up PostgreSQL with migrations and a bootstrap owner.
There are only two fakes in the whole package.

The asymmetry is diagnostic: the only handler tests that use a fake are the
ones whose dependency is narrow — `fakeAdminTelemetryStore` exists because
`telemetry.Store` has five methods.

The same split appears inside the repository. `transitionAdminAlert` is pure
and unit-tested, while `evaluateAdminAlertTx` — row locking, conditional
update, event insert, notification enqueue, realtime enqueue, and the
monitor-to-rule lock ordering — is observable only through a database. The
pure function is easy to test and passes; the behaviour that causes incidents
lives in how it is called.

The Console repeats the pattern: `adapters.ts` is covered, while `index.ts`,
where queries, keys, polling and live merge compose, has no test.

**Solution.** Introduce the capability interfaces from candidate 3 and provide
a fake per handler group. Handler tests then assert observable HTTP behaviour
against a fake adapter, and the PostgreSQL suite becomes a conformance suite —
one per capability — instead of the only way to observe handler logic.

**Wins.** Locality: behaviour is tested where it lives. Leverage: one
capability fake unlocks dozens of handler tests. The database suite shrinks to
contract checks, and the untested transaction wrapper gets covered.

## Top recommendation

Tackle **candidate 4** first, as the opening slice of candidate 3.

It has the highest fan-in of any candidate — 269 checks across 61 files — and
it is mechanically safe: each `errors.Is` branch becomes a typed error kind
with the same status and code. It also forces the repository contract to be
named, which is the prerequisite for the capability interfaces in candidate 3
and the fakes in candidate 6. Splitting the Repository in one step is a
high-risk change to review; a taxonomy plus one mapper lands first and unlocks
the rest.

Candidates 1 and 2 are lower risk and independently valuable — candidate 2's
Admin half can be closed by deleting dead code and routing the Admin mutations
through the existing policy — but they do not unblock anything in the backend.

## Not proposed

- Splitting files further for line count alone. The recent module splits in
  `httpapi` and `repository` are consistent with the existing convention, and
  length alone is not a finding under `AGENTS.md`.
- Adding a second telemetry store. The request index is PostgreSQL-backed and
  the external store was removed deliberately.
