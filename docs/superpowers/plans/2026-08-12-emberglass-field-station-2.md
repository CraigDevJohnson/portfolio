# Emberglass Field Station 2.0 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver a cohesive, warm, vibrant dark-pastel portfolio with
page-appropriate layouts, stronger Go/Templ/HTMX boundaries, and verified
responsive behavior across all public pages, Soccer states, and the management
portal.

**Architecture:** Migrate incrementally from the current cascade sandwich to a
single-owner CSS architecture. Preserve server-rendered routes and HTMX
contracts while moving presentation decisions into typed Go view models,
focused Templ components, shared component CSS, and route-scoped composition
files. Keep every migration independently buildable and browser-reviewable.

**Tech Stack:** Go 1.26, Templ, HTMX 1.9, Tailwind CSS 4 standalone CLI,
semantic CSS layers, vanilla JavaScript, Go `net/http`, and the in-app browser
for responsive visual QA.

## Global Constraints

- Treat the current working tree as the source of truth; it contains substantial
  user-owned work that must not be reset, overwritten, or swept into unrelated
  commits.
- Preserve existing copy and functional behavior except for concise usability
  clarification in controls, empty states, loading states, and errors.
- Keep the six core colors exact: Night Mulberry `#17121B`, Cocoa Cedar
  `#2E2130`, Candle Oat `#FFF0D8`, Campfire Apricot `#FFA677`, Rosehip
  `#FF7FA8`, and Pond Mint `#78E3C3`.
- Keep Bricolage Grotesque for display, Atkinson Hyperlegible for body and UI,
  and IBM Plex Mono for captions, dates, statistics, statuses, and data.
- Reset Tailwind's default breakpoint namespace, then map `sm`, `md`, `lg`, and
  `xl` to `30rem`, `48rem`, `70rem`, and `80rem`. Remove `xs`, `2xl`, and
  arbitrary width variants. Use container queries when a component must
  respond to its own width.
- Render exactly one typed `SignalTrail` with `data-signal-trail` on every full
  page, including the portal error page. Fragments render no trail. Keep it
  static under reduced motion and a structural rule under forced colors.
- Keep minimum pointer targets at 44 by 44 CSS pixels.
- Preserve primary navigation, project/contact links, Skills filter URLs,
  Soccer native downloads, and core portal navigation without HTMX or shared
  JavaScript.
- Preserve Soccer IDs, OOB swaps, data attributes, native download behavior,
  and event contracts unless handlers, JavaScript, templates, and tests are
  updated atomically.
- Preserve the portal `X-Portal-Fragment-Error` response contract.
- Edit `.templ` source only; never edit generated `*_templ.go` files.
- Edit Tailwind sources only; never edit generated
  `cmd/web/static/css/tailwind.css`.
- Run `task generate` after `.templ` edits and the repository-authoritative
  checks before completion.
- Before each repository-wide `task fmt`, capture `git status --short` and
  `git diff --name-only`; inspect the same outputs afterward and identify any
  formatter changes outside the task before staging.
- Because shared source files were already modified before this plan, do not
  create a source commit that silently includes pre-existing work. At each
  checkpoint, stage only newly created files and separable new hunks. A
  checkpoint commit is allowed only when every source, generated-template,
  test, and CSS dependency for that checkpoint is staged as one buildable set.
  Otherwise skip the commit and report the tested working-tree state.

---

## File Structure

### Shared presentation

- Create `cmd/web/partials/ui_types.go`: closed visual enums, typed component
  props, class mappings, and attribute-safe helpers.
- Create `cmd/web/partials/ui_hero.templ`: `PageHero`, `PageHeroIntro`,
  `SectionLabel`, and `SectionIntro`.
- Create `cmd/web/partials/ui_actions.templ`: `ActionLink`, `ActionButton`, and
  `PageCTA`.
- Create `cmd/web/partials/ui_stats.templ`: `StatCard` and `StatCardGrid`.
- Create `cmd/web/partials/ui_cards.templ`: feature, link-panel, info-row, and
  credential components.
- Create `cmd/web/partials/ui_feedback.templ`: shared feedback message and
  overflow-region components.
- Create `cmd/web/partials/ui_signal.templ`: the single typed route signal
  trail and its accessible structural marker.
- Delete `cmd/web/partials/ui.templ` only after every exported component has
  moved and all callers compile.
- Create `cmd/web/partials/ui_types_test.go`: enum/class and component rendering
  tests.
- Create `cmd/web/partials/navigation_test.go`: header/footer grouping and active
  navigation tests.

### Tailwind and CSS

- Modify `cmd/web/tailwind/theme.css`: canonical colors, semantic aliases,
  typography, containers, and four composition thresholds.
- Modify `cmd/web/tailwind/shared.css`: spacing, radii, type scale, timing, and
  motion primitives.
- Modify `cmd/web/tailwind/base.css`: base elements, skip link, focus, reduced
  motion, and forced colors.
- Rewrite `cmd/web/tailwind/components.css`: shared shell and component internals
  only.
- Create `cmd/web/tailwind/pages/home.css`.
- Create `cmd/web/tailwind/pages/about.css`.
- Create `cmd/web/tailwind/pages/experience.css`.
- Create `cmd/web/tailwind/pages/skills.css`.
- Create `cmd/web/tailwind/pages/projects.css`.
- Create `cmd/web/tailwind/pages/education.css`.
- Create `cmd/web/tailwind/pages/contact.css`.
- Rewrite `cmd/web/tailwind/soccer.css`: Matchday Planner workflow and results.
- Create `cmd/web/tailwind/portal.css`: operator workspace and responsive
  instance cards.
- Modify `cmd/web/tailwind/app.css`: explicit `theme`, `base`, `components`,
  `pages`, and `utilities` order.
- Delete `cmd/web/tailwind/emberglass.css`,
  `cmd/web/tailwind/emberglass-responsive.css`, and
  `cmd/web/tailwind/emberglass-accessibility.css` after their surviving rules
  have one canonical owner.
- Create `internal/app/style_architecture_test.go`: source-level CSS ownership,
  import, breakpoint, reduced-motion, and removed-layer assertions.

### Route composition and view models

- Modify all files in `cmd/web/pages/*.templ` named by the route tasks below.
- Modify `cmd/web/partials/experience_kit_stages.templ` and
  `cmd/web/partials/experience_viewmodels.go` for responsive era composition.
- Create `cmd/web/partials/projects_viewmodels.go` and
  `cmd/web/partials/projects_viewmodels_test.go` for explicit project
  presentation metadata.
- Create `cmd/web/pages/education_viewmodels.go` and
  `cmd/web/pages/education_viewmodels_test.go` for explicit credential domains.
- Create `cmd/web/partials/skills_viewmodels.go` and
  `cmd/web/partials/skills_viewmodels_test.go` for typed filter options and
  filtered catalog state.
- Create `cmd/web/partials/soccer_viewmodels.go` and
  `cmd/web/partials/soccer_viewmodels_test.go` for capability predicates and
  shared security copy.
- Create `cmd/web/partials/portal_viewmodels.go` and
  `cmd/web/partials/portal_viewmodels_test.go` for normalized state labels and
  responsive cell metadata.
- Modify `internal/portfolio/handlers.go` and add
  `internal/portfolio/handlers_test.go` for progressive Skills URLs.
- Modify `internal/portfolio/data/projects.json` and `types/types.go` to encode
  project presentation explicitly.
- Modify `internal/portal/preview.go` and its tests to expose every visual
  instance state without AWS side effects.
- Create `internal/app/ui_preview.go` and
  `internal/app/ui_preview_test.go`: loopback-only Soccer and Portal visual
  fixtures registered only while the existing safe local preview mode is on.

### Interaction and verification

- Modify `cmd/web/static/js/main.js`: canonical navigation breakpoint, remote
  image fallback, HTMX focus/state handling, and reduced-motion-safe behavior.
- Extend `internal/app/server_render_smoke_test.go`: semantic page markers,
  heading count, duplicate IDs, full-page/fragment boundaries, and progressive
  Skills URLs.
- Create `docs/superpowers/qa/2026-08-12-emberglass-field-station-2.md`: visual
  evidence matrix and iteration notes.

---

### Task 1: Establish Typed Shared UI Primitives

**Files:**

- Create: `cmd/web/partials/ui_types.go`
- Create: `cmd/web/partials/ui_hero.templ`
- Create: `cmd/web/partials/ui_actions.templ`
- Create: `cmd/web/partials/ui_stats.templ`
- Create: `cmd/web/partials/ui_cards.templ`
- Create: `cmd/web/partials/ui_feedback.templ`
- Create: `cmd/web/partials/ui_signal.templ`
- Create: `cmd/web/partials/ui_types_test.go`
- Modify: `cmd/web/tailwind/components.css`
- Modify: `cmd/web/tailwind/emberglass.css`
- Modify: `cmd/web/tailwind/emberglass-responsive.css`
- Modify: every `.templ` caller currently importing components from
  `cmd/web/partials/ui.templ`
- Delete: `cmd/web/partials/ui.templ`

**Interfaces:**

- Produces: `Tone`, `HeroVariant`, `ActionVariant`, `FeedbackKind`,
  `StatGridVariant`, `SignalTrailVariant`, and their closed constant sets.
- Produces: `PageHeroProps`, `PageHeroIntroProps`, `SectionIntroProps`,
  `ActionLinkProps`, `ActionButtonProps`, `PageCTAProps`, `FeedbackProps`,
  `OverflowRegionProps`, `StatCardProps`, and `StatCardGridProps`.
- Produces: `SignalTrailProps` and one `SignalTrail` component whose root has
  `data-signal-trail` and a typed route-shape class.
- Preserves exported component names `PageHero`, `PageHeroIntro`, `SectionIntro`,
  `FeatureCard`, `LinkPanelCard`, `LinkPanelRow`, `StatCard`, `StatCardGrid`, and
  `CredentialCard` so route migrations remain incremental.

- [ ] **Step 1: Write failing tests for the closed visual vocabulary**

Add table tests that require every enum to resolve to one stable class and
unknown values to resolve to the quiet default:

```go
func TestHeroVariantClass(t *testing.T) {
    tests := []struct {
        variant HeroVariant
        want    string
    }{
        {HeroIdentity, "page-hero-identity"},
        {HeroNarrative, "page-hero-narrative"},
        {HeroTimeline, "page-hero-timeline"},
        {HeroCatalog, "page-hero-catalog"},
        {HeroCaseStudy, "page-hero-case-study"},
        {HeroInvitation, "page-hero-invitation"},
        {HeroTool, "page-hero-tool"},
        {HeroVariant("unknown"), "page-hero-standard"},
    }
    for _, test := range tests {
        if got := heroVariantClass(test.variant); got != test.want {
            t.Errorf("heroVariantClass(%q) = %q, want %q", test.variant, got, test.want)
        }
    }
}
```

Add render tests requiring external `ActionLink` output to include
`target="_blank"`, `rel="noopener noreferrer"`, and accessible new-tab text;
disabled/loading `ActionButton` output to expose native disabled and busy
semantics; `Feedback` error output to use `role="alert"`; and `OverflowRegion`
to connect its visible hint through `aria-describedby`.

- [ ] **Step 2: Run the focused test and confirm the new API is absent**

Run:

```sh
go test ./cmd/web/partials -run TestHeroVariantClass -count=1
```

Expected: compilation fails because `HeroVariant` and `heroVariantClass` do not
exist.

- [ ] **Step 3: Define the exact typed interfaces**

Create `ui_types.go` with these core declarations:

```go
type Tone string

const (
    ToneApricot Tone = "apricot"
    ToneRose    Tone = "rose"
    ToneMint    Tone = "mint"
)

type HeroVariant string

const (
    HeroIdentity   HeroVariant = "identity"
    HeroNarrative  HeroVariant = "narrative"
    HeroTimeline   HeroVariant = "timeline"
    HeroCatalog    HeroVariant = "catalog"
    HeroCaseStudy  HeroVariant = "case-study"
    HeroInvitation HeroVariant = "invitation"
    HeroTool       HeroVariant = "tool"
)

type ActionVariant string

const (
    ActionPrimary   ActionVariant = "primary"
    ActionSecondary ActionVariant = "secondary"
    ActionQuiet     ActionVariant = "quiet"
    ActionDanger    ActionVariant = "danger"
)

type FeedbackKind string

const (
    FeedbackInfo    FeedbackKind = "info"
    FeedbackSuccess FeedbackKind = "success"
    FeedbackWarning FeedbackKind = "warning"
    FeedbackError   FeedbackKind = "error"
)

type StatGridVariant string

const (
    StatGridHero    StatGridVariant = "hero"
    StatGridCompact StatGridVariant = "compact"
    StatGridSummary StatGridVariant = "summary"
)

type SignalTrailVariant string

const (
    TrailTopology       SignalTrailVariant = "topology"
    TrailSwitchback     SignalTrailVariant = "switchback"
    TrailTimeline       SignalTrailVariant = "timeline"
    TrailWorkbench      SignalTrailVariant = "workbench"
    TrailDossier        SignalTrailVariant = "dossier"
    TrailFieldGuide     SignalTrailVariant = "field-guide"
    TrailCorrespondence SignalTrailVariant = "correspondence"
    TrailMatchday       SignalTrailVariant = "matchday"
    TrailOperator       SignalTrailVariant = "operator"
    TrailInterruption   SignalTrailVariant = "interruption"
)
```

Use `templ.Attributes` on `ActionButtonProps` so HTMX attributes can pass
through without encoding HTMX-specific fields in the visual component:

```go
type ActionLinkProps struct {
    Href       string
    Label      string
    Variant    ActionVariant
    External   bool
    ExtraClass string
    Attributes templ.Attributes
}

type ActionButtonProps struct {
    Type       string
    Label      string
    Variant    ActionVariant
    Disabled   bool
    Loading    bool
    ExtraClass string
    Attributes templ.Attributes
}
```

- [ ] **Step 4: Split the shared components without changing rendered behavior**

Move hero, action, statistic, card, and feedback markup into the focused Templ
files. Replace positional `PageCTA` arguments with:

```go
type PageCTAProps struct {
    Title     string
    Subtitle  string
    Primary   ActionLinkProps
    Secondary *ActionLinkProps
}
```

Implement `OverflowRegion` with a visible hint and one focusable local scroll
container:

```templ
templ OverflowRegion(props OverflowRegionProps) {
    <div class="overflow-region-shell">
        <p id={ props.ID + "-hint" } class="overflow-region-hint">{ props.Hint }</p>
        <div
            id={ props.ID }
            class="overflow-region"
            role="region"
            aria-label={ props.Label }
            aria-describedby={ props.ID + "-hint" }
            tabindex="0"
        >
            { children... }
        </div>
    </div>
}
```

Implement `SignalTrail` as one noninteractive, `aria-hidden="true"` structural
element. Page CSS may place and reshape that element, but may not create a
second trail with a pseudo-element or duplicate route decoration.

- [ ] **Step 5: Add reusable trail-count test helpers**

Test that `SignalTrail` renders exactly one `data-signal-trail` marker and add
helpers `requireOneFullPageTrail` and `requireNoFragmentTrail`. Require zero
markers from current HTMX-only Skills, Soccer, and Portal fragments. Each route
task adds its full-page component to the one-trail table only when it places the
trail in that route's approved structural location; no test is skipped or left
failing between tasks.

- [ ] **Step 6: Migrate every caller to typed constants**

Replace string values such as `Variant: "tool"` and `Tone: "primary"` with
closed constants such as `Variant: partials.HeroTool` and
`Tone: partials.ToneMint`. Replace every positional `PageCTA` call with a
`PageCTAProps` literal. Remove the old `ui.templ` only after `rg` finds no old
property or positional API use.

Run:

```sh
rg -n 'Variant:\s*"|Tone:\s*"|PageCTA\(' cmd/web -g '*.templ'
```

Expected: no stringly typed hero/tone assignments and only typed `PageCTA`
calls.

- [ ] **Step 7: Generate, format, and run focused tests**

Run:

```sh
task generate
task fmt
go test ./cmd/web/partials ./internal/app -count=1
git diff --check
```

Expected: all commands pass; full pages and fragments still render.

- [ ] **Step 8: Remove superseded shared-component CSS and checkpoint**

For each migrated hero, action, statistic, card, feedback, overflow, and signal
family, move its surviving styles into `components.css`, compare representative
pages, and remove the corresponding older declarations from `components.css`
and `emberglass.css` in the same step. Do not leave both component versions for
Task 12 to reconcile.

Run `task tailwind-build`, the focused partial/app tests, and
`git diff --check` again after this CSS consolidation, then repeat the
representative page comparison against the rebuilt stylesheet.

Inspect `git diff --stat` and `git diff -- cmd/web/partials`. Stage only new
files and separable new hunks. If a clean commit is possible, use:

```sh
git commit -m "refactor: type shared portfolio components"
```

Otherwise leave the source changes unstaged and record the completed test output
in the task handoff.

---

### Task 2: Consolidate Foundation, Shell, and Navigation

**Files:**

- Create: `cmd/web/partials/navigation_test.go`
- Modify: `cmd/web/layouts/base.templ`
- Modify: `cmd/web/partials/header.templ`
- Modify: `cmd/web/partials/nav.templ`
- Modify: `cmd/web/partials/footer.templ`
- Modify: `cmd/web/static/js/main.js`
- Modify: `cmd/web/tailwind/theme.css`
- Modify: `cmd/web/tailwind/shared.css`
- Modify: `cmd/web/tailwind/base.css`
- Modify: `cmd/web/tailwind/components.css`
- Modify: `cmd/web/tailwind/app.css`
- Modify: `cmd/web/tailwind/emberglass.css`
- Modify: `cmd/web/tailwind/emberglass-responsive.css`
- Modify: every `.templ` and Tailwind source returned by
  `rg -l '(xs:|2xl:|min-\[|max-\[)' cmd/web`
- Modify: `internal/app/server_render_smoke_test.go`

**Interfaces:**

- Produces: `layouts.ShellVariant`, `layouts.ShellPublic`, and
  `layouts.ShellOperator`.
- Produces: explicit `navItem.FooterGroup` values and deterministic
  `footerNavGroups()` output.
- Produces: canonical desktop-menu boundary at `70rem` shared by CSS and
  `window.matchMedia`.
- Produces: an import-owned layer strategy: imported files contain no outer
  `@layer` wrapper; `app.css` assigns every source to its named layer.
- Produces: Tailwind `sm=30rem`, `md=48rem`, `lg=70rem`, and `xl=80rem`, with
  all default and arbitrary width thresholds removed.
- Consumes: typed shared primitives from Task 1.

- [ ] **Step 1: Write failing navigation and accessibility tests**

Require explicit footer grouping and exactly one active item:

```go
func TestFooterNavigationGroupsAreExplicit(t *testing.T) {
    groups := footerNavGroups()
    if len(groups) != 2 {
        t.Fatalf("footerNavGroups() returned %d groups, want 2", len(groups))
    }
    if groups[0].Label != "Portfolio" || groups[1].Label != "Tools" {
        t.Fatalf(
            "footer groups = %q, %q; want Portfolio, Tools",
            groups[0].Label,
            groups[1].Label,
        )
    }
}
```

Extend the render smoke test to count one `<h1` per full page and require the
focused skip-link class marker `site-skip-link`.

- [ ] **Step 2: Run the tests and confirm explicit grouping is absent**

Run:

```sh
go test ./cmd/web/partials ./internal/app \
  -run 'TestFooterNavigationGroups|TestBuildMuxPublicRoute' \
  -count=1
```

Expected: the new footer-group test fails because grouping is currently
positional.

- [ ] **Step 3: Introduce public and operator shell variants**

Add:

```go
type ShellVariant string

const (
    ShellPublic   ShellVariant = "public"
    ShellOperator ShellVariant = "operator"
)

type BaseProps struct {
    Title       string
    Page        string
    Description string
    Shell       ShellVariant
}
```

