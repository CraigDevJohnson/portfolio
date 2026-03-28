## Plan: Split package main into clean layered packages

Refactor the current root-level `package main` monolith into a Go-standard `cmd/server + internal/...` architecture, using your `portfolio/` folder as the structural model. Preserve behavior by default, allow only explicitly-reviewed minor behavior improvements, and execute in small PR-sized phases with green checks after each phase. This plan also relocates Templ/static to a `cmd/web` layout and standardizes duplicated logic (session/cookies/request limits/error mapping) behind cohesive package boundaries.

**Steps**

### Phase 0 — Baseline and guardrails

1. Capture baseline architecture, behavior, and build/test expectations from `/Users/craigjohnson/repos/portfolio/PRD.md`, `/Users/craigjohnson/repos/portfolio/PROGRESS.md`, `/Users/craigjohnson/repos/portfolio/README.md`, and `/Users/craigjohnson/repos/portfolio/justfile`.
2. Freeze baseline verification artifacts before first refactor commit: `just ci`, route map, and smoke checks for soccer auth + schedule fetch + ICS + Google add flow.
3. Create a migration checklist documenting required parity for current handlers and security behavior (encrypted cookies, rate limiting, HTTPS-aware cookie flags).

### Phase 1 — Create target skeleton (parallel-safe with no behavior changes)

4. Create package skeleton with no logic movement yet:
   - `cmd/server`
   - `cmd/web`
   - `internal/app`
   - `internal/config`
   - `internal/httpx` (or `internal/web`)
   - `internal/session`
   - `internal/portfolio`
   - `internal/soccer`
   - `internal/lps`
   - `internal/schedule`
   - `internal/google/oauth`, `internal/google/calendar`, `internal/google/store`
2. Introduce an application wiring struct (dependency container) in `internal/app` and define constructor contracts, but keep existing handlers functional via adapters.
3. Move `main()` bootstrapping to `cmd/server/main.go` and keep a temporary compatibility entry in root until migration is complete; remove compatibility entry in final phase.

### Phase 2 — Move pure/domain logic first (lowest risk)

7. Move schedule/time/ICS pure logic into `internal/schedule` from:
   - `schedule.go`
   - `schedule_time.go`
   - `schedule_ics.go`
   - `schedule_errors.go`
   Keep API signatures stable via temporary wrapper functions in `package main`.
2. Move LPS request/decoding/resolution logic into `internal/lps` from:
   - `lps_client.go`
   - `lps_decode.go`
   - `lps_schedule.go`
   Maintain current behavior for `/users/check` discovery and team schedule fetching.
3. Move portfolio data and rendering helpers into `internal/portfolio` from:
   - `data_portfolio.go`
   - portfolio route handlers from `handlers_portfolio.go`.

### Phase 3 — Session/config/http utilities consolidation

10. Move config and runtime env handling to `internal/config` (`loadServerConfig`, session key validation, Google toggles), replacing global mutable access with injected config.
2. Move and standardize session/cookie logic in `internal/session` and `internal/httpx`:

- consolidate cookie creation (`HttpOnly`, `Secure`, `SameSite`, scoped path)
- keep AES-GCM encrypted session/token storage
- preserve login rate limiter semantics.

12. Standardize repeated constants and request limits across packages (single source of truth for request body limits, upstream body limits, soccer cookie path, default game duration).

### Phase 4 — Split HTTP handlers by domain with dependency injection

13. Migrate soccer handlers into `internal/soccer` sub-files (`auth`, `session state`, `schedule`, `download`, `google glue`) as thin orchestration layers.
2. Migrate Google OAuth and Calendar logic into `internal/google/oauth`, `internal/google/calendar`, and `internal/google/store`; inject store and clients through constructors.
3. Rebuild route registration in `internal/app` (or `internal/httpx/routes`) so `cmd/server/main.go` only initializes config/dependencies and starts server.
4. Remove now-redundant wrappers and duplicate logic after each group migration; keep one canonical implementation per concern.

### Phase 5 — Move web assets/components to cmd/web

17. Relocate Templ sources from `/components/...` to `/cmd/web/{layouts,pages,partials}` with import updates.
2. Relocate static asset serving toward `cmd/web` layout (and optional embed path if chosen during execution), preserving URLs used by templates (`/static/...`) for behavior parity.
3. Update generation/build pipeline in `justfile` so `just generate`, `just build`, `just test`, and `just ci` remain the canonical workflows.

### Phase 6 — Test restructure and cleanup hardening

20. Align tests with new package boundaries:

- move handler-focused tests beside new handler packages
- keep integration-style `httptest` coverage for soccer/LPS/Google flows
- keep helper fakes in package-local test helper files.

21. Remove stale imports, aliases, dead wrappers, and duplicated formatting/mapping code after each package migration.
2. Final pass for naming consistency and package cohesion (single-responsibility file purpose, minimal cross-package leakage).

### Phase 7 — Finalization and compatibility cleanup

