# Craig Johnson portfolio

This repository contains a server-rendered Go application built with Templ,
HTMX, and Tailwind CSS. It serves the public portfolio, a soccer schedule tool,
and an optional EC2 management portal. The same application can run as a
regular HTTP server or behind API Gateway on AWS Lambda.

Pinned versions:

- Go 1.27.0
- Templ 0.3.1020
- HTMX 1.9.10
- Tailwind CSS 4.2.4 standalone CLI

## What is included

The public site has Home, About, Experience, Skills, Projects, Education,
Contact, and Soccer pages. The Skills and Soccer pages use HTMX fragments while
keeping the main content server-rendered.

The Soccer tool supports:

- JWT import from an authenticated Let's Play Soccer browser session
- linked-player and team discovery
- schedule lookup and ICS download
- optional Google Calendar add and result sync
- optional DynamoDB workflow persistence

The management portal is disabled unless its Cognito and session settings are
valid. When enabled, it can list EC2 instances, request start, stop, and restart
actions, and load CloudWatch metrics and logs.

## Requirements

- Go 1.27.0
- [Task](https://taskfile.dev/)
- `curl`, used to install the pinned Tailwind CLI

Install the development tools and build the server:

```bash
go mod download
task install-tools
task build
```

Run the application:

```bash
task run
```

`task run` loads `.env` when it exists. The server listens on
`127.0.0.1:8080` by default. Set `APP_BIND_ALL=true` to listen on all network
interfaces.

## Development

Run Tailwind and Go reload loops in separate terminals:

```bash
task tailwind-watch
```

```bash
task dev
```

Air watches Go files only. After changing a `.templ` file, run
`task generate`; `task build` also regenerates Templ output. Generated
`*_templ.go` files and `cmd/web/static/css/tailwind.css` are ignored.

The main repository commands are:

| Command | Purpose |
| --- | --- |
| `task generate` | Generate Go files from `.templ` sources |
| `task tailwind-build` | Compile Tailwind source CSS |
| `task build` | Generate templates, compile CSS, and build `portfolio-server` |
| `task test` | Run the Go test suite |
| `task vet` | Run `go vet ./...` |
| `task fmt` | Format with `golangci-lint fmt` |
| `task lint` | Format, then run `golangci-lint run` |
| `task ci` | Clean, generate, format, vet, lint, test, and build |
| `task portal-preview` | Run the mock portal on loopback |
| `task compose` | Build and start the Compose service |

`Taskfile.yaml` is the command source of truth. In particular, use `task fmt`
instead of `go fmt ./...` for the repository formatting gate.

## Local configuration

Copy the example file before using Docker Compose or optional features:

```bash
cp .env.example .env
```

Do not commit `.env`.

### Soccer authentication

Set `LPS_SESSION_KEY` to a 64-character hexadecimal value. Generate one with:

```bash
openssl rand -hex 32
```

Without a valid key, JWT import is disabled. `LPS_API_BASE_URL` can override the
upstream API for local testing. The application accepts HTTPS endpoints and
loopback HTTP endpoints.

### Google Calendar

Direct calendar actions require soccer authentication plus:

- `CLIENT_ID_KEY`
- `CLIENT_SECRET_KEY`
- `GOOGLE_CONNECTION_TABLE_NAME`

Register each application URL ending in `/soccer` as an authorized redirect
URI in the Google OAuth client. Local DynamoDB access uses the standard AWS
credential chain.

Set `SOCCER_SESSION_TABLE_NAME` to store Soccer workflow state in DynamoDB.
Without it, the encrypted browser session remains the fallback.

### EC2 management portal

The portal requires:

- `MGMT_SESSION_KEY`, generated with `openssl rand -hex 32`
- `MGMT_COGNITO_DOMAIN`
- `MGMT_COGNITO_CLIENT_ID`

`MGMT_COGNITO_REDIRECT_URI` sets the OAuth callback. The optional
`MGMT_COGNITO_LOGOUT_URI` sets the post-logout return URL.
`MGMT_AWS_REGION` defaults to `us-east-1`.

The runtime AWS identity needs these actions:

- `ec2:DescribeInstances`
- `ec2:StartInstances`
- `ec2:StopInstances`
- `cloudwatch:GetMetricStatistics`
- `logs:FilterLogEvents`

For a mock review that constructs no Cognito or AWS clients, run:

```bash
task portal-preview
```

Then open `http://localhost:8080/mgmt`. Preview mode requires a loopback
listener and is unavailable in the Lambda handler.

## Soccer import flow

1. Sign in to Let's Play Soccer in a browser.
2. Copy the bearer JWT from the authenticated session. The helper extension in
   `chrome-extension/` can capture and copy it.
3. Import the JWT on `/soccer`.
4. The server calls `/users/check`, filters deleted players, and shows the
   linked players.
5. Select players and teams, then fetch schedules.
6. Download an ICS file or use the configured Google Calendar actions.

A new JWT import or explicit logout clears downstream workflow state. Google
connect, reconnect, disconnect, and calendar selection preserve the current
player and team workflow.

## Source layout

```text
portfolio/
├── chrome-extension/       Chrome helper for Soccer JWT import
├── cmd/
│   ├── lambda/             Lambda adapter and SSM secret resolution
│   ├── server/             HTTP server entry point
│   └── web/                Templ, Tailwind, JavaScript, and static assets
├── docs/deployment/        Runtime-specific deployment notes
├── infra/                  OpenTofu resources for AWS
├── internal/
│   ├── app/                Startup, dependency injection, and routes
│   ├── config/             Environment parsing and feature flags
│   ├── google/             OAuth, Calendar API, and connection storage
│   ├── httpx/              Request and cookie helpers
│   ├── lps/                Let's Play Soccer API client
│   ├── portal/             Cognito, EC2, metrics, and logs
│   ├── portfolio/          Portfolio handlers and embedded data
│   ├── schedule/           Schedule normalization and ICS output
│   ├── session/            Encryption and login rate limiting
│   └── soccer/             Soccer auth and schedule handlers
└── types/                  Shared application models
```

The source-of-truth order is:

1. `Taskfile.yaml` for commands
2. `cmd/server/main.go` and `internal/app/` for application wiring
3. this README for local usage and architecture
4. `DEPLOY-INSTRUCTIONS.md` and `infra/*.tf` for deployment

Edit `.templ` and `cmd/web/tailwind/` sources. Do not hand-edit generated
`*_templ.go` files or `cmd/web/static/css/tailwind.css`.

Portfolio content lives in embedded JSON under `internal/portfolio/data/`.
Education credentials live in `cmd/web/pages/education_viewmodels.go`.

## Routes

Public pages:

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/` | Home |
| `GET` | `/home` | Permanent redirect to `/` |
| `GET` | `/about` | About |
| `GET` | `/experience` | Experience |
| `GET` | `/skills` | Skills |
| `GET` | `/projects` | Projects |
| `GET` | `/education` | Education |
| `GET` | `/contact` | Contact |
| Any | `/soccer` | Soccer page and Google OAuth callback |

HTMX and form endpoints:

| Method | Path |
| --- | --- |
| `GET` | `/skills/filtered` |
| `GET` | `/skills/detail` |
| `GET` | `/soccer/session` |
| `POST` | `/soccer/import` |
| `POST` | `/soccer/logout` |
| `POST` | `/soccer/discover-teams` |
| `POST` | `/soccer/fetch` |
| `POST` | `/soccer/download` |
| `GET` | `/soccer/google/connect` |
| `POST` | `/soccer/google/calendar` |
| `POST` | `/soccer/google/disconnect` |
| `POST` | `/soccer/google/add` |
| `POST` | `/soccer/google/sync-results` |

Portal routes are registered only in valid production configuration or local
preview mode. They include `/login`, `/auth/callback`, `/logout`, `/mgmt`, and
the instance action, metrics, and logs paths under `/mgmt/instances/{id}/`.

## Chrome extension

`chrome-extension/` contains a Manifest V3 extension for the Soccer import
flow. Its service worker captures Let's Play Soccer bearer credentials from an
authenticated browser session. The content script fills the import field, and
the popup shows capture status and copy controls.

Load it from `chrome://extensions/` with Developer mode and "Load unpacked."
Select the `chrome-extension/` directory.

## Deployment

The repository supports AWS App Runner and AWS Lambda with API Gateway. Read
[`DEPLOY-INSTRUCTIONS.md`](./DEPLOY-INSTRUCTIONS.md) for shared infrastructure,
secrets, deployment, update, and teardown procedures. Lambda-specific behavior
is documented in
[`docs/deployment/aws-lambda-api-gateway.md`](./docs/deployment/aws-lambda-api-gateway.md).