Default an empty shell to `ShellPublic`. Add `data-shell` to `<body>`, use the
operator shell for wider portal content, and retain one shared header. Public
pages use the complete grouped footer. `ShellOperator` uses a compact footer
containing the Craig Johnson identity link, one “Back to portfolio” link, and
the copyright line; it never renders the public destination columns.

- [ ] **Step 4: Make navigation and footer data explicit**

Extend the navigation model:

```go
type navItem struct {
    Href        string
    Label       string
    Page        string
    FooterGroup string
}
```

Assign Home through Contact to `Portfolio` and Soccer to `Tools`. Render desktop
navigation from the complete list, mobile navigation from the complete list,
and footer columns from the explicit group values.

- [ ] **Step 5: Move responsive visibility out of template utilities**

Remove `max-[1120px]` classes from navigation markup. Set desktop/mobile
visibility in `components.css` at `69.999rem`, and update JavaScript to:

```js
const mobileNavBreakpoint = window.matchMedia('(max-width: 69.999rem)')
```

Keep the current inert background, focus loop, Escape restoration, and resize
closure behavior.

- [ ] **Step 6: Fix the skip link and reduced-motion foundation**

Render the skip link with one named class, not `focus:not-sr-only`. In base CSS,
ensure focused positioning is fixed above the header:

```css
.site-skip-link:focus-visible {
  position: fixed;
  inset: var(--space-sm) auto auto var(--space-sm);
  z-index: 300;
  width: auto;
  height: auto;
  clip: auto;
  overflow: visible;
}

@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after {
    animation-duration: 0.01ms !important;
    animation-delay: 0s !important;
    transition-duration: 0.01ms !important;
    scroll-behavior: auto !important;
  }
}
```

- [ ] **Step 7: Establish import-owned layers without nested wrappers**

Declare `@layer theme, base, components, pages, utilities` in `app.css`. Keep
all `@source` declarations. Assign imports with `layer(...)` in `app.css`, then
remove the outer `@layer base` and `@layer components` wrappers from imported
project files so the result does not contain `base.base` or
`components.components` nested layers.

During migration, keep `emberglass.css` and `emberglass-responsive.css` in the
`components` layer after `components.css`; later route modules use the `pages`
layer and therefore win without a specificity race. Keep
`emberglass-accessibility.css` temporarily unlayered after utilities until its
rules have been replaced by the authoritative forced-colors contract in Task
12.

Reduce `shared.css` to root tokens and reusable keyframes before importing it
into `theme`. Move its shell, skip-link, hamburger, reduced-motion, and utility
class rules into `base.css` or `components.css` according to ownership.

- [ ] **Step 8: Normalize foundation tokens and Tailwind breakpoints**

Keep the six exact palette values and add documented composition variables for
`30rem`, `48rem`, `70rem`, and `80rem`. Define this exact Tailwind namespace:

```css
--breakpoint-*: initial;
--breakpoint-sm: 30rem;
--breakpoint-md: 48rem;
--breakpoint-lg: 70rem;
--breakpoint-xl: 80rem;
```

Migrate every source `xs:` to `sm:`, `2xl:` to `xl:`, and every
`min-[...]`/`max-[...]` width variant to a canonical variant or route CSS.
Remove noncanonical raw width queries from the shared foundation and shell
families migrated in this task. Inventory remaining route-owned queries in the
migration files; each route task deletes its own entries, and Task 12 enforces
that none survive. Preserve non-width feature and container queries.

Remove aliases only after `rg` proves no consumer remains. Preserve semantic
success, warning, danger, copy, border, focus, and surface tokens. Replace the
literal `--pollen-gold: #FFD166` foundation with a semantic mix of Candle Oat
and Campfire Apricot; neither `--pollen-gold` nor `#FFD166` may remain.

- [ ] **Step 9: Give forced colors explicit cascade authority**

Add generic forced-color rules in `base.css` for surfaces, controls, focus,
feedback, tables, modal, overflow regions, and `data-signal-trail`. Because
`base` precedes later layers, use narrowly targeted system-color declarations
with `!important` only inside `@media (forced-colors: active)` and apply
`forced-color-adjust` only where native system rendering would erase necessary
state. Task 12 removes the temporary unlayered accessibility file only after
computed styles prove these rules win.

- [ ] **Step 10: Generate and verify shell behavior**

Run:

```sh
task generate
task fmt
go test ./cmd/web/partials ./internal/app -count=1
task tailwind-build
node --check cmd/web/static/js/main.js
git diff --check
```

Browser-check desktop navigation at 1121px, the mobile menu at 1119px, the
focused skip link, open-menu focus wrapping, Escape restoration, and header
state after 100px of scroll. Sample every route at 390 and 1440 pixels for gross
overflow after the breakpoint remap. Compile Tailwind and prove utility-emitted
navigation thresholds are canonical; record remaining raw migration-file
thresholds for their owning route tasks rather than hiding them with overrides.

- [ ] **Step 11: Remove superseded shell CSS and checkpoint**

After browser comparison, remove the migrated shell, header, navigation,
footer, skip-link, and reduced-motion declarations from both Emberglass
migration files. Rebuild Tailwind and repeat the navigation/skip-link checks
after deletion. Do not defer those component families to Task 12.

If the new navigation test and new files can be staged without existing work,
commit them with:

```sh
git commit -m "refactor: consolidate portfolio shell"
```

Leave inseparable pre-existing source hunks unstaged and document them.

---

### Task 3: Build the Home Systems Overlook

**Files:**

- Create: `cmd/web/tailwind/pages/home.css`
- Modify: `cmd/web/tailwind/app.css`
- Modify: `cmd/web/pages/home.templ`
- Modify: `cmd/web/static/js/main.js`
- Modify: `cmd/web/tailwind/components.css`
- Modify: `cmd/web/tailwind/emberglass.css`
- Modify: `cmd/web/tailwind/emberglass-responsive.css`
- Modify: `internal/app/server_render_smoke_test.go`

**Interfaces:**

- Produces: `data-layout="systems-overlook"` and ordered region markers for
  `photo`, `intro`, `topology`, and `proof`.
- Produces: `setupRemoteImageFallbacks(root = document)` and
  `data-remote-image` / `data-image-fallback` markup.
- Consumes: `SignalTrail(TrailTopology)` exactly once inside the topology.

- [ ] **Step 1: Add failing route-structure assertions**

Require one Home `h1`, one trail, the layout marker, all four region markers,
and DOM order `photo < intro < topology < proof`. The photo-first DOM order is
the compact-screen reading order; CSS moves it beside the intro on wide screens.

- [ ] **Step 2: Run the smoke test and confirm the markers are absent**

```sh
go test ./internal/app -run TestBuildMuxPublicRouteRenderingSmoke -count=1
```

- [ ] **Step 3: Recompose Home around a systems topology**

Keep all content. Give the hero a photographic build environment with the
portrait as a smaller witness, the identity thesis, one primary action, and
compact credibility proof. Connect Cloud Architecture to Infrastructure as
Code, Delivery Systems, and Operational Readiness in the topology. Follow it
with one wide Experience dossier, two medium Skills/Projects dossiers, and one
compact Soccer tool brief rather than four equal cards.

- [ ] **Step 4: Add a resilient portrait treatment**

Render `CJ` behind the Gravatar. On image load add `is-loaded`; on error add
`is-failed`; initialize at startup and after HTMX swaps. Give both image and
fallback fixed dimensions so failure cannot shift layout. Essential identity
text never depends on the remote image.

- [ ] **Step 5: Import and implement the route-owned composition**

Add `pages/home.css` to `app.css` in the `pages` layer in this task. Compact
screens render the photo portal before copy. At `80rem`, the portal occupies
45–55 percent of the hero and sits beside the copy. Define grid placement anew
at every column change and keep focusable portrait/link outlines outside any
clipped image inner wrapper.

- [ ] **Step 6: Place the single signal trail and remove old Home rules**

Place `TrailTopology` through the capability map so it communicates
cloud-to-delivery-to-operations flow. Remove Home declarations from the legacy
component and Emberglass files after side-by-side browser comparison; do not
leave Task 12 two competing Home systems.

- [ ] **Step 7: Generate, test, and review all widths**

```sh
task generate
task fmt
task tailwind-build
node --check cmd/web/static/js/main.js
go test ./internal/app -run TestBuildMuxPublicRouteRenderingSmoke -count=1
git diff --check
```

Review 390, 768, 1119, 1121, and 1440 pixels. Force the Gravatar to a failing
URL once, verify the `CJ` fallback, then reload. Confirm one trail, no implicit
grid track, no clipped focus, and no page overflow.

- [ ] **Step 8: Record the safe checkpoint**

If the staged set is independently buildable, use
`git commit -m "feat: build home systems overlook"`; otherwise skip the commit.

---

### Task 4: Build the About Alaska Switchback

**Files:**

- Create: `cmd/web/tailwind/pages/about.css`
- Modify: `cmd/web/tailwind/app.css`
- Modify: `cmd/web/pages/about.templ`
- Modify: `cmd/web/tailwind/components.css`
- Modify: `cmd/web/tailwind/emberglass.css`
- Modify: `cmd/web/tailwind/emberglass-responsive.css`
- Modify: `internal/app/server_render_smoke_test.go`

**Interfaces:**

- Produces: `data-layout="alaska-switchback"` and regions `story`, `facts`,
  `timeline`, `hobbies`, and `values`.
- Consumes: `SignalTrail(TrailSwitchback)` exactly once along the story path.

- [ ] **Step 1: Add failing About structure and trail tests**

Require the layout marker, all five region markers, a single `h1`, and exactly
one trail. Require story before timeline in the DOM and facts adjacent to the
story rather than detached after all narrative content.

- [ ] **Step 2: Run the focused smoke test and confirm failure**

```sh
go test ./internal/app -run TestBuildMuxPublicRouteRenderingSmoke -count=1
```

- [ ] **Step 3: Recompose the narrative as a switchback**