23. Remove temporary compatibility shims in root `package main` once all routes and services are served from new packages.
2. Ensure root directory contains only minimal entrypoint artifacts and project-level files; runtime logic should live under `internal/...`.
3. Update docs (`README.md`, deployment notes if affected) with the new structure and where to find handlers/logic.

**Parallelism and dependencies**

- Parallel-safe early work: Phase 1 skeleton, Phase 2 portfolio extraction, and schedule pure logic extraction.
- Strict dependency chain: Phase 3 config/session injection must precede full Phase 4 handler migration.
- Asset relocation (Phase 5) should begin only after route/handler package boundaries stabilize to avoid compounding breakage.
- Test restructuring (Phase 6) can run incrementally in lockstep with Phases 2–5 and finalize at the end.

**Relevant files**

- `/Users/craigjohnson/repos/portfolio/main.go` — current bootstrap and route registration to relocate into `cmd/server` + app router wiring.
- `/Users/craigjohnson/repos/portfolio/config.go` — env/config globals to convert into injected `internal/config` package.
- `/Users/craigjohnson/repos/portfolio/session.go` and `/Users/craigjohnson/repos/portfolio/cookies.go` — secure session and cookie behavior to preserve while modularizing.
- `/Users/craigjohnson/repos/portfolio/handlers_portfolio.go` and `/Users/craigjohnson/repos/portfolio/data_portfolio.go` — portfolio page/domain split target.
- `/Users/craigjohnson/repos/portfolio/handlers_soccer.go` — main orchestration surface to thin and split by concern.
- `/Users/craigjohnson/repos/portfolio/lps_client.go`, `/Users/craigjohnson/repos/portfolio/lps_decode.go`, `/Users/craigjohnson/repos/portfolio/lps_schedule.go` — LPS package extraction source.
- `/Users/craigjohnson/repos/portfolio/schedule.go`, `/Users/craigjohnson/repos/portfolio/schedule_time.go`, `/Users/craigjohnson/repos/portfolio/schedule_ics.go`, `/Users/craigjohnson/repos/portfolio/schedule_errors.go` — schedule domain package extraction source.
- `/Users/craigjohnson/repos/portfolio/google_oauth.go` and `/Users/craigjohnson/repos/portfolio/google_calendar.go` — Google package extraction source.
- `/Users/craigjohnson/repos/portfolio/components/layouts/base.templ` and `/Users/craigjohnson/repos/portfolio/components/{pages,partials}` — source to migrate into `cmd/web` layout.
- `/Users/craigjohnson/repos/portfolio/static/` — static asset organization target for `cmd/web` structure while preserving public URLs.
- `/Users/craigjohnson/repos/portfolio/justfile` — command/update anchor for generation, test, lint, and build.
- `/Users/craigjohnson/repos/portfolio/PRD.md` and `/Users/craigjohnson/repos/portfolio/PROGRESS.md` — behavior and completion references to avoid regressions.

**Verification**

1. After each phase: run `just test` and `just build`; for integration-impact phases run `just ci`.
2. Route-level smoke checks after each handler migration:
   - portfolio pages render
   - `/soccer/session`, `/soccer/import`, `/soccer/fetch`, `/soccer/download` behavior remains valid
   - Google connect/callback/disconnect/calendar add endpoints still function when enabled.
3. Security parity checks after session/config migration:
   - cookies keep `HttpOnly`, correct `Secure` behavior, and appropriate `SameSite`
   - encrypted session/token round trips pass existing tests
   - login rate limit behavior unchanged.
4. Templ/static migration checks:
   - `just generate` succeeds from new paths
   - templates render with expected imports
   - static asset URLs still resolve and page styling/scripts remain intact.
5. Final structure check:
   - server runtime logic is in `internal/...`
   - `cmd/server/main.go` is minimal and declarative
   - no duplicate/wrapper-only legacy code remains.

**Decisions**

- Use `cmd/server + internal/...` as the primary target layout.
- Include Templ/static relocation to `cmd/web` style as part of the main plan (not optional-only).
- Allow minor behavior improvements only when explicitly reviewed and documented at PR level.
- Deliver as many small PR-sized increments with green tests/build at each step.

**Scope boundaries**

- Included: package architecture refactor, handler/domain extraction, deduplication/standardization, web asset/component relocation, and docs updates needed for new structure.
- Excluded unless explicitly requested during execution: feature additions, UI redesign, API contract changes, infrastructure/Terraform redesign, or broad business-logic rewrites unrelated to structural cleanup.

**Further Considerations**

1. Decide during execution whether to keep disk-served static files or switch fully to embedded assets; recommendation: keep URLs stable first, embed in a final hardening pass.
2. Choose final naming convention for HTTP utility package (`httpx` vs `web`); recommendation: pick one early and enforce consistently.
3. Decide whether to keep transitional compatibility wrappers for one phase or remove immediately once tests pass; recommendation: keep wrappers briefly to reduce merge risk, then remove promptly in dedicated cleanup commits.
