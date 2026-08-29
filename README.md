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
- optional DynamoDB audit baselines for imported sessions

The management portal routes are disabled unless their Cognito and session
settings are valid. A registered OAuth redirect URI is also required for sign-in.
When enabled, the portal can list EC2 instances, request start, stop, and restart
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
| `task test` | Generate templates, then run the Go test suite |
| `task vet` | Generate templates, then run `go vet ./...` |
| `task fmt` | Format with `golangci-lint fmt` |
| `task lint` | Generate templates, format, then run `golangci-lint run` |
| `task ci` | Clean, generate, format, vet, lint, test, and build |
| `task build-image` | Build a local amd64 server image; no push or deploy |
| `task build-lambda-image` | Build amd64 Lambda image; no push or deploy |
| `task test-images` | Verify both local Linux amd64 image contracts |
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

The encrypted browser cookie is the Soccer workflow source of truth. When
`SOCCER_SESSION_TABLE_NAME` is set, the application also writes an import
baseline containing the username and discovered players; it does not restore
workflow state from that table.

### EC2 management portal

The portal requires:

- `MGMT_SESSION_KEY`, generated with `openssl rand -hex 32`
- `MGMT_COGNITO_DOMAIN`
- `MGMT_COGNITO_CLIENT_ID`

`MGMT_COGNITO_REDIRECT_URI` must be a callback registered with Cognito for
sign-in to work. The optional
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

Then open the `/mgmt` URL printed at startup (port `8080` by default). Preview
mode requires a loopback listener and is unavailable in the Lambda handler.

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
├── infra/                  Legacy Lambda/API and shared-data OpenTofu state
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

Dated files under `docs/superpowers/` are historical design and QA records, not
current runtime documentation. Validate operational behavior against the source
of truth above.

Portfolio content lives in embedded JSON under `internal/portfolio/data/`.
Education credentials live in `cmd/web/pages/education_viewmodels.go`.

## Routes

Public pages:

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/healthz` | JSON health and revision; no dependency probes |
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

The current release candidate prepares the application for a replacement AWS
Lambda and API Gateway environment, but it does not create or deploy that
environment. The replacement deployment contract is a 29-second Lambda timeout.
The Google add and result-sync handlers reserve 24 seconds of that window, which
leaves five seconds outside their application work budget. A later environment
plan must implement the 29-second setting before release.

The checked-in `infra/` directory retains the legacy Lambda/API Gateway
resources, shared data, ECR, and SSM configuration. The state still contains
the pending removal of App Runner-managed resources until an approved retirement
plan is applied. Its Lambda timeout defaults to 30 seconds. The retired
App Runner deployment and log interfaces are unavailable; the retained
`task deploy-lambda` and `task redeploy-lambda` helpers concern only that legacy
Lambda runtime and are not replacement-release commands.

App Runner retirement uses only `legacy-apprunner-retirement-init`,
`legacy-apprunner-retirement-plan`, and
`legacy-apprunner-retirement-apply`. They require
`AWS_PROFILE=portfolio-deployer`, `AWS_REGION=us-west-2`, the exact state-lock
URI `s3://portfolio-tofu-state-180294223248/portfolio/terraform.tfstate.tflock`,
a new absolute `PLAN_FILE`, the checker-reviewed saved plan, and a separately
approved SHA-256 checksum before apply. The live App Runner custom-domain
inventory and disassociation are an out-of-band, separately approved boundary
that must be complete before apply; this branch does not authorize it. See
[`DEPLOY-INSTRUCTIONS.md`](./DEPLOY-INSTRUCTIONS.md) for the complete contract.

At the Lambda boundary, the adapter derives an HTTPS origin from API Gateway's
typed request context. That context controls secure cookies and generated URLs;
client-supplied host and forwarding headers cannot override it. Cold-start
initialization has an eight-second bound. It reads configured SSM paths in one
decrypted batch, validates the complete response before changing the
environment, constructs the application once, and reuses the proxy on warm
invocations.

Both Google Calendar add and result-sync operations have a 24-second request
budget. If the deadline interrupts a batch, the response includes counts for
completed work and tells the user to retry. Retries match existing games and
update them instead of duplicating completed inserts.

`task build-image` and `task build-lambda-image` build local Linux amd64 images.
By default, they inject the current full Git SHA as the build revision; a
supplied `BUILD_REVISION` overrides it. An exact `/healthz` comparison against
that expected value proves the identity of those artifacts. Legacy deployment
helpers and direct builds that omit `BUILD_REVISION` may report `development`,
which is not immutable provenance proof. `task test-images` verifies the image
contracts. These tasks do not push an image, apply infrastructure, or deploy a
service.

Read [`DEPLOY-INSTRUCTIONS.md`](./DEPLOY-INSTRUCTIONS.md) for the retirement
boundary and retained legacy Lambda guidance.
Lambda runtime details are in
[`docs/deployment/aws-lambda-api-gateway.md`](./docs/deployment/aws-lambda-api-gateway.md).
