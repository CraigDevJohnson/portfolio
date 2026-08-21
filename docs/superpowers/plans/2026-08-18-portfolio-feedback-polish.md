# Portfolio Feedback Polish Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refine Home, About/shared motion, Experience, Skills, and Contact in response to the 2026-08-18 visual review while preserving the successful Emberglass redesign.

**Architecture:** Keep the existing Go/Templ/Tailwind page ownership. Remove redundant Home content, simplify the shared signal primitive, turn Experience technology into a route-owned panel, add multi-tag Skills discovery with quieter filters, and add a typed SVG icon primitive for Contact. Generated Templ and Tailwind outputs remain build products.

**Tech Stack:** Go, Templ, HTMX, Tailwind CSS v4 source files, vanilla JavaScript, repository style-contract tests.

**Spec:** `docs/superpowers/specs/2026-08-18-portfolio-feedback-adjustments-design.md`

## Global Constraints

- Preserve Night Mulberry `#17121B`, Cocoa Cedar `#2E2130`, Candle Oat `#FFF0D8`, Campfire Apricot `#FFA677`, Rosehip `#FF7FA8`, and Pond Mint `#78E3C3`.
- Preserve Bricolage Grotesque, Atkinson Hyperlegible, and IBM Plex Mono roles.
- Do not edit generated `*_templ.go` or `cmd/web/static/css/tailwind.css` by hand.
- Preserve unrelated dirty-worktree changes and patch only files named by each task.
- Projects, Education, Soccer behavior, and management behavior are outside this plan except shared-component regression checks.
- Run `task generate` after every `.templ` edit and `task fmt` after Go edits.

---

### Task 1: Make the shared signal trail static and node-free

**Files:**
- Modify: `cmd/web/partials/ui_signal.templ`
- Modify: `cmd/web/tailwind/components.css`
- Modify: `cmd/web/tailwind/pages/experience.css`
- Modify: `internal/app/experience_style_contract_test.go`
- Modify: `cmd/web/partials/ui_types_test.go`

**Interfaces:**
- Consumes: `SignalTrail(SignalTrailProps)` and existing trail variant classes.
- Produces: the same `data-signal-trail` root and SVG path interface, without `.signal-trail-node` elements or motion keyframes.

- [ ] **Step 1: Add a failing shared-markup test**

  Extend `TestSignalTrail` in `cmd/web/partials/ui_types_test.go` to require exactly
  two paths, zero `circle` elements, and the existing `data-signal-trail` marker.

- [ ] **Step 2: Run the focused tests and confirm the old nodes fail the contract**

  Run: `go test ./cmd/web/partials ./internal/app -run 'SignalTrail|Experience.*Trail'`

  Expected: failure because the current component renders three circles and the
  Experience contract still requires a route override that hides them.

- [ ] **Step 3: Remove node markup and animation ownership**

  Delete the three circles from `ui_signal.templ`. In `components.css`, remove
  `.signal-trail-node*`, `emberglass-signal-drift`,
  `emberglass-node-breathe`, the dash array, and both animation declarations.
  Keep `.signal-trail-line` as a solid gradient stroke and keep the blurred
  shadow. Remove the now-dead Experience selector that hides nodes at `80rem`.

- [ ] **Step 4: Update the Experience style contract to the new shared primitive**

  Remove node-visibility fixtures and mutation cases from
  `experience_style_contract_test.go`; retain assertions for trail placement,
  forced colors, and SVG visibility.

- [ ] **Step 5: Regenerate and verify**

  Run: `task generate`

  Run: `go test ./cmd/web/partials ./internal/app -run 'SignalTrail|Experience.*Trail'`

  Expected: PASS.

- [ ] **Step 6: Commit the shared primitive**

  ```sh
  git add cmd/web/partials/ui_signal.templ cmd/web/tailwind/components.css cmd/web/tailwind/pages/experience.css cmd/web/partials/ui_types_test.go internal/app/experience_style_contract_test.go
  git commit -m "fix(ui): simplify signal trail treatment"
  ```

### Task 2: Remove the redundant Home proof-card section

**Files:**
- Modify: `cmd/web/pages/home.templ`
- Modify: `cmd/web/tailwind/pages/home.css`
- Modify: `internal/app/home_style_contract_test.go`
- Modify: `internal/app/server_render_smoke_test.go`

**Interfaces:**
- Consumes: the approved Home hero, stats, and `data-region="topology"` section.
- Produces: a Home document ending after the capability topology, with no
  `data-region="proof"` or `home-proof-*` selectors.

- [ ] **Step 1: Write the absence contract**

  Add render assertions that Home contains its hero and topology exactly once
  and does not contain `data-region="proof"`, `home-proof-grid`, “Proof of Work,”
  or “Follow the systems behind the résumé.”

