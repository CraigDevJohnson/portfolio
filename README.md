# Craig Johnson Portfolio - Go + Templ + HTMX

A modern, responsive personal portfolio website built with **Go**, **Templ**, and **HTMX** for server-side rendering with dynamic client interactions.

![Go](https://img.shields.io/badge/Go-1.26.1-00ADD8?style=flat-square&logo=go&logoColor=white)
![Templ](https://img.shields.io/badge/Templ-0.3-FF6B6B?style=flat-square)
![HTMX](https://img.shields.io/badge/HTMX-1.9-3366CC?style=flat-square)

## Overview

A server-rendered Go application using Templ for type-safe component-based templates and HTMX for dynamic interactions. The site maintains a professional design and functionality while leveraging server-side rendering for improved performance and SEO.

## Features

- **Type-Safe Templates**: Templ provides compile-time type checking and component-based architecture
- **Server-Side Rendering**: Fast initial page loads with targeted client-side behavior
- **HTMX Interactions**: Dynamic content loading without full page refreshes
- **Responsive Design**: Mobile-first approach with beautiful desktop layouts
- **Professional UI**: Modern design with smooth animations and polish
- **Soccer Tool**: JWT import, automatic player discovery, schedule fetch, direct Google Calendar add, and ICS download

## Pages

- **Home** - Hero section with profile, stats, and quick links
- **About** - Personal background and values
- **Experience** - Career timeline with HTMX lazy loading
- **Skills** - Technical proficiencies by category
- **Projects** - Project showcase with links
- **Education** - Academic background and certifications
- **Contact** - Professional contact information
- **Soccer** - Interactive schedule download tool

## Tech Stack

- **Backend**: Go 1.26.1
- **Templating**: [Templ](https://templ.guide/) - Type-safe Go templating engine
- **Frontend Interactivity**: HTMX 1.9
- **Styling**: Tailwind CSS v4 standalone CLI with CSS-first theme and component layers
- **Fonts**: Inter (Google Fonts)

## Getting Started

### Prerequisites

- Go 1.26.1
- Just command runner
- `curl` (used by `just install-tailwind` to download the pinned standalone Tailwind CLI)

### Installation

```bash
# Install dependencies
go mod download

# Install the pinned Tailwind standalone CLI
just install-tailwind

# Build the project (generates Templ components and compiles)
just build
```

`just build` uses the Templ version pinned in `go.mod` via `go tool templ`, so no separate Templ install step is required.

When you edit Templ files (`*.templ`), run `just generate` before building unless
another command already does it for you.

### Running

```bash
# Run the server
./portfolio-server

# Or use just
just run
```

The server will start at `http://localhost:8080`

### Development

For development with hot reload, use `just dev` which requires [air](https://github.com/air-verse/air):

```bash
# Install air and the pinned Tailwind standalone CLI
just install-tools

# In one terminal, rebuild Tailwind CSS while you edit Tailwind source files
just tailwind-watch

# In another terminal, start the Go hot-reload loop
just dev
```

**Note**: When you edit `.templ` files, run `just generate` separately.
The `air` hot-reload loop started by `just dev` does not regenerate Templ output.

Tailwind source files live under `cmd/web/tailwind/`. The generated output is
written to `cmd/web/static/css/tailwind.css` and is not committed.

The remaining shared and page styles are compiled through the Tailwind pipeline
from `cmd/web/tailwind/legacy/`, so the app only serves the generated
`tailwind.css` at runtime.

### Validation

Use the existing `just` recipes for verification. See `justfile` for the full
command list, including:

```bash
# Format and lint
just fmt
just lint

# Fast checks during development
just vet
just test
just build

# Full project gate
just ci
```

`just build` regenerates Templ output before compiling, and `just ci` is the
final validation gate for formatting, vet, generation, build, and tests.

### Optional Google Calendar Integration

The soccer tool can optionally add games directly to Google Calendar.
To enable that feature, configure these runtime environment variables:

- `CLIENT_ID_KEY`
- `CLIENT_SECRET_KEY`
- `GOOGLE_CONNECTION_TABLE_NAME`

For local OAuth testing, add `http://localhost:8080/soccer` as an authorized
redirect URI in the Google Cloud OAuth client alongside the production
`https://craigdevjohnson.com/soccer` redirect.

Because the Google connection is stored server-side, local runs also need AWS
credentials or another valid AWS auth source that can reach the configured
DynamoDB table.

## Project Structure

```filetree
portfolio/
├── cmd/
│   ├── server/
│   │   └── main.go         # Thin executable entrypoint
│   └── web/
│       ├── layouts/        # Templ layouts
│       ├── pages/          # Full-page Templ components
│       ├── partials/       # HTMX fragments and reusable partials
│       └── static/         # CSS, JS, images served at /static/
├── go.mod                  # Go module definition
├── internal/
│   ├── app/                # Server startup, route wiring, dependency injection
│   ├── config/             # Env parsing, feature toggles, constants
│   ├── google/             # OAuth, Calendar API, DynamoDB store
│   ├── httpx/              # HTTP/cookie/request helpers
│   ├── lps/                # LPS API client, schedule resolver, decode, errors
│   ├── portfolio/          # Portfolio data and page handler helpers
│   ├── schedule/           # Game normalization, ICS building, time helpers
│   ├── session/            # AES-GCM encryption, login rate limiting
│   ├── soccer/             # Soccer auth, schedule fetch/download handlers
│   └── testutil/           # Shared test helpers
└── types/
    └── types.go            # Shared typed models
```

**Note**: `*_templ.go` files are auto-generated by Templ and are gitignored.

## API Endpoints
<!-- markdownlint-disable MD024  -->
### Pages

- `GET /` - Home page
- `GET /about` - About page
- `GET /experience` - Experience page
- `GET /skills` - Skills page
- `GET /projects` - Projects page
- `GET /education` - Education page
- `GET /contact` - Contact page
- `GET /soccer` - Soccer tool page

### HTMX Fragments

- `GET /experience/timeline` - Experience timeline fragment
- `GET /skills/grid` - Skills grid fragment
- `GET /skills/filtered` - Filtered skills grid fragment
- `GET /skills/detail` - Skill detail fragment
- `GET /projects/grid` - Projects grid fragment
- `GET /soccer/session` - Current soccer auth state fragment
- `GET /soccer/google/connect` - Start Google OAuth for calendar access
- `POST /soccer/google/calendar` - Save the selected Google Calendar
- `POST /soccer/google/disconnect` - Remove the persisted Google Calendar connection
- `POST /soccer/google/add` - Add selected games directly to Google Calendar
- `POST /soccer/import` - Import JWT and auto-discover linked players
- `POST /soccer/logout` - Clear imported soccer session
- `POST /soccer/fetch` - Fetch soccer schedules
- `POST /soccer/download` - Download ICS file

## Design Principles

1. **Type-Safe Components**: Templ provides compile-time type checking for templates
2. **Progressive Enhancement**: Core content works without JavaScript
3. **HTMX for Interactivity**: Dynamic updates without SPA complexity
4. **Server-Rendered**: Fast initial loads, great SEO
5. **Mobile-First**: Responsive design starting from mobile
6. **Accessible**: Semantic HTML, ARIA labels, keyboard navigation
7. **Component-Oriented**: Templ pages and partials keep server-rendered UI explicit

## Customization

### Updating Content

Portfolio content is loaded from embedded JSON in `internal/portfolio/data.go`:

- `ExperienceData()` - Work experience entries from `internal/portfolio/data/experience.json`
- `SkillsData()` - Skills by category from `internal/portfolio/data/skills.json`
- `ProjectsData()` - Project showcase from `internal/portfolio/data/projects.json`

The education page content is currently maintained directly in
`cmd/web/pages/education.templ`.

### Updating Templates

Templates are written in Templ (`.templ` files):

1. Edit the `.templ` files in `cmd/web/`
2. Run `just generate` to regenerate Go code
3. Build and run: `just build && ./portfolio-server`

For more information on Templ syntax, see the [Templ documentation](https://templ.guide/).

### Styling

Shared design tokens, reusable UI primitives, and the remaining CSS-first
legacy rules live under `cmd/web/tailwind/`, and compile into the generated
`cmd/web/static/css/tailwind.css`. The shared styling system owns:

- Colors for the dark-first Tailwind theme
- Spacing
- Typography
- Shadows
- Animations
- Reusable card, alert, stats-grid, and section-header primitives

## Chrome Extension

The `chrome-extension/` directory contains a Manifest V3 Chrome extension that streamlines the soccer login flow.

### What It Does

- **JWT Capture**: Passively intercepts bearer tokens from authenticated requests to letsplaysoccer.com.
- **Autofill**: When you open the soccer page, the extension automatically fills the JWT into the login modal.
- **Popup UI**: Shows capture status, lets you copy the token, or open the soccer page directly.

### Installation (Developer Mode)

1. Open `chrome://extensions/` and enable **Developer mode**.
2. Click **Load unpacked** and select the `chrome-extension/` folder.
3. Visit [letsplaysoccer.com](https://letsplaysoccer.com) while signed in — the extension captures the JWT automatically.
4. Navigate to the soccer page; the token is autofilled into the import modal.

### Files

| File | Purpose |
| --- | --- |
| `manifest.json` | Extension metadata and permissions |
| `lps-jwt-extractor.js` | Service worker that intercepts auth headers and cookies |
| `soccer-autofill.js` | Content script that fills the JWT into the login modal |
| `popup.html` / `popup.css` / `popup.js` | Popup UI for viewing and copying the captured token |

## Deployment

For production and AWS App Runner deployment, see
[`DEPLOY-INSTRUCTIONS.md`](./DEPLOY-INSTRUCTIONS.md).

For local container testing, use Docker Compose:

```bash
cp .env.example .env
# Set LPS_SESSION_KEY in .env to a 64-character hex string

docker compose up --build
```

The app is available at `http://localhost:8080`.

Compose reads the local `.env` file automatically. Set `LPS_SESSION_KEY` in `.env` to a 64-character hex string before starting the stack. This key is used to encrypt soccer session cookies, so rotating it invalidates existing sessions. `LPS_API_BASE_URL` is optional if you need to point the app at a non-default upstream API.

### JWT Soccer Import Workflow

The authenticated soccer import flow requires `LPS_SESSION_KEY` to be set before the server starts. Without it, the app cannot encrypt the current-session cookie that stores the imported bearer token and discovered players.

Use this flow to import a current Let's Play Soccer session token:

1. Sign in on letsplaysoccer.com in your browser.
2. Copy the bearer JWT from your current authenticated session. You can use
   browser DevTools or the included helper extension in `chrome-extension/`.
3. Open the soccer page in this app and import the JWT.
4. The server calls `/users/check`, discovers linked players, filters deleted players, and shows the player selector with all discovered players pre-selected.
5. Fetch schedules or export ICS for the selected players.
6. If the server reports that the token was expired or rejected, repeat the import with a fresh JWT from a current Let's Play Soccer session.

#### Chrome extension helper

This repository includes a working helper extension under `chrome-extension/`
for copying the current Let's Play Soccer JWT. Use the files in that directory
as the source of truth instead of older inline manifest examples.

### Container Notes

- Runtime image uses distroless and runs as a non-root user.
- Static assets are copied into the image at build time.
- Regenerate Templ output (`just generate`) before building if `.templ` files were changed.
- Templ source files are compiled into the binary, so they are not needed in the runtime image.

## License

MIT License - feel free to use this as a template for your own portfolio!
