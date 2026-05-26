---
description: "Use when editing Tailwind CSS source files or adding new component styles. Covers the CSS-first architecture, source file roles, design token conventions, and build commands for this portfolio."
applyTo: "cmd/web/tailwind/**"
---

# Tailwind CSS Authoring Rules

## ⚠ Never edit the generated output

`cmd/web/static/css/tailwind.css` is compiled output. **Do not edit it.**  
Edit the source files under `cmd/web/tailwind/` and run `task tailwind-build`.

## Source file roles

| File             | Purpose                                                                       |
| ---------------- | ----------------------------------------------------------------------------- |
| `app.css`        | Entry point — imports all other files in order; also declares `@source` globs |
| `theme.css`      | Color palette tokens and breakpoints                                          |
| `shared.css`     | Spacing, radius, typography, timing, and `--header-height` custom properties  |
| `base.css`       | Base element styles and `:root` semantic aliases                              |
| `components.css` | All shared component classes (layout, buttons, cards, forms, skills, etc.)    |
| `soccer.css`     | Soccer-page-specific components (login modal, hero overlay, games section)    |

## Design tokens

Always use tokens from `theme.css` and `shared.css` instead of raw values.

- **Colors**: `--color-primary-*`, `--color-secondary-*`, `--color-accent-*`, `--color-carbon-*`, and semantic aliases such as `--color-copy-primary`, `--color-copy-secondary`, `--color-canvas`, `--color-panel`.
- **Spacing**: `--space-*` (e.g., `--space-4`, `--space-8`).
- **Radius**: `--radius-*` (e.g., `--radius-sm`, `--radius-xl`).
- **Typography**: `--font-*`, `--leading-*`.
- **Timing**: `--duration-*`, `--ease-*`.

Do not introduce ad-hoc hex values or raw pixel sizes.

## Adding new component styles

1. Add component classes to `components.css` for shared components or `soccer.css` for soccer-page-specific styles.
2. Follow the existing `@apply` pattern for component classes.
3. Rebuild with `task tailwind-build` (or `task tailwind-watch` during development).

## Page structure

All portfolio content pages should be wrapped in `.page-kit-page` to apply the correct color scheme.

Common layout classes (defined in `components.css`):

- `.page-section` / `.page-section-tight` — section vertical spacing
- `.page-hero-title` / `.page-hero-description` — hero typography
- `.page-section-title` / `.page-section-subtitle` — section headings