Keep the hero, facts, story, timeline, hobbies, and values. Let the story and
timeline share a readable path, while facts are a compact field pack, hobbies
are an open strip, and values are a concluding set of principles. Do not render
all four as the same panel/card component.

- [ ] **Step 4: Import route CSS and define exact responsive behavior**

Add `pages/about.css` to `app.css`. The facts pack is normal-flow content below
`70rem` and sticky beside the story at `70rem` and above. Reset grid placement,
margins, transforms, and sticky offsets at the boundary. Keep body copy between
45 and 72 characters per line.

- [ ] **Step 5: Place the trail and remove old About ownership**

Run `TrailSwitchback` once through the story/timeline transition. Under forced
colors it is a solid structural rule; it never carries the only meaning of the
timeline. Remove migrated About rules from the legacy CSS files.

- [ ] **Step 6: Generate, test, and inspect**

```sh
task generate
task fmt
task tailwind-build
go test ./internal/app -run TestBuildMuxPublicRouteRenderingSmoke -count=1
git diff --check
```

Review every required width plus long paragraphs, facts-pack sticky entry/exit,
heading order, trail placement, focus, and page overflow.

- [ ] **Step 7: Record the safe checkpoint**

If independently buildable, use
`git commit -m "feat: build about Alaska switchback"`; otherwise skip it.

---

### Task 5: Build the Experience Three-Era Sequence

**Files:**

- Create: `cmd/web/tailwind/pages/experience.css`
- Modify: `cmd/web/tailwind/app.css`
- Modify: `cmd/web/pages/experience.templ`
- Modify: `cmd/web/partials/experience_kit_stages.templ`
- Modify: `cmd/web/partials/experience_viewmodels.go`
- Modify: `cmd/web/partials/experience_viewmodels_test.go`
- Modify: `cmd/web/tailwind/components.css`
- Modify: `cmd/web/tailwind/emberglass.css`
- Modify: `cmd/web/tailwind/emberglass-responsive.css`
- Modify: `internal/app/server_render_smoke_test.go`

**Interfaces:**

- Produces: `data-layout="career-eras"` and three ordered era markers.
- Consumes: existing `buildExperienceOverview` and `buildCareerStages` data,
  `StatGridSummary`, and one `SignalTrail(TrailTimeline)`.

- [ ] **Step 1: Write failing era, stat, and route tests**

Require exactly three nonempty career stages in chronological display order,
one route marker, one trail, and a summary-stat variant marker. Assert the
rendered role elements do not carry equal-height utility/classes.

- [ ] **Step 2: Run focused tests and confirm the new contract fails**

```sh
go test ./cmd/web/partials ./internal/app \
  -run 'TestBuildCareerStages|TestBuildMuxPublicRoute' -count=1
```

- [ ] **Step 3: Recompose Experience as a true sequence**

Keep every role and responsibility. Use a horizontal era rail only at `80rem`
and a single vertical rail below it. Keep dates/status next to role headings.
Use `StatGridSummary`: one column below `30rem`, two from `30rem`, and three
from `48rem`. Role cards size to their content and are never stretched to equal
heights.

- [ ] **Step 4: Import route CSS and reset alternating placement**

Add `pages/experience.css` to `app.css`. Base layout is linear DOM order. At
each wider composition, explicitly set every chapter's grid column/row; below
that threshold clear margins and transforms. Do not use the old `Side` field to
control narrow-screen reading order.

- [ ] **Step 5: Make the era rail the only trail and remove old rules**

Implement the era rail through the typed trail component instead of layering a
second decorative line behind it. Remove migrated Experience declarations from
legacy CSS after comparison.

- [ ] **Step 6: Generate, test, and inspect**

```sh
task generate
task fmt
task tailwind-build
go test ./cmd/web/partials ./internal/app -count=1
git diff --check
```

Review all widths, long company names, dates, technology chips, natural card
heights, rail continuity, anchor offsets, focus, and overflow.

- [ ] **Step 7: Record the safe checkpoint**

If independently buildable, use
`git commit -m "feat: build experience career eras"`; otherwise skip it.

---

### Task 6: Build Explicit Project Dossiers

**Files:**

- Create: `cmd/web/tailwind/pages/projects.css`
- Create: `cmd/web/partials/projects_viewmodels.go`
- Create: `cmd/web/partials/projects_viewmodels_test.go`
- Modify: `cmd/web/tailwind/app.css`
- Modify: `cmd/web/pages/projects.templ`
- Modify: `cmd/web/partials/projects_grid.templ`
- Modify: `internal/portfolio/data/projects.json`
- Modify: `internal/portfolio/data_test.go`
- Modify: `types/types.go`
- Modify: `cmd/web/tailwind/components.css`
- Modify: `cmd/web/tailwind/emberglass.css`
- Modify: `cmd/web/tailwind/emberglass-responsive.css`
- Modify: `internal/app/server_render_smoke_test.go`

**Interfaces:**

- Produces: `data-layout="project-dossiers"`.
- Produces: explicit `Featured`, `ImageRatio`, `Problem`, `Approach`, and
  `Outcome` JSON-backed project fields.
- Produces: `buildProjectDossiers([]types.Project) []projectDossier` with the
  best available destination and accessible external-link metadata.
- Consumes: one `SignalTrail(TrailDossier)` between lead and supporting work.

- [ ] **Step 1: Write failing data and route tests**

Require exactly one featured project, allowed ratios `landscape`, `portrait`,
or `square`, nonempty problem/approach/outcome fields, stable project order,
one route marker, and one trail. Test DemoURL preference, GitHub fallback, and
internal Soccer links without new-tab behavior.

- [ ] **Step 2: Run focused tests and confirm metadata is absent**

```sh
go test ./cmd/web/partials ./internal/portfolio ./internal/app \
  -run 'TestBuildProjectDossiers|TestEmbeddedData|TestBuildMuxPublicRoute' \
  -count=1
```

- [ ] **Step 3: Migrate existing project copy into explicit fields**

Restructure each record's existing `intro` and `description` wording into
`problem`, `approach`, and `outcome` without inventing claims or dropping the
existing facts. Add only label-level connective words when a sentence must be
split. Mark one lead record featured, assign a ratio, and remove filename-based
`projectImageHeight` branching.

- [ ] **Step 4: Recompose the dossier hierarchy**

Render the lead dossier wide with labeled problem, approach, outcome,
technology, and destination sections. Follow it with smaller asymmetric briefs
whose hierarchy comes from metadata, not array index or filename. Preserve
accessible external-link text and internal-link semantics.

- [ ] **Step 5: Import route CSS, place the trail, and remove old rules**

Add `pages/projects.css` to `app.css`. Base layout is one column; use explicit
metadata-driven placement at tablet/wide widths. Place the one dossier trail
between lead and supporting work. Remove migrated singular `.project-*` and
plural `.projects-*` rules from legacy CSS.

- [ ] **Step 6: Generate, test, and inspect**

```sh
task generate
task fmt
task tailwind-build
go test ./cmd/web/partials ./internal/portfolio ./internal/app -count=1
git diff --check
```

Review every width, all image ratios and failures, long technology chips,
internal/external destinations, focus, and overflow.

- [ ] **Step 7: Record the safe checkpoint**

If independently buildable, use
`git commit -m "feat: build project case study dossiers"`; otherwise skip it.

---

### Task 7: Build the Education Learning Field Guide

**Files:**

- Create: `cmd/web/tailwind/pages/education.css`
- Create: `cmd/web/pages/education_viewmodels.go`
- Create: `cmd/web/pages/education_viewmodels_test.go`
- Modify: `cmd/web/tailwind/app.css`
- Modify: `cmd/web/pages/education.templ`
- Modify: `cmd/web/tailwind/components.css`
- Modify: `cmd/web/tailwind/emberglass.css`
- Modify: `cmd/web/tailwind/emberglass-responsive.css`
- Modify: `internal/app/server_render_smoke_test.go`

**Interfaces:**

- Produces: `data-layout="learning-field-guide"`.
- Produces: five explicit, nonoverlapping credential domains with stable IDs
  `cloud`, `microsoft`, `linux`, `security`, and `delivery`.
- Consumes: one `SignalTrail(TrailFieldGuide)` from degree to credentials.

- [ ] **Step 1: Write failing mapping, hierarchy, and route tests**

Require this exact assignment with no duplicate or omitted credential title:

- Cloud: AWS Cloud Practitioner, Azure Fundamentals, and CompTIA Cloud+.
- Microsoft: MCSE Cloud Platform and MCSA Windows Server.
- Linux: Linux Essentials.
- Security: CompTIA Security+.
- Delivery: CompTIA Network+, Project+, and A+.

Require ten credentials total, the route marker, one trail, one page `h1`, and
the degree title at `h3` beneath its section `h2`.

- [ ] **Step 2: Run focused tests and confirm the builder is absent**

```sh
go test ./cmd/web/pages ./internal/app \
  -run 'TestEducationCredentialDomains|TestBuildMuxPublicRoute' -count=1
```

- [ ] **Step 3: Move credential data decisions out of Templ**

Define each card directly inside its domain; do not infer grouping by provider
or title at render time. The first credential in DOM order loads eagerly and
the remaining nine load lazily. Remove the existing final-card eager exception.

- [ ] **Step 4: Correct semantics and build the field guide**

Keep the degree as one foundational feature and render domains in stable DOM
order while their real sizes shape the mosaic. Credential images stay
secondary to qualification/provider text. Preserve a sequential heading tree.

- [ ] **Step 5: Import CSS, place the trail, and remove old ownership**

Add `pages/education.css` to `app.css`. Let `TrailFieldGuide` connect the degree
foundation to the domain sequence. Remove Education rules from legacy CSS after
comparison and reset every changed grid span at each breakpoint.

- [ ] **Step 6: Generate, test, and inspect**

```sh
task generate
task fmt
task tailwind-build
go test ./cmd/web/pages ./internal/app -count=1
git diff --check
```

Review every width, heading order, all ten credentials, image failure,
long names, focus, and overflow.

- [ ] **Step 7: Record the safe checkpoint**