- [ ] **Step 2: Run the Home contract and observe the expected failure**

  Run: `go test ./internal/app -run 'Home'`

  Expected: failure because the proof section still renders.

- [ ] **Step 3: Remove the redundant source**

  Delete `homeExploreCards`, the proof `<section>`, and all `home-proof-*` CSS,
  including breakpoint placement and forced-color selectors. Do not alter hero,
  statistics, or capability markup.

- [ ] **Step 4: Regenerate and verify Home**

  Run: `task generate`

  Run: `go test ./internal/app -run 'Home'`

  Expected: PASS.

- [ ] **Step 5: Commit Home cleanup**

  ```sh
  git add cmd/web/pages/home.templ cmd/web/tailwind/pages/home.css internal/app/home_style_contract_test.go internal/app/server_render_smoke_test.go
  git commit -m "fix(home): remove redundant proof cards"
  ```

### Task 3: Refactor Experience recurring technology into a field-kit panel

**Files:**
- Modify: `cmd/web/partials/experience_kit_stages.templ`
- Modify: `cmd/web/tailwind/pages/experience.css`
- Modify: `internal/app/experience_style_contract_test.go`
- Modify: `cmd/web/partials/experience_viewmodels_test.go`

**Interfaces:**
- Consumes: `experienceOverview.SpotlightTechnologies []string`, preserving its
  frequency-based order and eight-item limit.
- Produces: `.experience-technology-panel` with a single aligned heading block
  and `.experience-technology-chips` wrapping the eight labels.

- [ ] **Step 1: Add markup and ordering tests**

  Require the new panel and chip-list classes, the existing accessible heading,
  and the exact order `PowerShell, AD DS, Windows, AWS, Ansible, Azure, Bash, Go`
  for the current fixture.

- [ ] **Step 2: Run focused Experience tests**

  Run: `go test ./cmd/web/partials ./internal/app -run 'Experience|SpotlightTechnologies'`

  Expected: markup contract failure while the existing data-order test remains
  green.

- [ ] **Step 3: Recompose the section**

  Apply `page-kit-panel-strong` to the route-owned panel, keep “Recurring
  technology” and “Tools that followed the work,” add one sentence explaining
  that the tools recur across multiple roles, and render each technology as a
  compact chip in one wrapping list.

- [ ] **Step 4: Replace the detached grid CSS**

  Remove the two- and four-column cell borders. Use one aligned panel with
  route spacing, a restrained warm gradient, and chips that wrap from one row to
  multiple rows without horizontal overflow. Keep the era section's existing
  spacing rhythm.

- [ ] **Step 5: Regenerate and verify**

  Run: `task generate`

  Run: `go test ./cmd/web/partials ./internal/app -run 'Experience|SpotlightTechnologies'`

  Expected: PASS.

- [ ] **Step 6: Commit Experience polish**

  ```sh
  git add cmd/web/partials/experience_kit_stages.templ cmd/web/tailwind/pages/experience.css cmd/web/partials/experience_viewmodels_test.go internal/app/experience_style_contract_test.go
  git commit -m "fix(experience): align recurring technology panel"
  ```

### Task 4: Add secondary Skills tags without changing primary categories

**Files:**
- Modify: `internal/portfolio/data/skills.json`
- Modify: `types/types.go`
- Modify: `cmd/web/partials/skills_viewmodels.go`
- Modify: `cmd/web/partials/skills_viewmodels_test.go`
- Modify: `internal/portfolio/data_test.go`

**Interfaces:**
- Extends: `types.Skill` with `Tags []string` loaded from `skills.json`.
- Preserves: the existing canonical category and proficiency filter axes.
- Produces: tag-aware category filtering and search while retaining canonical
  unfiltered group ownership.

- [ ] **Step 1: Encode canonical ownership and tag invariants in tests**

  Assert that the existing primary category headings and skill-to-category
  assignments do not change. Require tags to use an existing non-concept
  category name, contain no duplicates, and not repeat the skill's primary
  category. Require GitHub to remain under Development Tools with a
  Collaboration Tools tag.

- [ ] **Step 2: Add failing tag-aware view-model tests**

  Retain Unicode query-length, hostile query encoding, deep-copy, stable skill
  IDs, featured skills, practices, proficiency, URL, and no-result tests. Add
  cases proving that an unfiltered GitHub card stays in Development Tools, a
  Collaboration Tools filter includes it beneath that same heading, and search
  can match a secondary tag without duplicating the skill.

- [ ] **Step 3: Run data and view-model tests and confirm tags are absent**

  Run: `go test ./cmd/web/partials ./internal/portfolio -run 'Skill|Skills'`

  Expected: failure because skills do not yet expose secondary tags and category
  filtering only checks the enclosing category.

