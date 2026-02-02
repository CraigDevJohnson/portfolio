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
go install github.com/air-verse/air@latest
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

See "Adding a new page" section above for the complete checklist.

### Updating Content

All content lives in `main.go` data functions. To update:

1. Find the relevant data function (`experienceData()`, `skillsData()`, etc.)
2. Update the struct data
3. Rebuild and test: `go build -o portfolio-server . && ./portfolio-server`
4. Verify the page displays correctly

### Modifying Styles

1. Edit the appropriate CSS file in `static/css/`
2. For theme-related changes, update the CSS custom properties defined in the `:root` scope
3. Test the updated styles in the browser
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
