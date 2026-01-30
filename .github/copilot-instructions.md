# Portfolio Codebase Instructions

## Big picture

- Go 1.21+ server-rendered site with HTMX for partial updates; all app code lives in `main.go`.
- Templates are split into layout/pages/partials; pages render through `renderPage`, fragments through `renderFragment`.

## Core structure and patterns

- Templates:
  - `templates/layouts/base.html` defines `{{ block "content" }}` and loads page CSS via `.Page`.
  - `templates/pages/*.html` implement `{{ define "content" }}` for each route.
  - `templates/partials/*.html` for shared chrome and HTMX fragments.
- Handlers live in `main.go` and follow:
  - `renderPage(w, "pagename", map[string]any{"Title": "...", "Page": "pagename", ...})`
  - Fragments use paired routes, e.g. `/experience` + `/experience/timeline` -> `experience_timeline.html`.
- Data is hardcoded in `main.go` via typed structs and `*_Data()` helpers (e.g., `experienceData()`, `skillsData()`).

## HTMX integration

- Fragment routes return only partial templates; see `experience_timeline.html`, `skills_grid.html`, `projects_grid.html`.
- Soccer page shows full HTMX form flow: `POST /soccer/fetch` returns `soccer_table_fragment.html` and `POST /soccer/download` streams ICS.

## Frontend conventions

- Per-page CSS in `static/css/{pagename}.css`; globals and theme tokens in `static/css/styles.css`.
- Theme persistence in `static/js/theme.js`; base interactions in `static/js/main.js`.
- Active nav uses `{{ if eq .Page "pagename" }}active{{ end }}` in `templates/partials/nav.html`.

## Dev workflow

- Build/run: `go build -o portfolio-server . && ./portfolio-server` (server at http://localhost:8080).
- Templates are loaded once at startup; restart required unless using `air` hot-reload.

## When adding a page

1. Add handler and data in `main.go`; register route in `main()`.
2. Create `templates/pages/{pagename}.html` and `static/css/{pagename}.css`.
3. Add nav entry in `templates/partials/nav.html`.
4. If HTMX fragments are needed, add partial + fragment handler.