- [ ] **Step 4: Add an audited set of secondary tags**

  Add tags only where a genuine cross-discipline relationship helps discovery.
  Do not move or duplicate skill objects and do not change IDs, links,
  descriptions, proficiency, featured flags, icon paths, or primary headings.

- [ ] **Step 5: Make filtering and search tag-aware**

  Keep `SkillFilters`, `CategoryOptions`, `ProficiencyOptions`, and their URL
  behavior. Match a category filter against the primary category or tags, and
  match search against name, description, primary category, and tags. Preserve
  the canonical category on every result and deep-copy nested tag slices.

- [ ] **Step 6: Format and verify**

  Run: `task fmt`

  Run: `go test ./cmd/web/partials ./internal/portfolio -run 'Skill|Skills'`

  Expected: PASS.

- [ ] **Step 7: Commit the data model**

  ```sh
  git add internal/portfolio/data/skills.json types/types.go cmd/web/partials/skills_viewmodels.go cmd/web/partials/skills_viewmodels_test.go internal/portfolio/data_test.go
  git commit -m "feat(skills): add cross-category discovery tags"
  ```

### Task 5: Simplify Skills filter presentation and normalize logos

**Files:**
- Modify: `cmd/web/partials/skills_grid.templ`
- Modify: `cmd/web/tailwind/pages/skills.css`
- Modify: `internal/portfolio/handlers.go`
- Modify: `internal/portfolio/handlers_test.go`
- Modify: `internal/app/skills_style_contract_test.go`
- Modify: `internal/app/server_render_smoke_test.go`
- Modify: `internal/app/task13_final10_product_contract_test.go`
- Modify: `cmd/web/static/js/main.js`

**Interfaces:**
- Consumes: the tag-aware multi-axis Skills props from Task 4.
- Preserves: progressive category, proficiency, and query filtering.
- Produces: one compact wrapping filter instrument and one replaceable catalog
  region; `/skills/filtered` remains the fragment endpoint.

- [ ] **Step 1: Write the quieter filter interaction contract**

  Require one search input, compact wrapping category and proficiency controls,
  one result summary, and one catalog region. Assert absence of horizontal
  filter rails, “Swipe to scan,” filter dots, and fade overlays. Retain normal
  form/link destinations and HTMX requests to `/skills/filtered`.

- [ ] **Step 2: Replace obsolete style tests**

  Replace the filter-rail clearance graph and its mutations with wrapping-control
  tests. Add tests for search focus clearance, readable active-filter states,
  wrapping catalog headings, and a logo plate that uses `var(--candle-oat)` with
  `object-fit: contain` and no brightness filter.

- [ ] **Step 3: Run the Skills interaction tests and confirm expected failures**

  Run: `go test ./internal/portfolio ./internal/app -run 'Skill|Skills'`

  Expected: failure against the current horizontal filter rails, decorative
  guidance, and logo treatment.

- [ ] **Step 4: Simplify Templ without removing filter behavior**

  Retain search, category, and proficiency controls, but place them in one
  wrapping instrument without a sideways scroller. Keep the normal full-page
  GET, fragment endpoint, result count, no-results guidance, clear-filters link,
  stable detail IDs, canonical category headings, and progressive enhancement.
  Render secondary tags in the detail panel rather than every catalog card.

- [ ] **Step 5: Reduce filter JavaScript**

  Delete rail-scroll guidance and decorative fade behavior. Preserve
  category/proficiency focus restoration, multi-axis history state,
  skill-detail focus management, and stale-response protection.

- [ ] **Step 6: Rebuild route CSS**

  Remove dot, horizontal-rail, swipe-hint, and fade styles. Let category and
  proficiency controls wrap naturally beneath the search field with one clear
  active state. Give featured vendor logos a consistent Candle Oat plate and
  optical size; keep Concepts & Practices on the existing dark icon treatment.

- [ ] **Step 7: Regenerate and verify Skills**

  Run: `task generate`

  Run: `node --check cmd/web/static/js/main.js`

  Run: `go test ./internal/portfolio ./internal/app -run 'Skill|Skills'`

  Expected: PASS.

- [ ] **Step 8: Commit Skills presentation**

  ```sh
  git add cmd/web/partials/skills_grid.templ cmd/web/tailwind/pages/skills.css internal/portfolio/handlers.go internal/portfolio/handlers_test.go internal/app/skills_style_contract_test.go internal/app/server_render_smoke_test.go internal/app/task13_final10_product_contract_test.go cmd/web/static/js/main.js
  git commit -m "refactor(skills): simplify toolkit browsing"
  ```

### Task 6: Add real SVG icons to Contact

