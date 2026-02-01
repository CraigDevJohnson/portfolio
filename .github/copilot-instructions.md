# Portfolio Codebase Instructions

## Architecture overview

Go 1.21+ server-rendered portfolio with HTMX for dynamic updates. All application code lives in a single `main.go` file—handlers, data structs, and template loading are collocated intentionally for simplicity.

## Template system

Three-tier template hierarchy loaded once at startup in `loadTemplates()`:

- **Layout**: `templates/layouts/base.html` — defines `{{ block "content" . }}`, loads per-page CSS via `.Page`
- **Pages**: `templates/pages/*.html` — each implements `{{ define "content" }}`
- **Partials**: `templates/partials/*.html` — shared components (nav, header, footer) and HTMX fragments

Custom template functions in `templateFuncs`:

- `Year` — current year for footer copyright
- `multiply`, `subtract`, `mod` — arithmetic for animation delays and grid layouts
- `slice` — create int slices in templates
- `hasPrefix` — string prefix checks

## Handler patterns

Full page render:

```go
renderPage(w, "pagename", map[string]any{
    "Title": "Page Title - Craig Johnson",
    "Page":  "pagename",  // Required: enables per-page CSS and active nav
    // ... page-specific data
})
```

HTMX fragment render (no layout wrapper):

```go
renderFragment(w, "pagename", "fragment_name.html", data)
```

## Data layer

Content is hardcoded via typed structs and factory functions:

- `experienceData()` → `[]Experience`
- `skillsData()` → `[]SkillCategory`
- `projectsData()` → `[]Project`
- `educationData()` → `[]Education`

Each struct includes all fields needed for template rendering (no database queries).

## HTMX integration patterns

Fragment routes return partial HTML for swap targets:

- `/experience` (full page) + `/experience/timeline` (fragment) → `experience_timeline.html`
- `/skills` + `/skills/grid` → `skills_grid.html`
- `/projects` + `/projects/grid` → `projects_grid.html`

Soccer page demonstrates form-driven HTMX:

- `POST /soccer/fetch` → returns `soccer_table_fragment.html`
- `POST /soccer/download` → streams ICS file attachment

## Frontend conventions

- Per-page CSS: `static/css/{pagename}.css` — linked automatically via `.Page` in base layout
- Global styles: `static/css/styles.css` — CSS custom properties for theming
- Active nav highlighting: `{{ if eq .Page "pagename" }}active{{ end }}` in `nav.html`

## Dev workflow

```bash
make run        # Build and run (http://localhost:8080)
make dev        # Hot-reload with air (requires: go install github.com/air-verse/air@latest)
make lint       # Run vet + staticcheck
make fmt        # Format source files
```

Templates load at startup — restart server or use `make dev` for template changes.

## Adding a new page

1. **Handler + data**: Add handler function in `main.go`; create data struct/factory if needed
2. **Route**: Register in `main()` routes section
3. **Templates**: Create `templates/pages/{pagename}.html` with `{{ define "content" }}`
4. **Styles**: Create `static/css/{pagename}.css`
5. **Navigation**: Add link in `templates/partials/nav.html`
6. **HTMX fragments** (optional): Add `templates/partials/{pagename}_fragment.html` + fragment handler route
