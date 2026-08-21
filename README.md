# Craig Johnson Portfolio - Go + Templ + HTMX

A modern, responsive personal portfolio website built with **Go**, **Templ**, and **HTMX** for server-side rendering with dynamic client interactions.

![Go](https://img.shields.io/badge/Go-1.26.6-00ADD8?style=flat-square&logo=go&logoColor=white)
![Templ](https://img.shields.io/badge/Templ-0.3.1020-FF6B6B?style=flat-square)
![HTMX](https://img.shields.io/badge/HTMX-1.9.10-3366CC?style=flat-square)

## Overview

A server-rendered Go application using Templ for type-safe component-based templates and HTMX for dynamic interactions. The site maintains a professional design and functionality while leveraging server-side rendering for improved performance and SEO.

## Features

- **Type-Safe Templates**: Templ provides compile-time type checking and component-based architecture
- **Server-Side Rendering**: Fast initial page loads with targeted client-side behavior
- **HTMX Interactions**: Dynamic content loading without full page refreshes
- **Responsive Design**: Mobile-first approach with beautiful desktop layouts
- **Professional UI**: Modern design with smooth animations and polish
- **Soccer Tool**: JWT import, automatic player discovery, schedule fetch, direct Google Calendar add, and ICS download
- **EC2 Management Portal**: Optional Cognito-authenticated EC2 controls, CloudWatch metrics, and CloudWatch Logs

## Pages

- **Home** - Hero section with profile, stats, and quick links
- **About** - Personal background and values
- **Experience** - Server-rendered career stages and role history
- **Skills** - Technical proficiencies by category
- **Projects** - Project showcase with links
- **Education** - Academic background and certifications
- **Contact** - Professional contact information
- **Soccer** - Interactive schedule download tool
- **Management portal** - Protected EC2 management dashboard (optional)

## Tech Stack

- **Backend**: Go 1.26.6
- **Templating**: [Templ](https://templ.guide/) - Type-safe Go templating engine
- **Frontend Interactivity**: HTMX 1.9.10
- **Styling**: Tailwind CSS 4.2.4 standalone CLI with CSS-first theme and component layers
- **Fonts**: Atkinson Hyperlegible, Bricolage Grotesque, and IBM Plex Mono

## Getting Started

### Prerequisites

- Go 1.26.6
- `task` command runner (installed via `go install github.com/go-task/task/v3/cmd/task`)
- `curl` (used by `task install-tailwind` to download the pinned standalone Tailwind CLI)

### Installation

```bash
# Install dependencies
go mod download

# Install Air, golangci-lint, the pinned Tailwind CLI, and Templ
task install-tools

# Build the project (generates Templ components and compiles)
task build
```

When you edit Templ files (`*.templ`), run `task generate` before building unless
another command already does it for you.

### Running

```bash
# Run the server
./portfolio-server

# Or use task
task run
```

The server will start at `http://localhost:8080`

### Development

For development with hot reload, use `task dev` which requires [air](https://github.com/air-verse/air):

```bash
# In one terminal, rebuild Tailwind CSS while you edit Tailwind source files
task tailwind-watch

# In another terminal, start the Go hot-reload loop
task dev
```

**Note**: When you edit `.templ` files, run `task generate` separately.
The `air` hot-reload loop started by `task dev` does not regenerate Templ output.

Tailwind source files live under `cmd/web/tailwind/`. The generated output is
written to `cmd/web/static/css/tailwind.css` and is not committed.

`cmd/web/tailwind/app.css` imports shared sources, route-specific files under
`cmd/web/tailwind/pages/`, Soccer styles, and portal styles. The app serves only
the generated `tailwind.css` at runtime.

### Validation

Use the existing `task` commands for verification. See `Taskfile.yaml` for the full task
command list, including:

```bash
# Format and lint
task fmt
task lint

# Fast checks during development
task vet
task test
task build

# Full project gate
task ci
```

`task build` regenerates Templ output before compiling. `task ci` cleans the
binary, runs vet and formatting/lint checks, runs tests, and builds the server.

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

### Optional EC2 Management Portal

The management portal is disabled by default in normal runtime. When enabled, it provides a
protected dashboard at `/mgmt` for viewing EC2 instances, requesting start,
stop, and restart actions, and loading CPU metrics and recent CloudWatch Logs.
Authentication uses Cognito's OAuth 2.0 authorization-code flow with PKCE; the
application does not receive or store Cognito passwords.

For local design review without Cognito or AWS credentials, run:

```bash
task portal-preview
```

Then open `http://localhost:8080/mgmt`. This mode uses representative mock
instances, metrics, logs, and harmless action responses; it never constructs
the portal's Cognito or AWS clients. The preview requires the explicit
`MGMT_LOCAL_PREVIEW=true` flag and a loopback-only listener. It is hard-disabled
for the Lambda handler and refused when the server binds to a network interface.

Configure these runtime variables to enable it:

- `MGMT_SESSION_KEY` — 64-character lowercase hexadecimal key for encrypted
    portal session and OAuth-state cookies. Generate one with
    `openssl rand -hex 32`.
- `MGMT_COGNITO_DOMAIN` — Cognito Hosted UI HTTPS origin, such as
    `https://portal.auth.us-east-1.amazoncognito.com`.
- `MGMT_COGNITO_CLIENT_ID` — Cognito app-client ID configured for authorization
    code flow and PKCE.
- `MGMT_COGNITO_REDIRECT_URI` — callback URI registered with Cognito, normally
    `https://your-domain.example/auth/callback` or
    `http://localhost:8080/auth/callback` for local testing.
- `MGMT_COGNITO_LOGOUT_URI` — optional Cognito sign-out return URI.
- `MGMT_AWS_REGION` — AWS region for EC2 and CloudWatch calls; defaults to
    `us-east-1`.

The runtime AWS identity must be allowed to call `ec2:DescribeInstances`,
`ec2:StartInstances`, `ec2:StopInstances`,
`cloudwatch:GetMetricStatistics`, and `logs:FilterLogEvents`. The portal does
not provision Cognito resources or these IAM permissions. See
`DEPLOY-INSTRUCTIONS.md` for setup details.

## Project Structure

```filetree
portfolio/
├── cmd/
│   ├── server/
│   │   └── main.go         # Thin executable entrypoint
│   ├── lambda/              # Lambda adapter and cold-start secret resolution
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
│   ├── portal/             # Cognito auth, EC2 actions, metrics, and logs
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

- `GET /skills/filtered` - Filtered skills grid fragment
- `GET /skills/detail` - Skill detail fragment
- `GET /soccer/session` - Current soccer auth state fragment
- `GET /soccer/google/connect` - Start Google OAuth for calendar access
- `POST /soccer/google/calendar` - Save the selected Google Calendar
- `POST /soccer/google/disconnect` - Remove the persisted Google Calendar connection
- `POST /soccer/google/add` - Add selected games directly to Google Calendar
- `POST /soccer/google/sync-results` - Update synced past games with result text
- `POST /soccer/import` - Import JWT and auto-discover linked players
- `POST /soccer/logout` - Clear imported soccer session
- `POST /soccer/discover-teams` - Discover teams for selected imported players
- `POST /soccer/fetch` - Fetch soccer schedules
- `POST /soccer/download` - Download ICS file

### EC2 Management Portal

In normal runtime these routes are registered only when portal configuration is
valid, and `/mgmt` requires an encrypted portal session cookie. `task
portal-preview` registers harmless loopback-only mock versions of the same
routes for design review.

- `GET /login` - Start Cognito OAuth 2.0 authorization-code + PKCE login
- `GET /auth/callback` - Complete the Cognito login callback
- `POST /logout` - Clear the portal session and optionally sign out of Cognito
- `GET /mgmt` - Protected EC2 management dashboard
- `POST /mgmt/instances/{id}/start` - Request an EC2 start action
- `POST /mgmt/instances/{id}/stop` - Request an EC2 stop action
- `POST /mgmt/instances/{id}/restart` - Stop then start an EC2 instance
- `GET /mgmt/instances/{id}/metrics` - Load the last hour of CPU metrics
- `GET /mgmt/instances/{id}/logs` - Load recent CloudWatch log events

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

Education credential data lives in `cmd/web/pages/education_viewmodels.go`; the
degree and page copy live in `cmd/web/pages/education.templ`.

### Updating Templates

Templates are written in Templ (`.templ` files):

1. Edit the `.templ` files in `cmd/web/`
2. Run `task generate` to regenerate Go code
3. Build and run: `task build && ./portfolio-server`

For more information on Templ syntax, see the [Templ documentation](https://templ.guide/).

### Styling

Shared design tokens, reusable UI primitives, and route-specific CSS live under
`cmd/web/tailwind/`, and compile into the generated
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

For the repository's AWS App Runner and Lambda deployment workflow, see
[`DEPLOY-INSTRUCTIONS.md`](./DEPLOY-INSTRUCTIONS.md).

For local container testing, use Docker Compose:

```bash
cp .env.example .env
# Set LPS_SESSION_KEY in .env to a 64-character hex string

docker compose up --build
```

The app is available at `http://localhost:8080`.

Compose reads the local `.env` file automatically. Set `LPS_SESSION_KEY` in
`.env` to a 64-character hex string before starting the stack. This key encrypts
soccer session cookies, so rotating it invalidates existing sessions. Compose
also passes through the documented LPS, Google, Soccer session-store, portal,
logging, and local AWS variables from `.env`.

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
- Regenerate Templ output (`task generate`) before building if `.templ` files were changed.
- Templ source files are compiled into the binary, so they are not needed in the runtime image.

## License

MIT License - feel free to use this as a template for your own portfolio!
