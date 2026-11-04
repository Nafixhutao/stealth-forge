# HANDOFF — Server-side session guard for the Console (DEPLOYED LOCALLY, NOT PUSHED)

Updated: 2026-10-30. Written for the next agent picking this up. Read this
before touching the working tree.

## State summary

- Branch `feat/server-session-guard` has the work in **2 commits**
  (`feat(console): route visitors by their session…` and
  `fix(ingress): expect the console root redirect to land on sign-in`).
  **Not pushed** — the owner asked to keep it off GitHub for now.
- Deployed to the live installation at `/home/ubuntu/.stealth` (CLI `stealth
  status` reports it): images `stealth-console:v26-session-guard-20261030` and
  `stealth-ingress-control:v26-session-guard-20261030` built from the final
  commit; `config.env` updated (backup:
  `config.env.before-session-guard-20261030`); only the console container was
  recreated (ingress-control is a maintenance-profile one-shot service).
- Live verified on 2026-10-30: `/` → 307 `Location: /login`; `/login` → 200;
  anonymous `/admin` → 307 `Location: /login?next=%2Fadmin`; console container
  and `stealth status` healthy.
- Added on the same branch (commit `712b054`): the user console dark palette is
  anchored on `#1A181D` (`--projects-bg`/`--projects-surface`), cards lift to
  `#201e26` and controls to `#24222b`, and the shared DesktopSidebar is now an
  elevated `#151318` panel with a **rounded right edge** (`rounded-r-xl`,
  soft `#2b2933` hairline) — the canvas shows through the corners. The
  light-theme literal-class remap in `admin.css` follows the renamed classes.
  Deployed live as `stealth-console:v27-palette-1a181d-20261030`
  (backup: `config.env.before-palette-20261030`). Admin console untouched by
  design.
- Refined on the same branch (commit `4cd9162`), deployed as
  `stealth-console:v28-floating-panel-20261030`
  (backup: `config.env.before-floating-panel-20261030`): the reference layout
  is now correct — the whole shell sits on the darkest canvas `#100f13`, the
  nav rail is flush with that canvas (no own panel tone, no divider), and the
  page content is the rounded `#1a181d` card (`rounded-2xl`, `#26242c`
  hairline) inset top/left/right, scrolling INSIDE itself via
  `lg:sticky lg:top-2 lg:h-[calc(100dvh-1rem)] lg:overflow-y-auto` in
  `application-shell.tsx`. Mobile keeps full bleed. Light mode: the shell ROOT
  needs its own selector (`[data-user][class~="bg-[#100F13]"]` → `#ececef`)
  because descendant remaps never reach it.
