---
description: "Use when writing or editing Templ templates (.templ files). Covers the edit/generate workflow, full-page vs fragment patterns, and shared component references for this codebase."
applyTo: "**/*.templ"
---

# Templ Authoring Rules

## Files

- **Edit `.templ` source files only.** The sibling `*_templ.go` files are generated output — never modify them directly.
- Run `task generate` after any `.templ` change. (`task build` also runs this.)
- The `air` hot-reload loop (`task dev`) does **not** regenerate Templ output; run `task generate` separately when working with hot reload.

## Full-page vs fragment pattern

| Use case         | Pattern                                                        |
| ---------------- | -------------------------------------------------------------- |
| Full page load   | Render layout wrapper (`base.templ`) around the page component |
| HTMX swap target | Return the partial HTML fragment only — **no layout wrapper**  |

- `cmd/web/layouts/base.templ` — the layout wrapper for full pages; includes asset links.
- `cmd/web/pages/soccer.templ` — exemplar of a full page mixing static content with HTMX swap targets.
- `cmd/web/partials/soccer_login_state.templ` and `soccer_table_fragment.templ` — exemplars of HTMX fragment returns.

## Shared UI components

`cmd/web/partials/ui.templ` contains reusable partial components:

- `PageHero` / `PageHeroIntro` — hero section with title and description
- `PageCTA` — call-to-action section at page bottom

Reuse these before adding inline markup.

## Styling

- Use CSS component classes defined in `cmd/web/tailwind/components.css` and `cmd/web/tailwind/soccer.css`.
- Tailwind utility classes are available but prefer named component classes for consistency.
- For page structure, wrap content in `.page-kit-page` to apply the correct color scheme.
- Common layout helpers: `.page-section`, `.page-section-tight`, `.page-hero-title`, `.page-hero-description`.

## No business logic

Templates are display-only. All data preparation and conditional logic belongs in handler functions or view model files (e.g., `experience_viewmodels.go`), not in `.templ` files.
