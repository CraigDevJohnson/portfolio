# Experience theme rollout plan

## Objective

Apply the same color theme, component styling, and overall visual feel used by the
refactored `experience` page to the rest of the portfolio while preserving each
page's content and behavior.

## Reference implementation

- `cmd/web/pages/experience.templ`
- `cmd/web/partials/experience_kit_stages.templ`
- `cmd/web/partials/experience_shared.templ`
- `cmd/web/tailwind/components.css`
- `cmd/web/tailwind/theme.css`

## Implementation strategy

1. Extract the reusable styling patterns from Experience into a shared page kit.
2. Update shared partials and shared Tailwind component classes so most pages can
   inherit the new look with minimal template churn.
3. Migrate the remaining page templates and specialized partials in waves.
4. Validate generated Templ output and build correctness after each wave.

## Phase 1 — Shared page kit foundation

- Add reusable page-kit tokens and classes based on the Experience shell, panels,
  typography, chips, actions, and CTA patterns.
- Keep Experience as the visual source of truth while introducing generic class
  names that other pages can use.
- Update shared UI partials such as section labels and page CTAs so the new visual
  system propagates automatically.
- Scope the new theme to the portfolio content pages so header/footer behavior is
  not accidentally altered.

## Phase 2 — Migrate pages

### Home

- Rework the hero to use the shared page-kit shell/backdrop treatment.
- Align hero status, buttons, and stat cards with Experience-style panel depth and
  accent usage.

### About

- Move the hero, stats, story layout, quick facts, timeline, and supporting cards
  onto the shared page-kit surface hierarchy.

### Contact

- Apply the shared hero, action-card, and CTA styling.
- Keep clear interaction affordances and accessible labels for external links.

### Education

- Restyle the hero, stats, featured degree, and certification cards using the same
  shell/panel hierarchy.

### Projects

- Align the hero, stats, filter controls, project grid cards, and footer CTA with
  the shared page-kit system.

### Skills

- Replace inline gradient wrappers in loading states and loaded HTMX content with
  shared page-kit containers.
- Preserve the current filtering behavior and progressive loading.

### Soccer

- Harmonize the hero and surrounding layout with the new shared styling while
  preserving the specialized tool interactions and table readability.

## Cross-cutting rules

- Use `cmd/web/tailwind/theme.css` as the source of truth for shared theme tokens.
- Favor shared component classes in `cmd/web/tailwind/components.css` over page-
  local inline gradient strings when possible.
- Keep the server-rendered and HTMX-first architecture intact.
- Preserve accessibility: visible focus, keyboard support, readable contrast,
  forced-colors support, and reflow at narrow widths.
- Edit `.templ` source files only, then regenerate generated output.

## Validation

- Run Templ generation after template changes.
- Run a full build after each migration wave.
- Verify page headings, CTA states, focus styling, and card hierarchy.
- Smoke test specialized interactions on Skills and Soccer after their migration.

## Initial execution order

1. Shared page kit foundation
2. Shared UI partial refactor
3. Home migration
4. About, Contact, Education, and Projects migration
5. Skills migration
6. Soccer harmonization
7. Final polish and validation