If independently buildable, use
`git commit -m "feat: build education field guide"`; otherwise skip it.

---

### Task 8: Build the Contact Correspondence Window

**Files:**

- Create: `cmd/web/tailwind/pages/contact.css`
- Modify: `cmd/web/tailwind/app.css`
- Modify: `cmd/web/pages/contact.templ`
- Modify: `cmd/web/tailwind/components.css`
- Modify: `cmd/web/tailwind/emberglass.css`
- Modify: `cmd/web/tailwind/emberglass-responsive.css`
- Modify: `internal/app/server_render_smoke_test.go`

**Interfaces:**

- Produces: `data-layout="correspondence-window"` and regions `intro`,
  `availability`, `channels`, and `expertise`.
- Consumes: one `SignalTrail(TrailCorrespondence)`.

- [ ] **Step 1: Write failing route-order and hierarchy tests**

Require the layout marker, one trail, and DOM order `intro < availability <
channels < expertise`. Require email to use the sole primary action variant and
external profiles to use secondary variants.

- [ ] **Step 2: Run the smoke test and confirm the contract fails**

```sh
go test ./internal/app -run TestBuildMuxPublicRouteRenderingSmoke -count=1
```

- [ ] **Step 3: Recompose around correspondence**

Keep all channels and expertise content. Put the compact availability ticket
immediately after the introduction in DOM order so mobile reading order is
correct. Use CSS at `70rem` and above to place that same ticket beside and
sticky relative to the channel list; do not render a duplicate mobile copy.

- [ ] **Step 4: Import CSS and define resilient text behavior**

Add `pages/contact.css` to `app.css`. Keep availability static below `70rem` and
sticky above it. Apply `overflow-wrap: anywhere` to long email/profile text
without shrinking icons or 44px actions. Keep the expertise ribbon quieter
than the email action.

- [ ] **Step 5: Place the trail and remove old Contact rules**

Use the one trail to connect invitation, availability, and channel choice.
Remove Contact declarations from legacy CSS after browser comparison.

- [ ] **Step 6: Generate, test, and inspect**

```sh
task generate
task fmt
task tailwind-build
go test ./internal/app -count=1
git diff --check
```

Review every width, mobile DOM order, long URLs/email, sticky behavior, external
link labels, focus, and overflow.

- [ ] **Step 7: Record the safe checkpoint**

If independently buildable, use
`git commit -m "feat: build contact correspondence window"`; otherwise skip it.

---

### Task 9: Make Skills a Searchable Progressive HTMX Workbench

**Files:**

- Create: `cmd/web/tailwind/pages/skills.css`
- Create: `cmd/web/partials/skills_viewmodels.go`
- Create: `cmd/web/partials/skills_viewmodels_test.go`
- Create: `internal/portfolio/skills_viewmodel.go`
- Create: `internal/portfolio/handlers_test.go`
- Modify: `cmd/web/tailwind/app.css`
- Modify: `cmd/web/pages/skills.templ`
- Modify: `cmd/web/partials/skills_grid.templ`
- Modify: `cmd/web/partials/skill_detail.templ`
- Modify: `internal/portfolio/handlers.go`
- Modify: `cmd/web/static/js/main.js`
- Modify: `cmd/web/tailwind/components.css`
- Modify: `cmd/web/tailwind/emberglass.css`
- Modify: `cmd/web/tailwind/emberglass-responsive.css`
- Modify: `internal/app/server_render_smoke_test.go`

**Interfaces:**

- Produces: `SkillFilters{Query, Category, Proficiency string}` and exported
  `NormalizeSkillFilters(url.Values, []types.SkillCategory) SkillFilters` in
  `partials`; accepted proficiency values remain `familiar`, `intermediate`,
  `advanced`, and `expert`.
- Produces: `BuildSkillsGridProps(categories, filters) SkillsGridProps` in
  `partials` and `buildSkillsPageProps` in `internal/portfolio`, avoiding a
  `partials` to `pages` import cycle.
- Produces: full-page GET
  `/skills?q=<text>&category=<value>&proficiency=<value>`. Normal HTMX requests
  return the filterable fragment; `HX-History-Restore-Request: true` returns a
  full document for cache-miss restoration.
- Produces: `data-layout="skills-workbench"` and exactly one
  `SignalTrail(TrailWorkbench)` in the full-page workbench wrapper immediately
  before the replaceable catalog; Skills fragments contain no trail.
- Preserves: `/skills/filtered` as a fragment compatibility endpoint and
  `/skills/detail` as the detail endpoint.

- [ ] **Step 1: Write failing filter normalization and response-mode tests**

Test valid values, invalid values, case-insensitive query matching, normal
full-page HTML, HTMX fragments, and HTMX history cache misses:

```go
func TestSkillsHandlerUsesURLFiltersAndHTMXResponseMode(t *testing.T) {
    req := httptest.NewRequest(
        http.MethodGet,
        "/skills?q=terraform&category=Cloud+Platforms&proficiency=advanced",
        nil,
    )
    req.Header.Set("HX-Request", "true")
    res := httptest.NewRecorder()
    SkillsHandler(res, req)
    body := res.Body.String()
    if strings.Contains(body, "<!DOCTYPE html>") {
        t.Fatal("HTMX request returned a full document")
    }
    if !strings.Contains(body, `id="skills-filterable"`) ||
        !strings.Contains(body, `id="skills-filter-controls"`) {
        t.Fatalf("filtered fragment missing result or control target: %s", body)
    }
}
```

Add separate tests that omit `HX-Request` and require `<!DOCTYPE html>`, send an
invalid category/proficiency and require normalized inactive filters, and send
both `HX-Request: true` and `HX-History-Restore-Request: true` and require a
full page.

- [ ] **Step 2: Run focused tests and confirm `/skills` ignores query/HTMX mode**

Run:

```sh
go test ./internal/portfolio ./internal/app \
  -run 'TestSkillsHandlerUsesURLFilters|TestBuildMuxPublicRouteRenderingSmoke' \
  -count=1
```

Expected: the new response-mode test fails because `/skills` always returns the
full unfiltered page.

- [ ] **Step 3: Move filter state into Go view models**

Validate category against actual names and proficiency against the existing
closed values `familiar`, `intermediate`, `advanced`, and `expert`; invalid
values normalize to empty. Trim query whitespace, cap it at 80 Unicode code
points, and match name, description, and category case-insensitively. Build
filter options, visible categories, visible counts, and the no-result message in
Go so Templ only renders prepared state.

- [ ] **Step 4: Make filter controls real links with HTMX enhancement**

Render category and proficiency options as `<a>` elements whose `href` is the
complete `/skills` URL and preserves `q` plus the other active filter. Add:

```templ
hx-get={ option.Href }
hx-target="#skills-filterable"
hx-swap="outerHTML"
hx-push-url="true"
hx-sync="#skills-filterable:replace"
```

Use `aria-current="page"` on the link representing each active filter. Do not
keep inert `<button>` controls that fail without HTMX. The response to any
enhanced filter request includes both the normal catalog root and the OOB
control root specified in Step 6. Put the loading status outside the two-column
filter grid so it cannot occupy an orphaned cell.

- [ ] **Step 5: Add a progressive search form**

Render `<form method="get" action="/skills">` with a labeled
`<input type="search" name="q">`, hidden inputs for active category and
proficiency, and a visible Search button. Enhance that form with `hx-get`, the
same target/swap/push/sync contract and its default `submit` trigger so Enter
and the button work. Because `hx-get` and `hx-trigger` are not inherited, put a
second complete request contract directly on the search input:

```templ
hx-get="/skills"
hx-trigger="input changed delay:300ms"
hx-include="closest form"
hx-target="#skills-filterable"
hx-swap="outerHTML"
hx-push-url="true"
hx-sync="#skills-filterable:replace"
```

Both paths target the same catalog and receive the same OOB control update.
Without HTMX, form submission navigates to the complete filtered page. Make
`/skills/filtered` accept and normalize `q` identically while always returning
the compatibility fragment.

- [ ] **Step 6: Make detail focus and history behavior exact**

Give every skill trigger `data-skill-detail-trigger` and every returned heading
the stable ID `skill-detail-heading-<id>`, `data-skill-detail-heading`, and
`tabindex="-1"`. Only a request initiated by a marked trigger focuses the new
heading. Closing restores focus to the trigger whose `aria-controls` names that
slot.

Put `hx-history-elt` on the stable `.skills-workbench-history` ancestor outside
and above both the persistent trail and `#skills-filterable`; that ancestor is
never an HTMX swap target. The replaceable element contains results and the
result count, while the controls are the stable sibling
`#skills-filter-controls` above the trail. Every fragment response has two
roots: `#skills-filterable` for the normal outerHTML swap and a freshly rendered
`#skills-filter-controls` with `hx-swap-oob="outerHTML"`. The OOB controls
update active links, preserved hrefs, hidden inputs, search value, and result
summary from the same Go view model without a client-side store.
Accept HTMX's cached history restoration; on `htmx:historyRestore`, reinitialize
remote images and skill interactions without forcing navigation. When HTMX
requests a cache miss with `HX-History-Restore-Request`, the handler returns the
full page. Do not add a separate client-side filter store.

- [ ] **Step 7: Import and compose the workbench CSS**

Add `pages/skills.css` to `app.css`. Use one featured capability mosaic, one
practice band, and one searchable/filterable catalog. Put the single trail
after filter/search controls and before the replaceable catalog root, so it
communicates filter-to-result flow without being duplicated by fragment swaps.
On compact screens, filter chips scroll locally with a visible edge fade and
text hint. The page never scrolls horizontally.

- [ ] **Step 8: Remove old Skills rules and exercise HTMX behavior**

Remove both `.skill-*` and `.skills-*` presentation rules from legacy CSS after
browser comparison. Preserve only selector/data contracts that JavaScript or
HTMX still consumes, now styled in the route module.

Run:

```sh
task generate
task fmt
task tailwind-build
go test ./cmd/web/partials ./internal/portfolio ./internal/app -count=1
node --check cmd/web/static/js/main.js
git diff --check
```

