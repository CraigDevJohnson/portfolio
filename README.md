# Craig Johnson portfolio

This repository contains a server-rendered Go application built with Templ,
HTMX, and Tailwind CSS. It serves the public portfolio, a soccer schedule tool,
optional invited site sign-in, and an optional EC2 management portal. The same
application can run as a regular HTTP server or behind API Gateway on AWS Lambda.

Pinned versions:

- Go 1.27.0
- Templ 0.3.1020
- HTMX 1.9.10
- Tailwind CSS 4.2.4 standalone CLI

## What is included

The public site has Home, About, Experience, Skills, Projects, Education,
Contact, and Soccer pages. The Skills and Soccer pages use HTMX fragments while
keeping the main content server-rendered.

The Soccer tool keeps Team ID schedule lookup and ICS download public. An
invited site account with the current `soccer` grant can also:

- import a JWT from an authenticated Let's Play Soccer browser session
- discover linked players and their teams
- connect Google Calendar separately to add games and sync results

Imported sessions can optionally write DynamoDB audit baselines.
The encrypted LPS import cookie stays in the same browser until the JWT expires,
for at most 12 hours; a JWT without a valid expiry is rejected. It is bound to
the importing site identity, and linked-player access requires that identity and
the current `soccer` grant. A site-session timeout hides the import until that
identity signs in again. Site sign-out, Clear import, and expiry remove it, as
does a Soccer visit by another signed-in site account. The import also writes a
guard cookie that its payload must match. Only an import writes the guard, so
once sign-out or Clear import deletes both, a Soccer response still in flight
cannot restore usable access. An anonymous Team ID
lookup saves its choices only in a cookie that ends with the browser session,
and never replaces a retained import.

The management portal uses the invited site session and its current
`management` grant. With site identity configured, it can list EC2 instances,
request start, stop, and restart actions, and load CloudWatch metrics and logs.

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

Without a valid key, JWT import is disabled. Import and linked-player actions
also require a signed-in site account with the current `soccer` grant. Imported
access is bound to the validated Cognito issuer and subject; old imports
without an owner cannot be reused. Team ID lookup and ICS download remain
available without sign-in. `LPS_API_BASE_URL` can override the upstream API
for local testing. The application accepts HTTPS endpoints and loopback HTTP
endpoints.

### Google Calendar

Google Calendar actions require the current site `soccer` grant, a separate
Google OAuth connection, and:

- `CLIENT_ID_KEY`
- `CLIENT_SECRET_KEY`
- `GOOGLE_CONNECTION_TABLE_NAME`

Register each application URL ending in `/soccer` as an authorized redirect
URI in the Google OAuth client. Local DynamoDB access uses the standard AWS
credential chain.

Calendar consent is separate from site sign-in and requests `openid` and
`email` alongside the Calendar scopes. **Connect Google Calendar** passes the
site sign-in email to Google as `login_hint`; **Use another Google account**
opens Google's account chooser instead. The callback reads the consenting
account from Google's UserInfo endpoint, stores it with the connection, and the
page shows that address; the site email is never treated as confirmation of
the Calendar owner. A connection without a verified Google account is not
used: the page marks its owner's card **Reconnect needed** and offers to
reconnect or disconnect it.

The connection belongs to the site owner who consented. Site sign-out keeps it
for that owner's next sign-in in the same browser, while a signed-out visitor
or another site owner cannot see or use it. Each site owner's connection has
its own cookie, named from the owner's Cognito issuer and subject, so owners
who share a browser keep separate connections. **Disconnect** deletes the stored
connection and its cookie, so the site keeps no token for it. It does not
revoke the grant at Google, because Google withdraws a grant for the whole
Google account and OAuth client: that would also disconnect the same account's
connections in other browsers, for other site owners, and in other
environments. **Change Google account** and **Switch to site account** replace
the stored connection the same way, leaving the previous account's grant at
Google. To withdraw the site's Calendar access entirely, remove the app from
the Google Account's third-party access settings.

The encrypted browser cookie is the Soccer workflow source of truth. Imported
access and Google OAuth state are bound to the validated Cognito issuer and
subject. Google connection records use the same owner coordinates and retain
the consenting Google account identity. Connections saved before #93 sit
behind the single browser-wide `google_connection` cookie and are never used:
an old ownerless connection, or an owner's connection without a verified Google
account. A granted visitor holding that cookie deletes an ownerless connection
or their own by disconnecting or reconnecting; another owner's stays. When
`SOCCER_SESSION_TABLE_NAME` is set, the application also writes an owner-bound
import baseline containing the username and discovered players; it does not
restore workflow state from that table.

### Site sign-in

