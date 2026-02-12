# Portfolio Codebase Instructions

**Always use Context7 MCP when I need library/API documentation, code generation, setup or configuration steps without me having to explicitly ask.**

## Architecture overview

Go 1.23+ server-rendered portfolio with Templ for type-safe component-based templates and HTMX for dynamic updates. All application code lives in a single `main.go` file—handlers and data structs are collocated intentionally for simplicity.

## Templ Component System

Type-safe component-based architecture using [Templ](https://templ.guide/):

- **Layouts**: `components/layouts/base.templ` — defines layout wrapper with children injection
- **Pages**: `components/pages/*.templ` — page components that use @layouts.Base() wrapper
- **Partials**: `components/partials/*.templ` — shared components (nav, header, footer) and HTMX fragments

### Key Templ Concepts

- **Type-safe props**: Each component has a Props struct defining its data
- **Component composition**: Use `@ComponentName(props)` to render child components
- **Children**: Use `{ children... }` in layouts to inject child content
- **Conditional classes**: Use `templ.KV("classname", condition)` for conditional CSS classes
- **No template functions needed**: Use native Go functions and time package directly

### Component Generation

Templ files (`*.templ`) are compiled to Go code (`*_templ.go`):

```bash
# Generate all Templ components
templ generate

# Or use make
make generate
```

Generated `*_templ.go` files are gitignored and should not be edited manually.

## Handler patterns

Full page render with Templ:

```go
func pageHandler(w http.ResponseWriter, r *http.Request) {
    props := pages.PageNameProps{
        Field1: "value",
        Field2: 123,
    }
    if err := pages.PageName(props).Render(context.Background(), w); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
    }
}
```

HTMX fragment render (no layout wrapper):

```go
func fragmentHandler(w http.ResponseWriter, r *http.Request) {
    props := partials.FragmentProps{
        Data: someData,
    }
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    if err := partials.Fragment(props).Render(context.Background(), w); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
    }
}
```

## Data layer

Content is hardcoded via typed structs and factory functions:

- `experienceData()` → `[]Experience`
- `skillsData()` → `[]SkillCategory`
- `projectsData()` → `[]Project`
- `educationData()` → `[]Education`

Each struct includes all fields needed for component rendering (no database queries).

## HTMX integration patterns

Fragment routes return partial HTML for swap targets:

- `/experience` (full page) + `/experience/timeline` (fragment) → `experience_timeline.templ`
- `/skills` + `/skills/grid` → `skills_grid.templ`
- `/projects` + `/projects/grid` → `projects_grid.templ`

Soccer page demonstrates form-driven HTMX:

- `POST /soccer/fetch` → returns `soccer_table_fragment.templ`
- `POST /soccer/download` → streams ICS file attachment

## Frontend conventions

- Per-page CSS: `static/css/{pagename}.css` — linked automatically via `.Page` prop in base layout
- Global styles: `static/css/styles.css` — CSS custom properties for theming
- Active nav highlighting: Use `templ.KV("active", page == "pagename")` in Templ components

## Dev workflow

```bash
# Generate Templ components (required after editing .templ files)
just generate

# Build and run
just build
just run

# Development with hot reload (requires air)
just dev

# Format and lint
just fmt
just vet
```

Templ files must be regenerated after editing—restart server or use `just dev` for auto-reload.

## Adding a new page

1. **Create Templ component**: `components/pages/newpage.templ`
   ```go
   package pages
   
   import "portfolio/components/layouts"
   
   type NewPageProps struct {
       // Define your props
   }
   
   templ NewPage(props NewPageProps) {
       @layouts.Base(layouts.BaseProps{
           Title: "Page Title - Craig Johnson",
           Page:  "newpage",
       }) {
           // Your page content
       }
   }
   ```

2. **Create handler in main.go**:
   ```go
   func newPageHandler(w http.ResponseWriter, r *http.Request) {
       props := pages.NewPageProps{ /* ... */ }
       if err := pages.NewPage(props).Render(context.Background(), w); err != nil {
           http.Error(w, err.Error(), http.StatusInternalServerError)
       }
   }
   ```

3. **Add route in main()**: `http.HandleFunc("/newpage", newPageHandler)`

4. **Update navigation**: Add link to `components/partials/nav.templ` and `header.templ` (mobile nav)

5. **Create page styles**: `static/css/newpage.css` (optional)

6. **Generate and build**: `just generate && just build`

## Testing and Validation

### Building and Running

```bash
# Generate Templ components
just generate

# Build the application  
just build

# Run the server
./portfolio-server

# Development with hot reload (recommended)
just install-air
just dev
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
5. ✅ Design looks good in the dark theme
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
  htmx.logger = function (elt, event, data) {
    if (console) {
      console.log(event, elt, data);
    }
  };
  ```