- Restyled on the same branch (commit `ffdbc3e`), deployed as
  `stealth-console:v29-geist-sidebar-20261030`
  (backup: `config.env.before-geist-sidebar-20261030`): the console font is now
  **Geist Variable** everywhere (`--font-sans` in globals.css `@theme`, the
  body alias renamed to `--font-ui-variable`, and the `[data-user]` token
  block; Inter's cv01/ss03 feature settings removed). The navigation is
  restyled to the reference: rounded 40px rows inset `mx-2` (44px under lg),
  all nav text 14px, a bordered Search field with a `Ctrl K` kbd hint
  (borderless icon square when the rail is collapsed), a muted "Projects"
  section label, a 24px circular header avatar, and a hairline divider above
  the profile footer. The light-theme literal remaps cover the new classes.
- Sharpened on the same branch (commit `4841f58`), deployed as
  `stealth-console:v30-reference-type-20261030`
  (backup: `config.env.before-reference-type-20261030`): base text on
  `[data-user]` is now the reference spec verbatim (Geist Variable with emoji
  fallbacks, 14px/20px, weight 400, `#efedf0`); the search field is a filled
  (white/4%) borderless rounded field with `Ctrl K` as plain muted text; nav
  rows take the base ink at a ~42px pitch with 20px icon insets. Light theme
  remaps added for `text-[#efedf0]`, `bg-white/[0.04]`, and the 0.04–0.06
  hovers so nothing vanishes on the pale panel.
- Agent surface added on the same branch (commit `4e9a973`, "add the Agent
  conversation surface"), deployed as
  `stealth-console:v34-agent-surface-20261101`. `/agent` is a chat layout
  matching the supplied reference: agent pill + star + options in a 56px
  header, day-separated thread (prompt right, reply left with copy), reply box
  pinned at the bottom with a skills menu and send button, `Agent` in the
  bottom-right corner. It reads `/v1/agents`, `/v1/agents/{id}/runs`, and
  `/v1/agent-catalog`, is guarded like `/user` (proxy matcher +
  `isGuardedConsolePath` + its own `SessionGate` layout), and is opened from
  the **Coding Agent** row (`NavItem.href`). Sending is gated on the run list's
  `can_manage`; a queued run states that the installation has no execution
  worker instead of faking a reply.
- Icons on the same branch (commit `5da87a7`, "shrink the sidebar glyphs to
  16px and name the person"), deployed as
  `stealth-console:v33-icons-16-20261101` (backup:
  `config.env.before-icons-16-20261101`): every sidebar glyph — nav rows,
  Slack mark, search, bell, both chevrons, panel toggle — draws in a 16px box,
  and the header drops the generated "<name>'s organization" suffix via
  `organizationDisplayName` in `lib/format.ts`.
- Sidebar rebuilt to the reference on the same branch (commit `e736e0f`,
  "rebuild the user sidebar to the reference spec"), deployed as
  `stealth-console:v32-sidebar-reference-20261101`
  (backup: `config.env.before-sidebar-reference-20261101`): 264px panel on
  `#121014`, 248x32 filled search, 240x32 rows on a 2px pitch, the
  Triage -> Slack Bot order, the Scopes / Organization settings group and the
  pinned 59px profile footer. Live CSS verified to carry the new palette
  (`#171519`, `#161418`, `#1C1A1F`, `#EEEDF0`); console healthy on
  `https://console.nazxf.my.id`.
- Pixel-aligned to the reference on the same branch (commit `6fe3068`),
  deployed as `stealth-console:v31-nav-reference-20261030`
  (backup: `config.env.before-nav-reference-20261030`). Verified with
  `agent-browser` (skill from vercel-labs/agent-browser) against a mock API at
  `/tmp/opencode/mockapi.py`, 1440x900, using computed geometry rather than
  eyeballing. Final numbers: 36px rows with no inter-row gap, 12px insets
  (`mx-3`), 10px radius, 40px search field, 17px/1.6 nav icons, section label
  aligned to the icon column (`mx-3 px-3`), active pill at 4.5% white, plan
  chip inline after the name with a **working** "Switch organization"
  chevron button. The rail switches geometry through `rowWidthClass`
  (centered 40px square, label hidden) instead of inheriting the panel inset.

## Context

Branch `fix/sign-in-provider-hint` and its PR #26 are **merged**; the sign-in
provider-hint fix is done and unrelated to what is below. The current uncommitted
working tree is a **separate, new feature**: resolving the Console session on the
server so anonymous visitors stop being handed `/admin` URLs before the browser
discovers they are signed out.

## What this changes (all uncommitted)

New files:

- `console/src/proxy.ts` — Next.js 16 proxy (`src/proxy.ts`, named export
  `proxy`, Node runtime — verified against Next 16 docs via Context7). Guards
  `/admin/*` and `/user/*` before HTML is sent: anonymous → `/login?next=…`
  (destination preserved and later validated by `getSafeNextPath`), non-admins
  pushed off `/admin` to `/user`, admins pushed off `/user` to `/admin`,
  redirect decisions through the pure `consoleRedirectFor`.
- `console/src/features/auth/session.server.ts` — extracts the internal API
  probe (`STEALTH_API_INTERNAL_URL`, default `http://stealth-api:8080`) into
  `resolveSession(cookieHeader)` returning `authenticated(role) | anonymous |
  unavailable`. 401 → anonymous; expired cookie, network error, timeout (2 s),
  non-401 error status, or unparsable body → `unavailable` (unknown, not
  absent). Plus `resolveSessionCached`: a bounded (1000 entries max, oldest
  evicted) 5 s-per-cookie-header cache used only by the proxy, which otherwise
  probes the account API on every request including RSC prefetch bursts.
- `console/src/features/auth/session.server.test.ts` — unit tests for the
  status mapping and the cache (TTL, per-cookie keying, eviction).
- `console/src/test/server-only.ts` + a `server-only` alias in
  `vitest.config.ts` — the `server-only` guard throws under vitest's Node
  resolution, so unit tests need this stub. Production builds keep the real
  package.

Modified files:

- `console/src/app/page.tsx` — root resolves the session itself:
  authenticated → `consoleHomeForRole`, anonymous → redirect `/login` (this is
  what the ingress probes and smoke script assert: 307, Location `/login`, no
  query), `unavailable` → renders a small "Console is unreachable / Retry"
  page instead of redirecting (an unknown session must not strand a signed-in
  operator on `/login`, matching the proxy's rule).
- `console/src/features/auth/authenticated-redirect.server.ts` — now uses
  `resolveSession`; behavior unchanged (fails open).
- `console/src/features/auth/console-home.ts` — pure routing rule
  (`isInstanceAdminRole`, `consoleHomeForRole`, `isGuardedConsolePath`,
  `consoleRedirectFor`), unit-tested in `console-home.test.ts`.
- `internal/ingresscontrol/probe.go` + `probe_test.go` — the local Traefik
  preflight now expects the console root to redirect to `/login` (not
  `/admin`) and to answer `/login` with a document.
- `scripts/compose-production-smoke.sh` — the ingress parity and
  restart checks now assert Location `/login` where they asserted `/admin`.
- `console/src/components/session-gate.tsx`,
  `console/src/app/user/layout.tsx` — comments updated to the decided rule.

## Decisions already made (do not reopen casually)

1. **Admins must not visit `/user` pages.** Confirmed intended by the user
   (2026-10-30). The proxy bounces admins to `/admin`; `/user` is for
   signed-in non-admins only. This is asserted in `console-home.test.ts`
   ("moves an admin off the customer console").
2. `unavailable` ≠ `anonymous`. The proxy and the console root never treat an
   unknown session as signed out.
3. The proxy probe may serve cached decisions up to 5 s old. Session
   revocation inside that window is still caught by the client-side
   SessionGate/AdminGate, which re-check via react-query (`staleTime` 60 s).
4. The client gates stay in place as defense in depth; the API remains the
   authority.

## Validation already run (all green)

- `go test ./internal/ingresscontrol/ -count=1` — ok
- `cd console && npm run typecheck` — ok
- `cd console && npm run lint` (eslint) — ok
- `cd console && npx prettier --check` on all touched files — ok
- `cd console && npm run test` — 42 files / 312 tests passed (includes the new
  session.server and console-home suites)

## Remaining work

1. Push `feat/server-session-guard` and open the PR when the owner says so.
2. Full `go test ./...` and the production smoke script (`scripts/compose-
   production-smoke.sh`) were NOT re-run against the live deploy; the smoke
   script now expects the console root to 307 to `/login`, which the live
   checks above confirm behaviorally.
3. Do NOT commit `HANDOFF.md` with the feature work — it is a working-tree
   aid; delete it once the next agent takes over.
