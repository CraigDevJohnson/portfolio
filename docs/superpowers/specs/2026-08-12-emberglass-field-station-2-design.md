# Emberglass Field Station 2.0 Design Specification

**Status:** Approved design direction  
**Approved:** 2026-08-12  
**Scope:** Portfolio pages, Soccer states, and the management portal

## Purpose

This specification defines the second Emberglass iteration of Craig Johnson's
portfolio. The site presents a principal cloud engineer to hiring managers,
technical peers, and potential collaborators. Its central job is to make Craig's
systems thinking tangible: visitors should quickly understand the person, the
career progression, the technical depth, the proof of work, and the usefulness
of the tools he builds.

The redesign must feel warm, inviting, and vibrant while retaining a dark,
pastel-led identity. Each route gets an information structure suited to its
content, but all routes must unmistakably belong to one site.

## Goals

1. Preserve the established dark warm-pastel Emberglass identity while making
   it more disciplined and distinctive.
2. Give each route a layout that reflects the route's actual content and job.
3. Remove CSS cascade conflicts, contradictory breakpoints, and hidden legacy
   theme behavior.
4. Preserve all existing content and functional behavior unless a small copy
   clarification is needed for usability or an error state.
5. Strengthen Go view models, typed Templ components, HTMX progressive
   enhancement, and Tailwind source organization.
6. Ensure navigation, controls, cards, tables, images, focus rings, and overlays
   do not overlap, clip, or create unintended page-level horizontal scrolling.
7. Verify the result visually across every route and important dynamic state.

## Non-goals

- Replacing the site's real content with marketing copy.
- Turning the portfolio into a single-page application.
- Introducing a JavaScript framework or client-side router.
- Redesigning backend integrations or changing Soccer, Google Calendar,
  Cognito, EC2, CloudWatch, or download semantics.
- Adding decorative motion to every component.
- Making the public portfolio and management portal visually identical in
  density; they share a system, not the same layout.

## Direction considered

Three approaches were considered:

1. **Emberglass Field Station 2.0:** retain the dark mulberry field-station
   identity, strengthen page-specific compositions, and consolidate the CSS.
2. **Daybreak Field Notes:** move to a light oat-colored editorial canvas.
3. **Two-Climate System:** use light public pages and dark operational tools.

The approved first approach preserves the existing identity and the previously
chosen dark-mode direction. Daybreak was rejected because a cream editorial
portfolio with warm accents is a common template default. Two-Climate was
rejected because it would make the site feel divided precisely where the brief
requires cohesion.

## Visual system

### Palette

The implementation derives every page and component color from these six core
colors and their semantic mixes:

| Token | Hex | Role |
| --- | --- | --- |
| Night Mulberry | `#17121B` | Main canvas and deep overlays |
| Cocoa Cedar | `#2E2130` | Panels, navigation, and raised surfaces |
| Candle Oat | `#FFF0D8` | Primary copy and high-contrast detail |
| Campfire Apricot | `#FFA677` | Primary actions and warm route signals |
| Rosehip | `#FF7FA8` | Active states, emphasis, and error-adjacent accents |
| Pond Mint | `#78E3C3` | Links, success, and operational signals |

Pollen/gold effects are produced as a semantic mix of Candle Oat and Campfire
Apricot rather than maintained as a seventh independent foundation.
Status colors must use accessible semantic tokens rather than repurposing a
brand color without regard to meaning.

### Typography

- **Bricolage Grotesque:** display headings and the strongest page thesis.
- **Atkinson Hyperlegible:** body copy, navigation, controls, and forms.
- **IBM Plex Mono:** captions, dates, statistics, status labels, and technical
  data.

Display typography should be memorable through scale, line breaks, and width,
not through repeated gradients or effects. Body copy should generally stay
between 45 and 72 characters per line.

### Spacing, shape, and depth

- Use one documented spacing scale and four composition breakpoints.
- Use generous rounded photographic portals for heroes and smaller, quieter
  radii for data-dense controls.
- Keep shadows soft and dark; use borders and tonal contrast before blur.
- Preserve minimum 44 by 44 CSS-pixel pointer targets.
- Avoid arbitrary per-template pixel values when a design token can express
  the same decision.

### Signature: the signal trail

A single luminous trail connects the characteristic content on each route.
It represents a system moving from input to useful outcome. It is not a generic
wave decoration: its placement should follow the page's actual structure, such
as career progression, filter-to-result flow, or schedule-to-calendar flow.