Browser-check default, search-only, category, proficiency, combined, invalid,
no-result, rapid-change, detail-open, detail-close, refresh, cached back/forward,
and cache-miss history restoration at 390px and 1440px. Exercise typing delay,
Enter, and button submit while a prior request is in flight; the last input wins
and the URL matches the visible state. During requests require `aria-busy=true`
on `#skills-filterable`; after settlement require `false`. Assert trail count is
one before and after every filter, search, and history swap.

- [ ] **Step 9: Record the safe checkpoint**

Use the conventional commit when safe:

```sh
git commit -m "feat: build searchable skills workbench"
```

---

### Task 10: Refine Soccer into a Matchday Planner

**Files:**

- Create: `cmd/web/partials/soccer_viewmodels.go`
- Create: `cmd/web/partials/soccer_viewmodels_test.go`
- Create: `cmd/web/partials/soccer_security_notice.templ`
- Modify: `cmd/web/tailwind/app.css`
- Modify: `cmd/web/pages/soccer.templ`
- Modify: `cmd/web/partials/soccer_login_state.templ`
- Modify: `cmd/web/partials/soccer_login_modal.templ`
- Modify: `cmd/web/partials/soccer_login_feedback.templ`
- Modify: `cmd/web/partials/soccer_player_select.templ`
- Modify: `cmd/web/partials/soccer_team_select.templ`
- Modify: `cmd/web/partials/soccer_table_fragment.templ`
- Rewrite: `cmd/web/tailwind/soccer.css`
- Modify: `cmd/web/tailwind/emberglass.css`
- Modify: `cmd/web/tailwind/emberglass-responsive.css`
- Modify: `cmd/web/static/js/main.js`
- Create: `internal/app/ui_preview.go`
- Create: `internal/app/ui_preview_test.go`
- Modify: `internal/app/server.go`
- Modify: `internal/app/handlers_soccer_test.go`
- Modify: `internal/app/handlers_soccer_schedule_test.go`

**Interfaces:**

- Produces: methods `SupportsDiscovery() bool` and `SupportsGoogle() bool` on
  `SoccerLoginStateProps`.
- Produces: `SoccerSecurityNoticeProps{Compact bool}` and one shared warning
  component used by the workflow and modal.
- Uses: shared `Feedback` for info, success, warning, and error anatomy.
- Produces: `data-layout="matchday-planner"` and one
  `SignalTrail(TrailMatchday)` through the five workflow stages.
- Produces: loopback-only `/__preview/soccer/{fixture}` pages for `manual`,
  `import`, `token-invalid`, `token-expired`, `token-rejected`,
  `token-upstream-error`, `players`, `no-players`, `team-selection`, `no-games`,
  `upcoming`, `past`, `combined`, `google-disconnected`, `google-connected`,
  `google-add-success`, `google-add-error`, `google-sync-success`,
  `google-sync-error`, `expired-session-reset`, and `loading` while safe local
  preview mode is enabled.
- Produces: `soccerPreviewFixture(name string) (soccerPreviewPage, bool)` where
  `soccerPreviewPage` contains the full-page props and optional results fragment
  props needed for one fixture; unknown names return `false`.
- Produces: loopback-preview-only `POST /__preview/soccer/download` that accepts
  the same selected-game fields, validates them against the in-memory fixture
  game set, and returns an ICS attachment without calling LPS. Production keeps
  the existing `/soccer/download` endpoint and never registers the preview
  download route.
- Preserves: `soccer-auth-panel`, `soccer-login-feedback`, `games-container`,
  `loading-indicator`, `data-game-*`, `data-loading-*`, and `soccer-logout`.

- [ ] **Step 1: Write failing capability and shared-copy tests**

Add table tests for manual-only, login-enabled, authenticated, Google-available,
and Google-connected combinations:

```go
func TestSoccerLoginStateCapabilities(t *testing.T) {
    props := SoccerLoginStateProps{LoginAvailable: true}
    if !props.SupportsDiscovery() || props.SupportsGoogle() {
        t.Fatalf("login-only capabilities are incorrect: %#v", props)
    }
    props = SoccerLoginStateProps{GoogleConnected: true}
    if props.SupportsDiscovery() || !props.SupportsGoogle() {
        t.Fatalf("google-only capabilities are incorrect: %#v", props)
    }
}
```

Add a render assertion that the security notice text occurs once in the page
workflow and once in the modal through the same component marker. Require the
`matchday-planner` layout marker, one page trail, and no trail in standalone
Soccer fragments. Add a table test that every named preview fixture resolves to
the expected auth, selection, result, feedback, Google, or expired-session state
and that an unknown name fails closed.

- [ ] **Step 2: Run focused tests and confirm duplicate predicates remain**

Run:

```sh
go test ./cmd/web/partials ./internal/app \
  -run 'TestSoccerLoginStateCapabilities|TestSoccerPageRenders' \
  -count=1
```

Expected: compilation fails because the exported capability methods do not
exist.

- [ ] **Step 3: Centralize capabilities and sensitive-copy rendering**

Move the two predicates out of page and Templ helper duplication into methods
defined in `soccer_viewmodels.go`. Extract the JWT explanation into
`SoccerSecurityNotice`; use concise and full layouts through `Compact`, while
keeping the security meaning identical.

- [ ] **Step 4: Recompose the page as one visible workflow**

Order the page as:

1. Connect/import or choose manual team IDs.
2. Confirm players and teams when discovery is available.
3. Review returned games.
4. Select none, some, or all.
5. Download ICS or add/sync Google Calendar.

Keep manual fallback available without visually competing with the recommended
path. Keep existing backend form names and endpoints.

- [ ] **Step 5: Unify feedback and loading states**

Map Soccer feedback kinds to typed `FeedbackKind`. Give empty results, no games,
invalid input, expired session, upstream failure, Google failure, and success
the shared icon/summary/detail/next-action anatomy. Every HTMX target exposes
`aria-busy` while requesting and retains a useful minimum height.

- [ ] **Step 6: Add safe visual fixtures for every state family**

Register `internal/app/ui_preview.go` routes only when the existing local
preview flag has already passed its loopback-address safety check. Build props
from in-memory fixtures; never initialize Soccer persistence, Google storage,
or upstream HTTP clients. In preview fixture forms, point native download to
`/__preview/soccer/download`; generate its ICS from the fixture set through the
same calendar serialization helper used by production after schedule fetching.
Unknown fixture names/game IDs return 404/400. Tests require both preview route
families to be absent outside preview mode, the download to set the existing
attachment/content-type headers, and every preview request to need no outbound
dependency.

- [ ] **Step 7: Rebuild Matchday CSS without legacy overrides**

Keep `soccer.css` in the `pages` layer. Preserve local scrolling for game
tables, the visible scroll hint, focusable region, modal safe-area handling, and
44px controls. The selection toolbar is static below `48rem` and sticky beneath
the header at `48rem` and above. Use route signal colors for upcoming versus
completed sections without encoding result meaning by color alone. Make the
workflow trail the only pitch/path decoration and remove the old
`.soccer-hero::after` field drawing.

- [ ] **Step 8: Remove old Soccer ownership and verify dynamic states**

Remove migrated Soccer selectors from `components.css`, `emberglass.css`, and
`emberglass-responsive.css`; `soccer.css` becomes their only owner.

Run:

```sh
task generate
task fmt
task tailwind-build
go test ./cmd/web/partials ./internal/soccer ./internal/google ./internal/app -count=1
node --check cmd/web/static/js/main.js
git diff --check
```

Browser-review every local fixture plus modal open/close/focus trap,
none/partial/all selection, `aria-busy` request transitions, and native ICS
download through the loopback preview endpoint. Handler tests with an injected
fake LPS client continue to cover production `/soccer/download`. The Google
fixtures are visual only and send no requests. Do not submit real credentials
during visual QA.

- [ ] **Step 9: Record the safe checkpoint**

Use the conventional commit when safe:

```sh
git commit -m "feat: refine soccer matchday workflow"
```

---

### Task 11: Build the Responsive Portal Operator Workspace

**Files:**

- Create: `cmd/web/tailwind/portal.css`
- Create: `cmd/web/partials/portal_viewmodels.go`
- Create: `cmd/web/partials/portal_viewmodels_test.go`
- Modify: `cmd/web/tailwind/app.css`
- Modify: `cmd/web/pages/portal_mgmt.templ`
- Modify: `cmd/web/pages/portal_error.templ`
- Modify: `cmd/web/partials/portal_fragments.templ`
- Modify: `internal/portal/preview.go`
- Modify: `internal/portal/preview_test.go`
- Modify: `internal/portal/mgmt_test.go`
- Modify: `internal/app/server_portal_preview_test.go`
- Modify: `internal/app/server_render_smoke_test.go`
- Modify: `internal/app/ui_preview.go`
- Modify: `internal/app/ui_preview_test.go`
- Modify: `cmd/web/static/js/main.js`
- Modify: `cmd/web/tailwind/components.css`
- Modify: `cmd/web/tailwind/emberglass.css`
- Modify: `cmd/web/tailwind/emberglass-responsive.css`

**Interfaces:**

- Consumes: `layouts.ShellOperator`, typed actions, feedback, and overflow
  region.
- Produces: `PortalState` constants for `pending`, `running`, `stopping`,
  `stopped`, `shutting-down`, and `terminated`.
- Produces: `portalStatePresentation(state string) PortalStateView` with
  `Class`, `Label`, and `Description`.
- Produces: `data-layout="operator-workspace"`, one
  `SignalTrail(TrailOperator)` on the dashboard, and one
  `SignalTrail(TrailInterruption)` on the error page.
- Produces: loopback-only fixtures `/mgmt?fixture=empty`,
  `/mgmt?fixture=retrieval-error`, `/__preview/portal/error`, and
  `/mgmt/instances/i-0f1e2d3c4b5a69788/metrics?fixture=empty|error` plus the
  matching `/logs?fixture=empty|error` URLs. Keep that running instance ID
  stable in `previewInstances()` as the fixture anchor.
