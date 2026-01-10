# Portfolio Codebase Instructions

## Architecture Overview

This is a **Go + HTMX** personal portfolio site using server-side rendering with dynamic client interactions. Single-file architecture: all application code lives in [main.go](../main.go).

### Core Patterns

- **Template System**: Go `html/template` with a composable structure:

  - `templates/layouts/base.html` — master layout (defines `{{ block "content" }}`)
  - `templates/pages/*.html` — page-specific content (implements `{{ define "content" }}`)
  - `templates/partials/*.html` — reusable fragments (header, nav, footer, HTMX fragments)

- **Handler Pattern**: Each page has a handler in `main.go` following this structure:

  ```go
  func pageHandler(w http.ResponseWriter, r *http.Request) {
      renderPage(w, "pagename", map[string]any{
          "Title": "Page Title - Craig Johnson",
          "Page":  "pagename",  // Used for CSS loading and nav highlighting
          // page-specific data...
      })
  }
  ```

- **HTMX Fragment Pattern**: Pages with dynamic content use paired handlers:
  - Full page: `/experience` → `experienceHandler` → renders full page
  - Fragment: `/experience/timeline` → `experienceTimelineHandler` → renders only `experience_timeline.html`

## Data Flow

All data is **hardcoded in `main.go`** via functions like `experienceData()`, `skillsData()`, `projectsData()`. Each returns typed structs. When adding content, update these functions directly.

## Key Conventions

### Templates

- **Page CSS**: Each page has matching CSS in `static/css/{pagename}.css`, auto-loaded via `{{ .Page }}` in base.html
- **Template Functions**: Custom funcs in `templateFuncs` map: `Year`, `multiply`, `slice`, `hasPrefix`, `mod`, `subtract`
- **Active Nav**: Nav links use `{{ if eq .Page "pagename" }}active{{ end }}` for highlighting

### HTMX Integration

- HTMX loaded in base.html with SRI integrity check
- Fragment targets use `hx-target`, `hx-swap="innerHTML"`, and `hx-indicator` for loading states
- Soccer page demonstrates full HTMX form pattern: `hx-post="/soccer/fetch"` → returns `soccer_table_fragment.html`

### CSS Architecture

- Design tokens in `:root` (light theme) and `.dark` class (dark theme) in `styles.css`
- Theme toggle persists to localStorage, handled by `static/js/theme.js`
- Mobile-first responsive design with consistent spacing/radius variables

## Development Workflow

```bash
# Build and run
go build -o portfolio-server . && ./portfolio-server

# Hot reload (recommended)
go install github.com/cosmtrek/air@latest
air
```

Server runs at `http://localhost:8080`. Templates are loaded once at startup; restart required for template changes unless using `air`.

## Adding New Pages

1. Create handler in `main.go` with data function if needed
2. Register route: `http.HandleFunc("/newpage", newpageHandler)`
3. Add template: `templates/pages/newpage.html` with `{{ define "content" }}`
4. Add CSS: `static/css/newpage.css`
5. Update nav in `templates/partials/nav.html`
6. If HTMX fragments needed, create partial and fragment handler

## File Reference

| Path                          | Purpose                                             |
| ----------------------------- | --------------------------------------------------- |
| `main.go`                     | All handlers, data, routes, template loading        |
| `templates/layouts/base.html` | HTML shell, loads page-specific CSS                 |
| `templates/partials/nav.html` | Navigation with active state logic                  |
| `static/css/styles.css`       | Global styles, CSS variables, dark/light themes     |
| `static/js/theme.js`          | Theme persistence                                   |
| `static/js/main.js`           | Mobile menu, HTMX event handlers, scroll animations |
