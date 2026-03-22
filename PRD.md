# Feature: Soccer Page UX Overhaul

## Overview

Redesign the soccer schedule tool page to reorder its input methods by usage likelihood, add progress/loading indicators for all async operations, and elevate the visual design to be the standout page of the portfolio site.

## Success Criteria

- [ ] All tasks complete
- [ ] All tests passing (`just test`)
- [ ] Build succeeds (`just build`)
- [ ] Lint passes (`just lint`)
- [ ] No regressions to existing soccer auth, fetch, download, or Google Calendar flows
- [ ] No blockers

## Tasks

### Task-001: Reorder Input Sections — Manual Team IDs First

**Priority**: High
**Estimated Iterations**: 1-2

The "Fetch Schedules" unified card currently renders three sections in this order:

1. JWT import (soccer-auth-shell with HTMX-loaded soccer-auth-panel)
2. Google Calendar connection (inside soccer-auth-panel via `SoccerLoginState`)
3. Manual team IDs (soccer-manual-entry with fetch form)

Swap the order so the manual team ID entry appears **first**, Google Calendar stays in the **middle**, and JWT import moves **last**.

**Files to modify**:

- `components/pages/soccer.templ` — reorder the DOM structure inside `.unified-form-section`
- `components/partials/soccer_login_state.templ` — no logic changes needed, but confirm Google Calendar section remains between the new ordering boundaries
- `static/css/soccer.css` — adjust any CSS assumptions tied to ordering (border-top on `.soccer-manual-entry`, spacing)

**Acceptance Criteria**:

- [ ] Manual team ID form appears first inside the unified card's left column
- [ ] Google Calendar connection/selection appears second (middle position)
- [ ] JWT import section (soccer-auth-shell) appears last
- [ ] "How to Import Access" sidebar instructions remain on the right
- [ ] Each section has clear visual separators (borders or spacing) so they read as distinct options
- [ ] Existing HTMX swap targets (`#soccer-auth-panel`, `#games-container`, `#loading-indicator`) remain functional
- [ ] All existing tests pass without modification

**Verification**:

```bash
just generate && just test && just build
```

---

### Task-002: Add Loading/Progress Indicators to Async Operations

**Priority**: High
**Estimated Iterations**: 2-3

Currently, clicking "Import access" in the JWT modal and "Add Selected to Google Calendar" provides no visual feedback while the request is in-flight. Users cannot tell whether their click registered.

Add clear loading/progress states to every async action on the page:

1. **JWT Import modal** — show a spinner and disable the "Import access" button while the HTMX POST to `/soccer/import` is in flight
2. **Google Calendar "Add Selected"** — show a spinner and disable the button while the HTMX POST to `/soccer/google/add` is in flight
3. **Google Calendar "Save calendar"** — show a spinner on submission
4. **Google Calendar "Connect" / "Reconnect" / "Disconnect"** — disable on click so double-clicks are prevented
5. **"Fetch Player Schedules"** (player select form) — show a spinner while fetching
6. **"Clear import" / Logout** — brief disabled state while clearing

Use HTMX's built-in `hx-indicator` and `hx-disabled-elt` attributes where possible. For the JWT modal, add an `htmx:beforeRequest` / `htmx:afterRequest` listener in `main.js` to toggle a spinner inside the submit button and disable it.

**Files to modify**:

- `components/partials/soccer_login_modal.templ` — add `hx-indicator` and `hx-disabled-elt` attributes to the import form/button
- `components/partials/soccer_login_state.templ` — add `hx-disabled-elt` to Clear import, Disconnect, Save calendar, Reconnect buttons
- `components/partials/soccer_player_select.templ` — add `hx-indicator` / `hx-disabled-elt` to "Fetch Player Schedules"
- `components/partials/soccer_table_fragment.templ` — add `hx-indicator` / `hx-disabled-elt` to "Add Selected to Google Calendar"
- `static/css/soccer.css` — add spinner styles for buttons (`.btn-loading` class with inline spinner)
- `static/js/main.js` — add HTMX event listeners for the JWT modal form to toggle loading state

**Acceptance Criteria**:

- [ ] Clicking "Import access" in the modal shows a spinner inside the button and disables it until the response arrives
- [ ] Clicking "Add Selected to Google Calendar" shows a spinner inside the button and disables it until the response arrives
- [ ] "Save calendar", "Clear import", "Disconnect", "Reconnect Google", and "Fetch Player Schedules" show a disabled/loading state while their respective requests are in flight
- [ ] Double-clicking any button does not fire duplicate requests
- [ ] Loading states automatically clear on both success and error responses
- [ ] Spinner is accessible (uses `aria-busy` or similar)
- [ ] All existing tests still pass

**Verification**:

```bash
just generate && just test && just build
```

Manual test: Open the soccer page, trigger each action, and confirm spinner/disabled state appears and clears.

---

### Task-003: Visual Design Polish — Make the Soccer Page Stand Out

**Priority**: Medium
**Estimated Iterations**: 3-5

After Tasks 001 and 002 are complete, review the full soccer page and polish it to be the most visually striking page on the site. It should feel premium and draw visitors in while remaining functional and minimal.

**Design direction**:

- Maintain the existing dark theme with warm orange (`--accent-primary`) and teal (`--accent-secondary`) accents
- Introduce a secondary accent glow / gradient flair that gives the page its own identity (e.g., a subtle green-to-teal pitch gradient in the hero background overlay, or a neon pitch-line accent on cards)
- Ensure the three reordered input sections in the unified card are visually distinct and easy to scan — consider numbered step badges, subtle color-coded left borders, or icon badges for each option
- Polish the "How It Works" and "Built For Players" card sections for consistency with other pages
- Improve the empty state in the games container — add an icon or illustration placeholder
- Ensure the schedule table, when loaded, looks polished with subtle row highlights and clear action buttons
- Review spacing, font sizing, and padding for a balanced, airy feel that matches the rest of the portfolio

**Files to modify**:

- `static/css/soccer.css` — primary file for all style updates
- `components/pages/soccer.templ` — minor markup tweaks for any new visual elements (section badges, icons, decorative elements)
- `components/partials/soccer_login_state.templ` — any markup tweaks for visual improvements
- `components/partials/soccer_table_fragment.templ` — table styling improvements if markup changes are needed

**Acceptance Criteria**:

- [ ] The three input sections (Manual Team ID, Google Calendar, JWT Import) are visually distinct with clear headers and subtle differentiation (e.g., numbered badges, colour-coded left borders, or distinct icons)
- [ ] Hero section has a unique visual flair that distinguishes it from other portfolio pages (e.g., pitch-inspired gradient, soccer-themed accent)
- [ ] "How It Works" steps update to reflect the reordered flow (Manual Team ID first)
- [ ] Empty state in the games container is visually appealing (icon + descriptive text)
- [ ] Schedule table rows have clear hover states and the action bar is well-styled
- [ ] All card sections (How It Works, Features, Unified Card) have consistent border radius, shadow, spacing
- [ ] Page reads as cohesive, polished, and premium on both desktop and mobile
- [ ] Color contrast meets WCAG AA (4.5:1 for text, 3:1 for interactive elements)
- [ ] No visual regressions on other pages
- [ ] Responsive layout still works correctly at 768px and 1024px breakpoints

**Verification**:

```bash
just generate && just build
```

Manual test: View on desktop and mobile viewport; compare visual consistency with home, projects, and skills pages.

---

## Technical Constraints

- Language: Go 1.26.1
- Templating: Templ (edit `.templ` files, run `just generate`)
- Frontend: HTMX + vanilla JS (no build step, no bundler)
- Styling: Plain CSS with CSS custom properties (no preprocessor)
- Testing: `go test` via `just test`
- Linting: `just lint` (golangci-lint + stylelint)
- Formatting: `just fmt`

## Architecture Notes

- All changes are UI/UX focused — no new Go handlers or API routes required
- HTMX swap targets (`#soccer-auth-panel`, `#games-container`, `#loading-indicator`, `#soccer-login-feedback`, `#google-calendar-feedback`) must remain stable since handlers render HTML fragments targeting these IDs
- The `SoccerLoginState` component is loaded via `hx-get="/soccer/session"` on page load and swapped via `hx-swap-oob` after import/logout — reordering in `soccer.templ` must keep the `#soccer-auth-panel` div functional
- `main.js` handles modal open/close and select-all logic — new loading state JS should follow the same IIFE pattern

## Out of Scope

- Changing Go handler logic or API routes
- Adding new backend features (email subscription is already stubbed)
- Modifying other portfolio pages' styles
- Changing the chrome extension
- Introducing a CSS preprocessor or JS bundler