- Produces: closed `PortalPreviewFixture` constants and
  `parsePortalPreviewFixture(string) (PortalPreviewFixture, bool)`; an empty
  value selects normal preview data and unknown nonempty values return 404.
- Preserves: every `/mgmt/instances/{id}` endpoint and
  `X-Portal-Fragment-Error` swap contract.

- [ ] **Step 1: Write failing normalized-state and preview-coverage tests**

Add:

```go
func TestPortalStatePresentationCoversEC2Lifecycle(t *testing.T) {
    states := []string{
        "pending", "running", "stopping", "stopped",
        "shutting-down", "terminated",
    }
    for _, state := range states {
        view := portalStatePresentation(state)
        if view.Class == "portal-state-unknown" ||
            view.Label == "" || view.Description == "" {
            t.Errorf("state %q is not fully presented: %#v", state, view)
        }
    }
}
```

Require `previewInstances()` to include all six states so browser review does
not depend on AWS. Require every named fixture to work only in safe local
preview mode. Require the `operator-workspace` layout marker and exactly one
trail on both dashboard and error pages.

- [ ] **Step 2: Run tests and confirm rare states lack presentation**

Run:

```sh
go test ./cmd/web/partials ./internal/portal ./internal/app \
  -run 'TestPortalStatePresentation|TestPreview' \
  -count=1
```

Expected: the new presentation API is absent and preview lacks lifecycle states.

- [ ] **Step 3: Normalize portal state and action presentation**

Map unknown states to a neutral `Unknown` label and disable destructive actions
when state semantics are not known. Render the normalized label while retaining
the raw state in a data attribute for diagnostics.

- [ ] **Step 4: Make every required Portal state reachable without AWS**

Implement the named preview fixtures with in-memory data. Dashboard
`fixture=empty` returns zero instances; `fixture=retrieval-error` supplies the
existing retrieval-error prop. On the stable running instance, these exact URLs
return typed empty content or an `X-Portal-Fragment-Error` feedback fragment:

```text
/mgmt/instances/i-0f1e2d3c4b5a69788/metrics?fixture=empty
/mgmt/instances/i-0f1e2d3c4b5a69788/metrics?fixture=error
/mgmt/instances/i-0f1e2d3c4b5a69788/logs?fixture=empty
/mgmt/instances/i-0f1e2d3c4b5a69788/logs?fixture=error
```

The preview error route renders `PortalError` directly. Reject unknown fixtures
with 404. Production handlers ignore fixture parameters and never register
`/__preview/*`.

- [ ] **Step 5: Convert the instance table into responsive cards**

Keep one semantic table. Add `data-label` to each cell and convert each instance
row to a CSS grid card below `48rem`; at `48rem` and above use the table. Hide
the header visually only in card mode while preserving header relationships.
Keep each detail row immediately after its instance row. When both target
containers are empty, hide the detail row; after a metrics/log swap, show it,
join its border/background to the preceding card, and stack its panels. Do not
render a second mobile copy with duplicate IDs.

- [ ] **Step 6: Add explicit table guidance and operator shell**

Use `OverflowRegion` at `48rem` and above with a visible scroll hint when the
table is wider than its container. Render dashboard and error pages with
`ShellOperator` and its compact identity/back-to-portfolio footer. Keep session
and preview status prominent and style empty/retrieval/error pages with shared
feedback anatomy.

- [ ] **Step 7: Preserve HTMX error, expansion, and focus behavior**

Retain the response-header override in `main.js`. On `keydown` for Enter or
Space on a marked Portal detail control, set `data-focus-after-swap="true"` on
that control. A pointerdown clears the marker. In `htmx:afterRequest`, response
error, and send error, clear stale markers after processing. In
`htmx:afterSwap`, resolve the initiating control from
`evt.detail.requestConfig.elt`; focus the loaded region only when that exact
control still has the marker. Pointer activation announces but does not move
focus. Add browser checks for both paths and cleanup after failed requests.

Set every metrics/log control's initial `aria-expanded="false"`. On a completed
swap set the initiating control to `true`, reveal only that instance's detail
row, and preserve all other expanded rows. Empty and error responses remain
visible and announced. On `htmx:beforeRequest`, set the exact target region's
`aria-busy="true"`; on after-swap, after-request, response-error, and send-error
set it back to `false`. Add handler/browser checks covering pending, success,
empty, and error paths for both metrics and logs.

- [ ] **Step 8: Remove old Portal rules, generate, and review**

Keep `portal.css` in the `pages` layer and remove `.portal-*` presentation from
legacy component/Emberglass files after comparison.

Run:

```sh
task generate
task fmt
task tailwind-build
go test ./cmd/web/partials ./internal/portal ./internal/app -count=1
node --check cmd/web/static/js/main.js
git diff --check
```

Run the loopback-only mock portal and review all six states, disabled actions,
action success, invalid-ID error, loaded/empty metrics, loaded/empty logs,
retrieval error, empty instance list, and portal error page at all required
widths. Verify card/table switch on both sides of 48rem and one trail per full
page.

- [ ] **Step 9: Record the safe checkpoint**

Use the conventional commit when safe:

```sh
git commit -m "feat: build responsive portal workspace"
```

---

### Task 12: Enforce Single-Owner CSS and Remove Migration Layers

**Files:**

- Create: `internal/app/style_architecture_test.go`
- Modify: `cmd/web/tailwind/app.css`
- Rewrite: `cmd/web/tailwind/components.css`
- Modify: every source file under `cmd/web/tailwind/pages/`
- Modify: `cmd/web/tailwind/soccer.css`
- Modify: `cmd/web/tailwind/portal.css`
- Modify: `cmd/web/tailwind/theme.css`
- Modify: `cmd/web/tailwind/shared.css`
- Modify: `cmd/web/tailwind/base.css`
- Delete: `cmd/web/tailwind/emberglass.css`
- Delete: `cmd/web/tailwind/emberglass-responsive.css`
- Delete: `cmd/web/tailwind/emberglass-accessibility.css`

**Interfaces:**

- Produces: one explicit import graph with shared component internals before
  route composition and utilities last.
- Produces: source-level tests preventing reintroduction of override files,
  noncanonical media thresholds, missing reduced-motion delay reset, and
  route selectors in shared component CSS.
- Consumes: every incrementally migrated component and route from Tasks 1
  through 11; this task is an enforcement/removal audit, not a second redesign.

- [ ] **Step 1: Write the failing architecture contract**

Use `runtime.Caller` to locate the repository root. Parse nonblank `app.css`
directives and compare their complete order, not unordered substring presence:

```go
func TestTailwindSourceArchitecture(t *testing.T) {
    appCSS := readTailwindSource(t, "app.css")
    wantImports := []string{
        `@import "tailwindcss/theme.css" layer(theme);`,
        `@import "./theme.css" layer(theme);`,
        `@import "./shared.css" layer(theme);`,
        `@import "./base.css" layer(base);`,
        `@import "./components.css" layer(components);`,
        `@import "./pages/home.css" layer(pages);`,
        `@import "./pages/about.css" layer(pages);`,
        `@import "./pages/experience.css" layer(pages);`,
        `@import "./pages/skills.css" layer(pages);`,
        `@import "./pages/projects.css" layer(pages);`,
        `@import "./pages/education.css" layer(pages);`,
        `@import "./pages/contact.css" layer(pages);`,
        `@import "./soccer.css" layer(pages);`,
        `@import "./portal.css" layer(pages);`,
        `@import "tailwindcss/utilities.css" layer(utilities);`,
    }
    if got := importDirectives(appCSS); !slices.Equal(got, wantImports) {
        t.Fatalf("imports = %#v, want %#v", got, wantImports)
    }
}
```

Also require the exact layer declaration, all four current `@source`
directives, no duplicate imports, and no outer `@layer` wrapper in imported
project files. Use `os.Stat` plus `errors.Is(err, fs.ErrNotExist)` to prove all
three migration files are deleted.

Add source scans that:

- require `--breakpoint-*: initial` plus only `sm`, `md`, `lg`, and `xl` at the
  four approved values;
- reject `xs:`, `2xl:`, arbitrary min/max-width variants, and raw noncanonical
  width media features across `.templ`, Tailwind source, and `main.js`;
- require the sole JavaScript width contract to be the canonical navigation
  max-boundary query `(max-width: 69.999rem)`;
- permit non-width reduced-motion, forced-colors, hover, and pointer queries;
- reject both `--pollen-gold` and `#FFD166`;
- require `animation-delay: 0s !important` in reduced motion;
- reject `.home-`, `.about-`, `.experience-`, `.skill-`, `.skills-`,
  `.project-`, `.projects-`, `.education-`, `.contact-`, `.soccer-`, and
  `.portal-` selectors in `components.css`.

- [ ] **Step 2: Run the architecture test and confirm the old import graph fails**

Run:

```sh
go test ./internal/app -run TestTailwindSourceArchitecture -count=1
```

Expected: failure while any migration import/file, old literal color, nested
layer wrapper, forbidden responsive variant, or misplaced route selector
remains.

- [ ] **Step 3: Audit residual migration rules one family at a time**

At this point Tasks 1–11 have already moved and visually compared each owned
family. Inventory any remaining selector in the three migration files. For each
residual family: name its canonical owner, move it, compile, compare the affected
route at one narrow and one wide width, then delete only that residual block.
Never copy an entire migration sheet into a new file or restore an override
after a regression.

- [ ] **Step 4: Finalize the complete ordered import graph**

Use this final import order:

```css
@layer theme, base, components, pages, utilities;

@import "tailwindcss/theme.css" layer(theme);
@import "./theme.css" layer(theme);
@import "./shared.css" layer(theme);
@import "./base.css" layer(base);
@import "./components.css" layer(components);
@import "./pages/home.css" layer(pages);
@import "./pages/about.css" layer(pages);
@import "./pages/experience.css" layer(pages);
@import "./pages/skills.css" layer(pages);
@import "./pages/projects.css" layer(pages);
@import "./pages/education.css" layer(pages);
@import "./pages/contact.css" layer(pages);
@import "./soccer.css" layer(pages);
@import "./portal.css" layer(pages);
@import "tailwindcss/utilities.css" layer(utilities);
```