Each page uses the trail once. Other decorations remain quiet so the signature
retains impact. Under reduced motion the trail is static. Under forced colors
it becomes a simple structural rule.

### Motion

- Use one short, coordinated arrival sequence for the hero and signal trail.
- Use motion for state changes, HTMX swaps, disclosure, and focus restoration.
- Avoid independent card entrance effects unless order communicates something.
- Reset animation duration **and delay** for reduced-motion users.
- Never begin essential content at zero opacity when JavaScript or animation
  support is uncertain.

## Layout concepts

### Shared shell

```text
+---------------------------------------------------------------+
| identity                         route navigation / menu       |
+---------------------------------------------------------------+
| route-specific hero thesis                                    |
|                                                               |
| route-owned information structure                             |
|                                                               |
| contextual next step                                          |
+---------------------------------------------------------------+
| compact shared footer                                         |
+---------------------------------------------------------------+
```

The header remains a floating field-station dock. Desktop navigation appears
only where every item fits; the mobile menu owns smaller widths and traps focus
without making the underlying page interactive. The footer groups destinations
explicitly rather than splitting the navigation list by array position.

### Home: systems overlook

**Job:** establish identity and make the portfolio's systems-oriented point of
view clear in one screen.

```text
+----------------------------+-------------------------------+
| availability + identity    | panoramic build environment   |
| thesis + primary action    | portrait as a small witness   |
| compact credibility proof  |                               |
+----------------------------+-------------------------------+
| capability topology: cloud -> delivery -> operations       |
+------------------------------------------------------------+
| varied proof-of-work briefs, not four identical cards       |
+------------------------------------------------------------+
```

The hero remains the strongest visual risk: an asymmetrical photographic portal
occupies roughly half the wide composition and shifts above the copy on smaller
screens. Statistics support the thesis but do not become the hero's main idea.

### About: Alaska switchback

**Job:** connect Craig's personal path, working style, and values to the technical
career without repeating the resume.

The main narrative follows a switchback path. A compact facts pack can remain
sticky on wide screens but must rejoin document flow on smaller screens. Story,
timeline, hobbies, and values use different compositions so the page does not
become a continuous card grid.

### Experience: three eras

**Job:** show career progression, expanding responsibility, and durable themes.

The three eras are a real sequence, so explicit era markers are meaningful. A
horizontal progression rail is permitted on wide screens; mobile uses a single
vertical rail. Role details remain readable without forced equal heights.
Hero and summary statistics use the shared statistic component with responsive
one-, two-, and three-column variants.

### Skills: working toolkit

**Job:** let visitors inspect breadth and depth without scanning an undifferentiated
logo wall.

Featured capabilities form a deliberate mosaic, operating practices form a
smaller supporting band, and the full catalog is a searchable/filterable
workbench. Filters remain understandable without HTMX, preserve their state in
the URL, expose loading/result state, and restore focus predictably after detail
disclosure.

### Projects: case-study dossiers

**Job:** prove that Craig ships useful software and infrastructure.

The leading project receives a wide dossier with problem, approach, outcome,
technology, and destination. Remaining projects use smaller asymmetric briefs
whose visual hierarchy is determined by real metadata, not filename checks.
Project links consistently identify external destinations.

### Education: learning field guide

**Job:** show sustained learning and credential depth without presenting a badge
wall.

The degree is a single foundational feature. Certifications are grouped by
learning domain through explicit data, with varied group sizes allowed to shape
the layout. Headings preserve a valid hierarchy. Credential imagery stays
secondary to the qualification name and provider.

### Contact: correspondence window

**Job:** make starting the right kind of conversation immediate.

Contact channels sit beside a concise availability ticket and expertise ribbon.
The primary email action is visually dominant; external profiles are secondary.
On mobile the availability ticket follows the introduction rather than becoming
a detached sticky element.

### Soccer: matchday planner

**Job:** turn league schedule data into clear, selectable, calendar-ready games.

The page presents a visible workflow: connect or choose a team, review games,
select matches, then export or add them. Authentication, manual fallback,
selection, loading, empty, error, success, Google Calendar, and expired-session
states share one feedback vocabulary. Wide game tables retain an explicit scroll
hint and keyboard-focusable scroll region.

### Management portal: operator workspace

**Job:** safely inspect and act on instances while keeping status, feedback, and
details legible.

