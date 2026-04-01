## Plan: Full Portfolio Polish & Refactor

Aggressively review and refine the entire repo in phased passes: establish a baseline, standardize frontend design patterns across every page, close accessibility/UX gaps, simplify backend architecture and duplicated logic, remove dead code, and continuously validate with `just compose` after significant changes plus `just ci` quality gates.

**Steps**

### Phase 0 — Baseline, guardrails, and inventory

1. Capture current baseline health and architecture snapshot: run `just ci`, record current test count/failures, and enumerate touched domains (frontend pages, shared CSS/JS, backend soccer/google/lps/app). This blocks all later phases.
2. Build a master review matrix (page-by-page + package-by-package) so every area is explicitly reviewed: Home, About, Experience, Education, Skills, Projects, Contact, Soccer, shared layout/partials, and all `internal/*` packages.
3. Define canonical style/interaction contracts before edits: buttons, card shells, shadows, hover/focus states, form controls, status/alert patterns, motion behavior, and responsive breakpoints. This blocks Phase 1 and Phase 2.
4. Establish aggressive redeploy cadence agreed in alignment: run `just compose` after every significant change cluster (not only phase boundaries), with smoke checks for `/`, `/projects`, `/skills`, `/contact`, `/soccer`.

### Phase 1 — Frontend consistency and design-system consolidation

5. Normalize shared visual primitives into canonical reusable rules in shared CSS (button variants, interactive card pattern, icon-wrap, section labels, form feedback states) and replace page-local duplicates. (*parallelizable by page family after shared primitives land*)
2. Perform page-by-page consistency pass (tone, hierarchy, spacing, CTA style, hover/focus treatments, shadows, borders, empty states, messaging language) for all pages and soccer fragments. (*depends on 5*)
3. Standardize UI behavior patterns in JS and templates (modal open/close/focus consistency, loading states, table interactions, filter controls keyboard behavior, reusable status message semantics). (*depends on 5, parallel with 6 where isolated*)
4. Complete responsive/reflow harmonization for 320px and mobile/tablet breakpoints, ensuring grid collapse and no horizontal reading scroll across all content sections. (*depends on 6*)
5. Run `just compose` + manual smoke pass after each major UI cluster (global styles; core pages; soccer flows) and correct regressions immediately. (*continuous with steps 5-8*)

### Phase 2 — Accessibility hardening across the full UI

10. Execute explicit WCAG 2.2 AA-oriented remediation sweep: landmarks/headings, keyboard operability, visible focus, labels/accessibility names, form errors (`aria-invalid` + descriptors), color/contrast, reduced motion, and forced-colors behavior. (*depends on 5-8*)
2. Validate modal/dialog and table interaction edge cases (Escape handling, focus restoration, skip-link behavior, sticky headers layering, non-color-only status cues). (*depends on 10*)
3. Re-run `just compose` with accessibility smoke scenarios after each significant remediation batch. (*continuous with 10-11*)

### Phase 3 — Backend maintainability and simplification refactor

13. Start low-risk simplifications: consolidate duplicated form parsing and repeated handler error mapping; unify shared cookie/session handling helpers where behavior is equivalent. (*can start in parallel with late Phase 2 once stable UI baseline exists*)
2. Execute medium-risk extraction refactors: isolate calendar sync decision logic, consolidate soccer schedule resolution pathways, and split overloaded LPS resolver responsibilities while preserving behavior. (*depends on 13*)
3. Execute aggressive architecture cleanup: reduce/reshape bidirectional soccer↔google coupling into a clearer coordination model, then split oversized handler responsibilities into focused services. (*depends on 14; highest risk*)
4. After each backend change cluster, run `just test`, `just vet`, `just build`, then `just compose` smoke checks on soccer and Google-related routes. (*continuous with 13-15*)

### Phase 4 — Dead code elimination, standards conformance, and docs

17. Perform dead/stale code review repo-wide: remove unused helpers, stale copy paths, duplicate CSS declarations, and redundant abstractions not aligned with current flow. (*depends on 6 and 13 to avoid deleting active migration bridges*)
2. Standards pass for complexity and conventions: simplify over-complex functions, tighten naming, keep behavior-preserving refactors surgical, and remove non-essential duplication.
3. Refresh docs for final reality: README, PROGRESS, and deployment notes to reflect canonical styles, current soccer flow, verification workflow, and compose cadence.

### Phase 5 — Final verification and release readiness

20. Full validation gate: `just ci` + repeated `just compose` smoke walkthrough of all pages and key flows (soccer auth import, schedule fetch, ICS download, Google connect/add/disconnect where enabled).
2. Final consistency and accessibility sign-off checklist across every page and component, ensuring visual and interaction parity and no high-priority regressions.
3. Prepare implementation handoff/PR structure by phase to keep reviewable chunks and reversible commits.