Keep `@source` declarations before project imports. Remove complete legacy light
theme blocks, duplicate component definitions, compatibility aliases with no
remaining consumers, and repeated media rules. Imported project files have no
outer `@layer` wrappers because `app.css` owns layer assignment.

- [ ] **Step 5: Prove shared CSS contains no route composition**

Run:

```sh
rg -n '\.(home|about|experience|skill|skills|project|projects)-' \
  cmd/web/tailwind/components.css
rg -n '\.(education|contact|soccer|portal)-' \
  cmd/web/tailwind/components.css
```

Expected: no route-prefixed selectors in `components.css`. Shared component
names such as `.page-hero`, `.action`, `.feedback`, `.stat-grid`, and
`.overflow-region` remain there; route selectors live only in their page file.

- [ ] **Step 6: Prove compiled breakpoints and forced colors**

Build Tailwind, then run the architecture test with
`VERIFY_COMPILED_CSS=1`. In that mode, parse every compiled width media feature
and accept only 30, 48, 70, and 80rem plus the Tailwind max-boundary subtraction.
Fail on 40rem, 64rem, raw pixel width features, or any other threshold.

Temporarily remove the old accessibility import before deleting its source.
Emulate forced colors and inspect computed border, color, background,
outline, and `forced-color-adjust` on navigation, actions, feedback, table,
modal, portal controls, and every trail variant. Fix the generic `base.css`
contract until it wins without the old unlayered sheet.

- [ ] **Step 7: Delete migration files and run the full application suite**

Run:

```sh
task generate
task fmt
task tailwind-build
VERIFY_COMPILED_CSS=1 go test ./internal/app \
  -run TestTailwindSourceArchitecture -count=1
go test ./...
node --check cmd/web/static/js/main.js
git diff --check
```

Expected: all checks pass; all page-module sentinel selectors are present; no
migration file, forbidden breakpoint, nested layer, or seventh core color
remains.

- [ ] **Step 8: Run the final post-deletion route comparison**

Build and reload every route after the deletion and final rebuild. If a
component regresses, fix its canonical owner; do not restore an override file.

- [ ] **Step 9: Record the safe checkpoint**

Use the conventional commit when safe:

```sh
git commit -m "refactor: enforce single-owner style architecture"
```

---

### Task 13: Perform Full Visual, Responsive, and Accessibility QA

**Files:**

- Create: `docs/superpowers/qa/2026-08-12-emberglass-field-station-2.md`
- Modify: any in-scope source file where the browser reveals a defect
- Modify: relevant regression test for every defect fixed

**Interfaces:**

- Consumes: all route and component work from Tasks 1 through 12.
- Produces: a checked evidence matrix with route, viewport, state, overflow
  metrics, target sizing, trail count, keyboard/focus result, contrast result,
  busy/loading result, visual result, screenshot path, and follow-up fix.

- [ ] **Step 1: Start one loopback preview with mock portal data**

Run:

```sh
lsof -nP -iTCP:8182 -sTCP:LISTEN
task build
env PORT=8182 HOST=127.0.0.1 APP_BIND_ALL=false MGMT_LOCAL_PREVIEW=true ./portfolio-server
```

Expected: the port check returns no listener, the site listens on
`http://127.0.0.1:8182`, Soccer runs in its configured local capability state,
and `/mgmt` uses harmless mock data. Keep the server in a managed terminal
session and stop it after the final inspection.

- [ ] **Step 2: Create the evidence matrix before testing**

Create rows for `/`, `/about`, `/experience`, `/skills`, `/projects`,
`/education`, `/contact`, `/soccer`, `/mgmt`, and
`/__preview/portal/error` at 390, 768, 1119, 1121, and 1440 pixels. Use columns
for state/fixture, screenshot, `clientWidth`, `scrollWidth`, trail count, heading
and ID validity, 44px targets, focus/clipping/header coverage, contrast,
reduced-motion/forced-colors, `aria-busy`, visual result, and fix reference.
Add state rows for mobile navigation, Skills search/filter/detail/history, every
Soccer preview fixture, and every Portal preview fixture. Use `Pending`, `Pass`,
or `Fail`; do not leave blank cells.

- [ ] **Step 3: Run structural DOM checks at every route/width pair**

For each route, evaluate:

```js
const isVisible = element => {
  const style = getComputedStyle(element)
  const rect = element.getBoundingClientRect()
  return style.visibility !== 'hidden' && style.display !== 'none' &&
    rect.width > 0 && rect.height > 0
}
const controls = [...document.querySelectorAll([
  'a[href]',
  'button:not([disabled])',
  'input:not([type="hidden"]):not([disabled])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  'summary',
  '[role="button"]',
].join(','))].filter(isVisible)
({
  clientWidth: document.documentElement.clientWidth,
  scrollWidth: document.documentElement.scrollWidth,
  h1Count: document.querySelectorAll('h1').length,
  trailCount: document.querySelectorAll('[data-signal-trail]').length,
  duplicateIds: [...document.querySelectorAll('[id]')]
    .map(element => element.id)
    .filter((id, index, ids) => ids.indexOf(id) !== index),
  smallTargets: controls.filter(element => {
    const rect = element.getBoundingClientRect()
    return rect.width < 44 || rect.height < 44
  })
    .map(element => element.id || element.textContent.trim().slice(0, 40)),
  busyRegions: [...document.querySelectorAll('[aria-busy]')]
    .map(element => [element.id, element.getAttribute('aria-busy')]),
})
```

Require `h1Count === 1`, `trailCount === 1`, no duplicate IDs, no small control
targets, and `scrollWidth - clientWidth <= 1`. Standalone HTMX fragments require
zero trails. Intentional local scroll regions are measured separately and must
have a visible hint plus keyboard-focusable region.

The 44px rule applies to every visible interactive target, including plain
anchors, credential cards, project links, skill-detail links, and inline text
links. If a true prose citation must remain inline, give it at least 44px line
height and adequate horizontal hit area; do not exempt it silently from the
matrix.

- [ ] **Step 4: Inspect visual composition and interaction at each breakpoint**

Capture viewport and full-page screenshots. Check hero image crops, signal
trail placement, heading wraps, natural card heights, grid span resets, sticky
behavior, footer rhythm, and long content. Tab through each component family;
for every focused target confirm the complete outline is visible outside
ancestor overflow and the target's top edge is below the fixed header after
scrolling. Exercise mobile-menu focus wrapping, Escape restoration, skip link,
active navigation, and header scrolling.

Use the browser's computed accessibility/contrast inspection on normal text,
muted text, primary/secondary/danger controls, links, focus outlines, and every
feedback/status variant. Require WCAG AA: 4.5:1 for normal text, 3:1 for large
text, and 3:1 for meaningful non-text boundaries/focus indicators. Record the
measured ratio, not only `Pass`.

- [ ] **Step 5: Exercise stateful interfaces**

On Skills, test URL-backed search, filters, invalid values, rapid replacement,
no results, detail open/close focus, refresh, cached history, and cache-miss
history. Confirm `aria-busy` changes to true during requests and false after
success/error.

Open every `/__preview/soccer/{fixture}` named in Task 10, then test modal,
selection states, loading, and native download without real credentials. On
Portal, test `/mgmt`, `/mgmt?fixture=empty`,
`/mgmt?fixture=retrieval-error`, `/__preview/portal/error`, all lifecycle rows,
action feedback, and these exact fragment fixtures:

```text
/mgmt/instances/i-0f1e2d3c4b5a69788/metrics?fixture=empty
/mgmt/instances/i-0f1e2d3c4b5a69788/metrics?fixture=error
/mgmt/instances/i-0f1e2d3c4b5a69788/logs?fixture=empty
/mgmt/instances/i-0f1e2d3c4b5a69788/logs?fixture=error
```

Confirm one expanded instance never collapses another, keyboard activation
moves focus while pointer activation does not, and each target toggles
`aria-busy` true then false on success, empty, and error.

Force the Home Gravatar `src` to a known local 404, wait for `error`, and require
the fixed-size `CJ` fallback to be visible without layout shift; reload to
restore the real URL.

- [ ] **Step 6: Inspect reduced motion and forced colors**

Emulate reduced motion and confirm essential content is immediately visible,
animation delay is zero, and the trail is static. Emulate forced colors and
verify computed system colors for borders, controls, focus, statuses, tables,
modal, Portal actions, and the trail structural rule. Capture one public-page,
Soccer, and Portal screenshot in each mode.

- [ ] **Step 7: Fix every observed defect through a regression loop**

For each failure:

1. Add or extend the narrowest regression test.
2. Run it and confirm failure.
3. Fix the canonical source owner.
4. Rebuild and reload the affected route.
5. Recheck the failing viewport and one adjacent viewport.
6. Mark the evidence row `Pass` with the changed file/test reference.

Do not mark a row pass based only on source inspection.

- [ ] **Step 8: Run repository-authoritative completion checks**

Run in this order:

```sh
task generate
task fmt
task lint
task test
task build
VERIFY_COMPILED_CSS=1 go test ./internal/app \
  -run TestTailwindSourceArchitecture -count=1
node --check cmd/web/static/js/main.js
git diff --check
```

Expected: every command passes. If `task lint` hits a local toolchain
incompatibility, capture the exact output in the QA document and run `task vet`
plus all remaining checks; do not label lint as passing.

- [ ] **Step 9: Audit the final diff and acceptance criteria**

Compare the current tree against every acceptance criterion in the design spec.
Confirm each criterion has source, test, runtime, or visual evidence. Confirm no
generated files are staged, no pre-existing unrelated work was removed, and the
three old Emberglass override files are absent.

- [ ] **Step 10: Record the final checkpoint**

If source hunks can be staged without sweeping in pre-existing work, use:

```sh
git commit -m "feat: complete Emberglass Field Station 2.0"
```

Otherwise leave source changes unstaged, commit only the new QA evidence when
appropriate, and report the exact dirty-file scope.

Stop the managed preview server and confirm `lsof` shows no listener on 8182.