Site sign-in uses Cognito's Google federation and a separate encrypted session
cookie. It does not depend on the management portal's AWS clients: if they
cannot be created, sign-in still works and the portal routes stay unregistered. Set `SITE_SESSION_KEY` (a 64-character lowercase hex key),
`SITE_COGNITO_DOMAIN`, `SITE_COGNITO_ISSUER`, `SITE_COGNITO_CLIENT_ID`,
`SITE_COGNITO_REDIRECT_URI`, `SITE_COGNITO_LOGOUT_URI`, and
`SITE_INVITATIONS_JSON` in each environment. The callback URL must end in
`/auth/callback`, and the HTTPS logout return URL must end in `/sign-in`. A
registered HTTP loopback callback also needs `SITE_ALLOW_LOCAL_CALLBACK=true`.

`SITE_INVITATIONS_JSON` is a reviewed JSON object mapping normalized, verified
email addresses to page grants. The initial owner entry is
`{"craigdevjohnson@gmail.com":["soccer","management"]}`. Each environment
must supply its own map and Cognito user pool; the app has no default invite.
An invited address may have an empty grant list. Supported grants are `soccer`
and `management`. Changes require updated environment configuration and a
deployment. The app reads that current map for every request; grants are never
stored in the browser cookie. The encrypted session retains the validated
Cognito issuer, subject, email, and bounded expiry.

When site sign-in is configured, shared navigation links to `GET /sign-in`
with a local return path; without complete configuration it shows no sign-in
entry. The landing page starts Google sign-in only on `POST /sign-in`; Cognito
returns to `GET /auth/callback`. Return paths must be local, and a return path
to the callback itself falls back to `/`. `POST /sign-out` clears the session,
pending OAuth state, and imported LPS access before ending the Cognito
managed-login journey. A
denied, uninvited identity is offered **Use a different account**, which uses
that sign-out path so the next attempt does not reuse the same managed login.
A stale callback leaves an existing valid session in place. Public
pages stay available when site sign-in is disabled, rejected, or expired.
Site cookies are host-only, so the callback URL's host is the canonical site
host: while site sign-in is configured, every request for its `www.` alias
receives a `308` to the same path and query on that host.
Responses from the sign-in routes and pages rendered for a signed-in account
send `Cache-Control: no-store`; anonymous portfolio pages keep their existing
cache headers. The site auth routes and configuration are offline application
support; Cognito resources, Lambda runtime variables, and live activation
require separate review.

The separate, unapplied development and production Cognito roots, session
parameter paths, and Lambda handoff are described in
[site identity configuration](./docs/deployment/site-identity.md). Run
`task cognito-site-ci` for offline identity and infrastructure checks.

The session is an encrypted bearer cookie that lasts at most one hour, or less
when the Cognito ID token expires sooner. Sign-out clears that cookie and ends
the Cognito managed login, so the browser loses restricted access. The server
keeps no session state, so a previously copied cookie can still be replayed
until that expiry. This falls short of invalidating the session everywhere at
sign-out; that would require shared server-side session or revocation state.

### EC2 management portal

Configure the shared `SITE_*` identity settings above and give an invited,
verified account the `management` grant in `SITE_INVITATIONS_JSON`. Each
`/mgmt` page and action reads the site session and checks the grant from the
current configuration. A valid site session without that grant receives a clear
denial; a signed-out visitor is sent to `/sign-in` and then back to `/mgmt`.
The portal's sign-out button uses the same `/sign-out` route as shared
navigation. The old `/login`, `/callback`, and `/logout` routes and
`mgmt_session` cookie do not authorize the portal. `MGMT_AWS_REGION` defaults
to `us-east-1`.

The shared site Cognito callback and logout URL must be registered before
configuring the portal in an environment. Existing management-only Cognito
registration and `MGMT_*` identity settings do not activate this flow. The
application ignores `MGMT_SESSION_KEY`, `MGMT_COGNITO_*`,
`MGMT_ALLOWED_EMAILS`, and `MGMT_ALLOW_LOCAL_CALLBACK`, and logs one warning
naming any that remain set.

The runtime AWS identity needs these actions:

- `ec2:DescribeInstances`
- `ec2:StartInstances`
- `ec2:StopInstances`
- `cloudwatch:GetMetricStatistics`
- `logs:FilterLogEvents`

The development dashboard enables start, stop and restart only for instances
tagged `PortfolioManagement=dev`, subject to their lifecycle state. Other
instances remain visible with read-only metrics and logs; IAM enforces actions.
The deployed runtime role grants only `ec2:DescribeInstances` and
`cloudwatch:GetMetricStatistics` (D22), so start, stop, restart, and log reads
report a failure there even for an account with the `management` grant.