**Relevant files**

- `/Users/craigjohnson/repos/portfolio/cmd/web/layouts/base.templ` — landmark, skip-link, shared page wrapper consistency.
- `/Users/craigjohnson/repos/portfolio/cmd/web/pages/*.templ` — page-level tone/style/structure consistency and content hierarchy.
- `/Users/craigjohnson/repos/portfolio/cmd/web/partials/*.templ` — reusable fragments, modal/table/login/select consistency.
- `/Users/craigjohnson/repos/portfolio/cmd/web/static/css/styles.css` — canonical tokens, shared button/card/form/focus/animation utilities.
- `/Users/craigjohnson/repos/portfolio/cmd/web/static/css/*.css` — page-specific styles to deduplicate and align with shared primitives.
- `/Users/craigjohnson/repos/portfolio/cmd/web/static/js/main.js` — modal, nav, filter, keyboard/focus behavior consistency.
- `/Users/craigjohnson/repos/portfolio/internal/soccer/auth.go` — auth/session UI flow and handler simplification opportunities.
- `/Users/craigjohnson/repos/portfolio/internal/soccer/schedule.go` — schedule resolution duplication and error mapping cleanup.
- `/Users/craigjohnson/repos/portfolio/internal/soccer/helpers.go` and `/Users/craigjohnson/repos/portfolio/internal/soccer/form.go` — shared parser and helper deduplication.
- `/Users/craigjohnson/repos/portfolio/internal/google/calendar.go` and `/Users/craigjohnson/repos/portfolio/internal/google/oauth.go` — calendar sync complexity and service responsibility boundaries.
- `/Users/craigjohnson/repos/portfolio/internal/google/cookies.go` — cookie/session utility consolidation with soccer/session patterns.
- `/Users/craigjohnson/repos/portfolio/internal/lps/resolver.go` and `/Users/craigjohnson/repos/portfolio/internal/lps/decode.go` — mapping/fetching responsibility split and complexity reduction.
- `/Users/craigjohnson/repos/portfolio/internal/app/server.go` and `/Users/craigjohnson/repos/portfolio/internal/app/soccer_bridge.go` — cross-package wiring and decoupling strategy.
- `/Users/craigjohnson/repos/portfolio/internal/app/*_test.go`, `/Users/craigjohnson/repos/portfolio/internal/google/*_test.go`, `/Users/craigjohnson/repos/portfolio/internal/lps/*_test.go`, `/Users/craigjohnson/repos/portfolio/internal/schedule/*_test.go` — regression protection during refactor.
- `/Users/craigjohnson/repos/portfolio/README.md`, `/Users/craigjohnson/repos/portfolio/PROGRESS.md`, `/Users/craigjohnson/repos/portfolio/DEPLOY-INSTRUCTIONS.md` — documentation updates.
- `/Users/craigjohnson/repos/portfolio/justfile` — command source of truth for validation cadence.

**Verification**

1. Baseline and final: run `just ci` and compare test/build/lint outcomes to ensure no regressions.
2. Iterative backend verification: after significant backend changes, run `just test`, `just vet`, and `just build` before compose smoke tests.
3. Iterative frontend verification: after significant UI/style clusters, run `just build` then `just compose` and verify every page visually.
4. Accessibility verification each phase: keyboard-only traversal, focus visibility, skip-link behavior, form error behavior, and modal focus lifecycle checks.
5. Contrast and non-color cue checks across status/interactive states, including hover/focus and message variants.
6. Forced-colors and reduced-motion behavior checks for shared components and soccer-heavy interactions.
7. Reflow checks at 320px to ensure no two-dimensional reading scroll for multi-line content.
8. Functional smoke checks for soccer flow and Google integration paths after each relevant change batch via `just compose`.

**Decisions**

- Refactor depth: Aggressive (includes deeper architecture cleanup, not only superficial polish).
- Design-system scope: Full consolidation of duplicated patterns across pages/components.
- Compose cadence: `just compose` after every significant change cluster.
- Definition of done includes: full visual consistency pass, accessibility pass, code simplification/dead-code cleanup, expanded tests where critical paths change, docs refresh, and no high-priority lint/test/build regressions.
- Included scope: full codebase and full UI/UX consistency + maintainability review.
- Excluded scope: new product features unrelated to polish/refactor, and major platform migration away from current server-rendered Templ/HTMX architecture.

**Further Considerations**

1. For Phase 3 decoupling strategy, choose one target model early (coordinator service vs event-driven) to avoid partial rewrites.
2. Keep refactors split into small reversible commits per sub-phase to preserve rollback safety.
3. Prioritize high-risk regression zones (OAuth state handling, calendar dedup IDs, session encryption behavior) with focused test expansion before deep structural edits.