The portal uses the same colors, typography, controls, and signal language but a
denser application shell. Wide screens use a table with expandable metrics and
logs. Narrow screens use instance cards rather than compressing the full table.
Preview, empty, retrieval-error, pending, running, stopped, shutting-down, and
terminated states all receive explicit styles.

## Target source architecture

### CSS ownership

The final cascade must have one owner for every rule:

- `theme.css`: core palette, semantic colors, type families, breakpoints, and
  container sizes.
- `shared.css`: spacing, radii, typography scale, timing, motion primitives,
  and reusable low-level properties.
- `base.css`: element defaults, body/page semantics, selection, focus, forced
  colors, and reduced-motion behavior.
- `components.css`: shared shell and reusable UI components only.
- `pages/home.css`, `pages/about.css`, `pages/experience.css`,
  `pages/skills.css`, `pages/projects.css`, `pages/education.css`, and
  `pages/contact.css`: route-scoped layout rules that do not restyle shared
  component internals.
- `soccer.css` and `portal.css`: stateful tool-specific components.

`app.css` imports the sources in an explicit layer order. The current
Emberglass override files are migration inputs, not a permanent layer above a
complete older theme. Once their surviving declarations have a single home,
the override files and compatibility aliases are removed.

Use four shared composition thresholds: compact phone at `30rem`, tablet at
`48rem`, navigation/dense layout at `70rem`, and wide canvas at `80rem`. Use
container queries where a reusable component should respond to its own width.
Do not mix several near-equivalent raw pixel and rem breakpoints for the same
transition.

### Templ components

The `partials` package remains the shared presentation layer, but the current
large UI source is split by responsibility. Typed props and constants replace
stringly typed variants and positional argument lists.

Shared components include:

- page hero and photographic portal;
- section introduction;
- action link and action button;
- statistic grid and statistic item;
- feedback/status message;
- horizontal overflow region and hint;
- route-aware next-step block.

Page templates own their semantic hierarchy and route-specific composition.
View-model preparation and state predicates live in Go files, not in `.templ`
markup. Templ source remains the only edited template source; generated files
are regenerated through the Taskfile.

### Go view models

- Centralize Soccer capability predicates and security-warning content so the
  full page and HTMX fragments cannot drift.
- Encode navigation/footer groups explicitly.
- Encode credential groups and project presentation metadata explicitly rather
  than branching on titles, providers, or filenames in templates.
- Keep component variants closed and typed so invalid combinations fail during
  compilation or tests.

## HTMX and interaction design

### Skills filtering

1. A normal GET URL represents category and proficiency state.
2. Without HTMX, the server renders the complete Skills page for that URL.
3. With HTMX, the server renders only the filterable catalog fragment.
4. The enhanced control updates the fragment, active state, result count, and
   browser URL.
5. Rapid requests synchronize so stale responses cannot replace newer results.
6. Skill detail disclosure restores focus to the triggering control when closed.

### Soccer

Existing fragment IDs, data attributes, OOB swaps, native download behavior,
and trigger contracts remain stable unless updated together with their handlers
and tests. Every swap must preserve selection controls, loading state, focus,
and actionable feedback.

### Portal

The `X-Portal-Fragment-Error` contract remains intact so meaningful non-2xx
responses can still render inside the page. Metrics and logs must focus or
announce newly loaded detail without collapsing another instance's state.

### Progressive enhancement

Primary navigation, project/contact links, Skills filter URLs, Soccer native
downloads, and core portal navigation must remain useful if HTMX or the shared
JavaScript file fails to load. JavaScript enhances focus, selection, and motion;
it does not supply essential content.

## Error, empty, and loading states

All stateful interfaces use the same feedback anatomy:

1. Meaningful status icon or label.
2. Plain-language summary.
3. Specific cause or current state when known.
4. One direct next action when recovery is possible.

Errors do not apologize or use vague language. Empty states explain what input
or action will populate the area. Loading indicators retain the final region's
minimum useful size and expose `aria-busy`. Success messages use the same action
name as the control that produced them.

## Accessibility requirements

- One visible `h1` per page and a sequential heading hierarchy.
- A skip link that appears above the fixed header when focused.
- Visible keyboard focus that is not clipped by hero or card overflow.
- Menu and modal focus containment with Escape restoration.
- Correct active/current/filter/disclosure semantics.
- No essential meaning conveyed by color alone.
- Contrast suitable for normal copy, muted copy, controls, and statuses.
- Reduced-motion and forced-color behavior for all new components.
- External-link treatment conveyed accessibly without noisy repeated copy.
- Horizontal data regions focusable and paired with visible guidance.