For a mock review that constructs no Cognito or AWS clients, run:

```bash
task portal-preview
```

Then open the `/mgmt` URL printed at startup (port `8080` by default). Preview
mode requires a loopback listener and is unavailable in the Lambda handler.
It also serves `/__preview/account/signed-out` and
`/__preview/account/signed-in`, which render the About page with each shared
navigation account state; `/__preview/account/soccer-signed-out`,
`/__preview/account/soccer-ungranted`, and `/__preview/account/soccer-granted`,
which render an inert Soccer page for each `soccer` grant state; and
`/__preview/portal/error?fixture=access-denied`, the denial a signed-in account
without the `management` grant receives. Preview mode never enables real site
sign-in.

## Soccer import flow

Anyone can enter Team IDs on `/soccer`, fetch a schedule, and download an ICS
file. Linked-player import requires a site account with the current `soccer`
grant:

1. Sign in to the site through `/sign-in` with an invited account holding the
   `soccer` grant, then sign in to Let's Play Soccer in a browser.
2. Copy the bearer JWT from the authenticated LPS session. The helper extension
   in `chrome-extension/` can capture and copy it.
3. Import the JWT on `/soccer`.
4. The server calls `/users/check`, filters deleted players, and shows the
   linked players.
5. Select players and teams, then fetch schedules.
6. Download an ICS file or separately connect Google Calendar to use its actions.

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
├── infra/                  Shared ECR, DynamoDB, and IAM resources
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
│   ├── siteauth/           Site Cognito sign-in and encrypted sessions
│   ├── siteidentity/       Request principal and current page grants
│   ├── soccer/             Soccer auth and schedule handlers
│   └── soccerarchive/      Durable LPS team history and on-demand refresh worker
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

Site account routes remain registered even when site identity is not
configured; the landing page then reports that sign-in is unavailable:

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/sign-in` | Signed-out landing and local return destination |
| `POST` | `/sign-in` | Start Google-federated Cognito sign-in |
| `GET` | `/auth/callback` | Complete site sign-in |
| `POST` | `/sign-out` | Clear site session and imported LPS access; end managed login |

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

The Soccer page, Team ID `/soccer/fetch`, and Team ID `/soccer/download` are
public. The import, logout, discover-teams, and Google routes require the
current `soccer` grant. Fetch and download requests using linked-player IDs or
the imported team-selection form also require that grant. The Google OAuth
callback is served on `/soccer` and requires the same grant. Protected routes
return `401` without a site session and `403` when the account lacks the grant.
An htmx request from an open Soccer page receives the same status with an
explanation swapped into that control's target, so an expired session or a
revoked grant is visible where the visitor acted.
An environment without complete site sign-in configuration has no signed-in
visitors, so its private Soccer actions stay unavailable and the page says so.

Portal routes are registered with valid `SITE_*` identity configuration or
local preview mode. They include `/mgmt` and the instance action, metrics, and
logs paths under `/mgmt/instances/{id}/`.

## Chrome extension

`chrome-extension/` contains a Manifest V3 extension for the Soccer import
flow. Its service worker captures Let's Play Soccer bearer credentials from an
authenticated browser session. The content script fills the import field, and
the popup shows capture status and copy controls.

Load it from `chrome://extensions/` with Developer mode and "Load unpacked."
Select the `chrome-extension/` directory.

## Deployment

The portfolio runs on AWS Lambda behind an API Gateway HTTP API in the
workloads AWS account, us-west-2, with prod at `craigdevjohnson.com` and dev at
`dev.craigdevjohnson.com`. The OpenTofu roots live under `infra/lambda/`. A
merge to `main` builds one image, deploys it to dev, and plans prod; Craig
approves the `production` GitHub Environment to apply it. The legacy `infra/`
root is retired. Dated designs and plans under `docs/superpowers/` are
historical records rather than operator instructions.

The Lambda timeout is 29 seconds. The Google add and result-sync handlers
reserve 24 seconds of that window, which leaves five seconds outside their
application work budget.

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
that expected value proves the identity of those artifacts. Direct builds that
omit `BUILD_REVISION` may report `development`,
which is not immutable provenance proof. `task test-images` verifies the image
contracts. These tasks do not push an image, apply infrastructure, or deploy a
service.

Read [`DEPLOY-INSTRUCTIONS.md`](./DEPLOY-INSTRUCTIONS.md) for accounts,
approvals, the release workflow and rollback, and the
[Cloudflare runbook](./docs/deployment/cloudflare-dns.md) for DNS records.
Lambda runtime details are in
[`docs/deployment/aws-lambda-api-gateway.md`](./docs/deployment/aws-lambda-api-gateway.md).