**Files:**
- Create: `cmd/web/partials/ui_icons.templ`
- Modify: `cmd/web/partials/ui_types.go`
- Modify: `cmd/web/partials/ui_cards.templ`
- Modify: `cmd/web/partials/ui_types_test.go`
- Modify: `cmd/web/pages/contact.templ`
- Modify: `cmd/web/tailwind/pages/contact.css`
- Modify: `internal/app/contact_style_contract_test.go`
- Modify: `internal/app/server_render_smoke_test.go`

**Interfaces:**
- Produces: `UIIconName` constants `mail`, `linkedin`, `github`, `cloud`,
  `automation`, `security`, and `observability`, rendered by
  `UIIcon(name UIIconName)`.
- Extends: `FeatureCardProps` and `LinkPanelRowProps` with optional
  `IconName UIIconName`; existing string `Icon` remains as a compatibility
  fallback for other routes.

- [ ] **Step 1: Add failing icon API and Contact render tests**

  Test all seven closed icon names for one `svg`, `viewBox="0 0 24 24"`,
  `aria-hidden="true"`, and no embedded title. Assert Contact contains no
  field marks with `@`, `IN`, `GH`, `☁`, `↗`, `◇`, or `◎`.

- [ ] **Step 2: Run focused Contact and partial tests**

  Run: `go test ./cmd/web/partials ./internal/app -run 'Icon|Contact'`

  Expected: failure because the typed icon API does not exist.

- [ ] **Step 3: Implement the typed inline icon set**

  Render local SVG paths for mail, the official recognizable LinkedIn and
  GitHub silhouettes, cloud architecture, automation arrows/gears, a shield,
  and an observability pulse. All semantic icons use `fill="none"`,
  `stroke="currentColor"`, `stroke-width="1.8"`; brand icons use
  `fill="currentColor"`.

- [ ] **Step 4: Add optional component rendering to shared cards**

  In `FeatureCard` and `LinkPanelRow`, render `UIIcon(props.IconName)` when the
  typed value is non-empty and otherwise render the existing escaped text icon.
  Keep surrounding accessible labels unchanged.

- [ ] **Step 5: Switch Contact and tune icon framing**

  Map the three channels and four expertise cards to the typed icons. Size SVGs
  consistently, let existing rose/mint/apricot tones supply `currentColor`, and
  avoid adding new card decoration.

- [ ] **Step 6: Regenerate and verify Contact**

  Run: `task generate`

  Run: `task fmt`

  Run: `go test ./cmd/web/partials ./internal/app -run 'Icon|Contact'`

  Expected: PASS.

- [ ] **Step 7: Commit Contact icons**

  ```sh
  git add cmd/web/partials/ui_icons.templ cmd/web/partials/ui_types.go cmd/web/partials/ui_cards.templ cmd/web/partials/ui_types_test.go cmd/web/pages/contact.templ cmd/web/tailwind/pages/contact.css internal/app/contact_style_contract_test.go internal/app/server_render_smoke_test.go
  git commit -m "feat(contact): add meaningful svg icons"
  ```

### Task 7: Run portfolio-wide regression and visual QA

**Files:**
- Create: `docs/superpowers/qa/2026-08-18-portfolio-feedback-polish.md`
- Modify only if a verified defect is found: source files owned by Tasks 1–6.

**Interfaces:**
- Consumes: the final built server and the repository's existing route/state
  fixtures.
- Produces: reproducible checks and final screenshots for the accepted source.

- [ ] **Step 1: Run repository-authoritative source checks**

  Run: `task generate`

  Run: `task fmt`

  Run: `task lint`

  Run: `task test`

  Run: `task build`

  Run: `node --check cmd/web/static/js/main.js`

  Run: `git diff --check`

  Expected: all pass. If `task lint` repeats the known `no go files to analyze`
  toolchain failure, record it verbatim and do not describe lint as passing.

- [ ] **Step 2: Review affected routes at required widths**

  Capture `/`, `/about`, `/experience`, `/skills`, and `/contact` at 390, 768,
  1119, 1121, and 1440 pixels after the final build. Check alignment, overlap,
  clipping, cutoffs, keyboard focus, and page-level overflow.

- [ ] **Step 3: Run unchanged-route regression review**

  Capture `/projects`, `/education`, `/soccer`, `/mgmt`, and the portal error
  fixture at 390 and 1440 pixels. Confirm that shared trail and icon changes do
  not alter their content or behavior.

- [ ] **Step 4: Verify interaction and accessibility states**

  Exercise Skills default, search match, no result, clear search, rapid search,
  detail open/close, back/forward, no-JavaScript GET, reduced motion, forced
  colors, and keyboard focus.

- [ ] **Step 5: Write the QA report and commit evidence pointers**

  Record exact commands, results, viewport paths, and any blocked boundary in
  the QA document.

  ```sh
  git add docs/superpowers/qa/2026-08-18-portfolio-feedback-polish.md
  git commit -m "test(portfolio): verify feedback polish"
  ```
