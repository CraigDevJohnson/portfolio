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

## Testing and Validation

### Building and Running

```bash
# Build the application
go build -o portfolio-server .

# Run the server
./portfolio-server

# Development with hot reload (recommended)
go install github.com/cosmtrek/air@latest
air
```

### Testing

```bash
# Run all tests (when tests are added)
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests with verbose output
go test -v ./...
```

### Code Quality

```bash
# Format Go code (required before committing)
go fmt ./...

# Run Go linter (if golangci-lint is installed)
golangci-lint run

# Check for common issues
go vet ./...
```

### Validation Checklist

Before submitting changes:

1. ✅ Code builds successfully: `go build -o portfolio-server .`
2. ✅ Server starts and runs: `./portfolio-server`
3. ✅ All pages load correctly at `http://localhost:8080`
4. ✅ HTMX fragments load without errors
5. ✅ Dark/light theme toggle works
6. ✅ Responsive design looks good on mobile and desktop
7. ✅ Code is formatted: `go fmt ./...`
8. ✅ No Go vet issues: `go vet ./...`

## Security Guidelines

### Critical Security Rules

- **Never commit secrets**: API keys, tokens, passwords must never be in source code
- **Validate user input**: All external input (query params, form data) must be validated
- **Escape HTML output**: Use Go's `html/template` package (already enforced) to prevent XSS
- **HTMX security**: Be cautious with `hx-get`/`hx-post` on user-generated content
- **Dependencies**: Keep Go and dependencies updated for security patches

### Safe Patterns to Follow

- Use `html/template` for all HTML rendering (already in use)
- Validate and sanitize all user input
- Use HTTPS in production (handled by deployment/proxy)
- Set appropriate HTTP security headers
- Keep sensitive data in environment variables

## Code Style and Conventions

### Go Style

- Follow standard Go conventions and `gofmt` formatting
- Use tabs for indentation (enforced by `.editorconfig`)
- Keep functions focused and under 50 lines when possible
- Use descriptive variable names (avoid single letters except in short loops)
- Add comments for exported functions and complex logic

### Template Style

- Use semantic HTML5 elements
- Maintain consistent indentation (2 spaces)
- Keep templates DRY - extract repeated patterns to partials
- Use descriptive CSS class names (kebab-case)

### JavaScript Style

- Use modern ES6+ syntax
- Keep inline scripts minimal
- Prefer HTMX attributes over custom JavaScript when possible
- Comment complex DOM manipulations

## Contribution Workflow

### Making Changes

1. Create a feature branch from `main`
2. Make focused, atomic commits
3. Test changes locally following the validation checklist
4. Format code with `go fmt ./...` and check with `go vet ./...`
5. Create a pull request with clear description

### Commit Messages

Follow conventional commits format:

- `feat: add new contact form page`
- `fix: resolve dark theme color contrast issue`
- `docs: update README with deployment instructions`
- `style: format Go code with gofmt`
- `refactor: simplify template rendering logic`

### Pull Request Guidelines

- Keep PRs focused on a single feature/fix
- Include before/after screenshots for UI changes
- Reference related issues with `#issue-number`
- Ensure all validation checks pass before requesting review

## Common Tasks

### Adding a New Page

See "Adding New Pages" section above for the complete checklist.

### Updating Content

All content lives in `main.go` data functions. To update:

1. Find the relevant data function (`experienceData()`, `skillsData()`, etc.)
2. Update the struct data
3. Rebuild and test: `go build -o portfolio-server . && ./portfolio-server`
4. Verify the page displays correctly

### Modifying Styles

1. Edit the appropriate CSS file in `static/css/`
2. For theme-related changes, update both light (`:root`) and dark (`.dark`) variables
3. Test in both light and dark modes
4. Verify responsive behavior on mobile and desktop

### HTMX Debugging

- Open browser DevTools Network tab to see HTMX requests
- Check server logs for handler errors
- Verify fragment templates render correctly in isolation
- Use `htmx.logger` for detailed HTMX debugging: add to `main.js`:
  ```javascript
  htmx.logger = function(elt, event, data) {
    if(console) {
      console.log(event, elt, data);
    }
  }
  ```