## Responsive requirements

The implementation must be reviewed at minimum at these viewport widths:

- 390 pixels: narrow phone composition.
- 768 pixels: tablet and multi-column transition.
- 1119 pixels: immediately below desktop navigation.
- 1121 pixels: immediately above desktop navigation.
- 1440 pixels: wide composition.

At each size:

- `document.documentElement.scrollWidth` must not exceed the layout viewport,
  except inside intentional local scroll regions.
- Fixed navigation must not cover focused or anchored content.
- Text and actions must remain inside their containers.
- Hero images and portrait treatments must not clip focus rings or captions.
- Grid items must reset spans when column definitions change.
- Long labels, status values, IDs, and URLs must wrap or truncate intentionally.
- Tables must scroll locally or switch to a mobile card representation.

## Verification matrix

### Routes

Review `/`, `/about`, `/experience`, `/skills`, `/projects`, `/education`,
`/contact`, `/soccer`, `/mgmt`, and the portal error page.

### Shared states

- Desktop and mobile navigation, including open menu and keyboard traversal.
- Fixed header before and after scrolling.
- Focused skip link.
- Remote-image success and fallback behavior.
- Reduced motion and forced colors.

### Skills states

- Default, category-filtered, proficiency-filtered, combined, and no-result.
- Rapid filter changes.
- Detail open, detail close, and restored focus.
- Direct navigation and back/forward history with active URL filters.

### Soccer states

- Authentication disabled/manual-only.
- Unauthenticated import flow and modal.
- Invalid, expired, rejected, and upstream token errors.
- Imported players, no players, team selection, and manual entry.
- No games, upcoming games, past results, and combined results.
- None, partial, and all selection states.
- Loading, native ICS download, Google disconnected/connected, add/sync success,
  failure, and expired-session reset.

### Portal states

- Preview dashboard and harmless action feedback.
- Running, stopped, pending, shutting-down, and terminated instances.
- Metrics and logs loaded, empty, and failed.
- Empty instance list, retrieval error, invalid ID, and portal error page.
- Wide table and narrow instance-card compositions.

### Commands

Run the repository-authoritative checks after implementation:

```sh
task generate
task fmt
task lint
task test
task build
git diff --check
node --check cmd/web/static/js/main.js
```

If local linter/toolchain incompatibility prevents `task lint`, capture the exact
failure, run the remaining checks, and do not represent lint as passing.

## Acceptance criteria

The redesign is complete only when all of the following are true:

1. Every in-scope route uses the approved palette, typography, control language,
   and restrained signal-trail signature.
2. Home, About, Experience, Skills, Projects, Education, Contact, Soccer, and
   Portal have visibly different information structures appropriate to their
   content.
3. Shared shell, actions, statistics, status messages, focus treatment, and
   spacing remain consistent across routes.
4. The final CSS has no complete legacy theme overridden by another complete
   theme, and breakpoint ownership is explicit.
5. Go prepares presentation state, Templ owns semantic markup, HTMX enhances
   server-rendered fragments, and Tailwind/CSS owns the visual system.
6. Existing portfolio, Soccer, Calendar, portal, download, navigation, and
   external-link behavior remains functional.
7. Required responsive viewports and dynamic states have been visually reviewed
   without unintended overlap, clipping, cutoff, or page-level overflow.
8. Keyboard focus, reduced motion, forced colors, error states, empty states,
   and loading states have been inspected.
9. Repository checks pass, or any environment-specific blocked check is reported
   accurately with all remaining checks passing.

## Risks and mitigations

- **Dirty working tree:** treat the current tree as the source of truth; patch
  narrowly and review diffs by file before every migration step.
- **Cascade regression:** migrate one component family at a time, remove its old
  rules only after browser comparison, and use route-scoped composition names.
- **HTMX selector drift:** preserve or atomically update IDs/data attributes,
  handlers, JavaScript, and tests.
- **Responsive implicit tracks:** explicitly reset grid spans whenever a column
  definition changes and measure scroll width at every required viewport.
- **External asset variability:** provide stable image dimensions and fallbacks,
  then verify both loaded and failed states.
- **Portal state coverage:** extend preview fixtures or render tests so uncommon
  operational states can be reviewed without AWS side effects.
