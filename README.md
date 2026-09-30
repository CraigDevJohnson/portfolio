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

The Soccer planner starts with an ICS or Google Calendar output choice. Team ID
schedule lookup and ICS download stay public. An invited site account with the
current `soccer` grant can also:

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

Durable Soccer history collection is off in every environment until the issue
#80 activation review wires the history archive. Once wired, the import dialog
says that linked-player history is kept indefinitely, and only an import sent
from that dialog collects it: every linked player's team and LPS season
memberships, including unselected players, stored under the importing site
identity with their source and observation time, never the JWT and without a
session TTL. Their teams are enrolled for refresh without selecting any
planner games. A Team ID lookup never records player membership. An import
that collects history gives its LPS lookups, the account and then each linked
player's teams one at a time in the order LPS lists them, one 11-second
deadline. A player LPS has not listed by then is skipped: the import still
completes, keeps the other players' history, and names the player, whose
teams a later import may collect. Lookups keep that order on every import, so
an LPS that stays slow can skip the same last players each time.

With history wired, the LPS card of an imported, granted owner also offers to
remove each linked player's kept data. `POST /soccer/players/remove` accepts
only this site's own pages, the current site session and `soccer` grant, and
the same owner's unexpired import, and it confirms the player ID with a fresh
LPS lookup through that import; a name, a stale player list, or another
owner's import is refused. It deletes the player's identity, every owner link,
and every current and past team-season membership from the history table,
including those recorded through other site accounts, and keeps team, season,
facility, and game facts. It does not reach the import baseline in
`SOCCER_SESSION_TABLE_NAME`: each import's record keeps its players beside the
importing owner until DynamoDB deletes it some time after its TTL, at most 12
hours after that import. Nor does it reach point-in-time backups, which keep
removed items restorable for up to 35 days wherever `enable_pitr` is on. The
reply names only the removed player and says what removal does not reach. A
completed removal clears the browser's import, so only a later disclosed
import collects the player again; a failed delete keeps the import for a
retry.

Enrollment has a reviewed capacity. Teams already enrolled keep their daily
refresh; a new team past capacity is refused, the visitor sees why, and each
refusal is logged once. A refused team that a granted player import found
raises an alarm; a refused anonymous Team ID is only counted. A granted import still completes when
some of its new teams are refused: it keeps the other teams' history and names
the teams it could not add. Some slots are reserved for teams a granted player
import finds, so anonymous Team IDs cannot fill them. A separate scheduled
worker attempts every enrolled team at each daily run, dormant and entered
teams included, whenever they were last fetched, within a per-run request,
retry, pacing and time budget. It reports each team it refreshed or failed,
and each due team it left for the next run and why. Without every reviewed
limit there is no collection and no worker; see
[infra/lambda/README.md](infra/lambda/README.md#soccer-history-collection-and-daily-refresh).

The management portal uses the invited site session and its current
`management` grant. With site identity configured, it lists EC2 instances and
loads their CloudWatch metrics. Its start, stop, restart and log views remain
in the code, but the deployed runtime role does not allow them (D22).

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

A new connection starts with the primary calendar as its destination, with no
separate confirmation; a same-account reconnect keeps the chosen calendar (see
below). A visitor can save another calendar the connected account can write;
events already added stay in their original calendar, and later Adds go to the
new one. If the chosen calendar disappears or stops accepting this account's
events, Add and result sync pause and the card asks for a new choice; they
never fall back to primary, and the calendar returning does not resume them on
its own. An Add that loses its destination
partway reports the games it already added, which stay in that calendar. When Google rejects the connection
itself, the site removes it and asks the visitor to connect again. Any other
refusal, such as a Google usage limit, keeps the connection: the card still
names the connected account and asks the visitor to retry.

Consenting again as the same Google account while the owner's connection is
still stored, for example through **Change Google account**, keeps the chosen
calendar as the destination if that account can write it, and resumes writes
that were paused for it. A consent starts at primary when it uses a different
Google account or when the chosen calendar is gone or read-only. Connecting
after **Disconnect**, or after Google rejected the connection, also starts at
primary, because the site no longer holds the earlier connection.

Viewing the page and fetching schedules write no Google events. **Add selected
to calendar** writes only the selected upcoming games; one LPS has not given a
start time yet is listed as upcoming, and Add skips and reports it. Each event
it writes carries this provenance, which result sync relies on to recognize
the site's own events:

- The Google event ID is the game's canonical ID: the LPS game ID, or a hash
  of the game's schedule fields when LPS gives none.
- The private extended property `game_id` repeats that ID.
- The private extended property `portfolio_app=soccer` marks the event as added
  by this site, unlike an event imported from an .ics file.
- The event source is titled `Soccer Schedule` and links to the site's
  `/soccer` page.

Events added before the `portfolio_app` marker existed carry the other three.
Result sync recognizes such an event when its event ID and `game_id` both
equal the game's ID and its source is the site's `Soccer Schedule` page, since
Add cannot re-add a past game to mark it.

Repeating Add in the same calendar finds the existing event by event ID or
`game_id` and updates it instead of inserting another. When Google refuses a
change to one existing event, such as an invitation copy another organizer
owns, Add skips and reports that game and keeps the destination; only a
refusal that names the calendar pauses writes.

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
email addresses to page grants. The initial development owner entry is
`{"craigdevjohnson@gmail.com":["soccer","management"]}`; production's is
`{"craigdevjohnson@gmail.com":["soccer"]}`, and the production root refuses a
`management` grant because production has no portal EC2 grants. Each
environment must supply its own map and Cognito user pool; the app has no
default invite.
An invited address may have an empty grant list. Supported grants are `soccer`
and `management`. Changes require updated environment configuration and a
deployment. The app reads that current map for every request; grants are never
stored in the browser cookie. The encrypted session retains the validated
Cognito issuer, subject, email, and bounded expiry.

When site sign-in is configured, shared navigation links to `GET /sign-in`
with a local return path; without complete configuration it shows no sign-in
entry. The landing page starts Google sign-in only on `POST /sign-in`; Cognito
returns to `GET /auth/callback`. Return paths must be local, and a return path
to the callback itself falls back to `/`. `POST /sign-out` clears the session
and pending OAuth state before ending the Cognito managed-login journey. It
also clears features' browser state that depends on the site session: imported
LPS access and pending Google Calendar consent. The owner-bound Google
connection stays, so it is available again when its owner signs back in. Only
the site's own pages may submit `POST /sign-in` and `POST /sign-out`: a form
another site submits receives `403` and changes no cookies. A
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

The deployed runtime role grants the portal only these actions (D22):

- `ec2:DescribeInstances`
- `cloudwatch:GetMetricStatistics`

It has no EC2 start/stop or CloudWatch Logs grants, so IAM denies the portal's
start, stop and restart actions and its instance log reads; they report a
failure there even for an account with the `management` grant. The planned
Foundry backend replaces direct EC2 control. The dashboard offers start, stop
and restart only for instances tagged `PortfolioManagement=dev`, subject to
their lifecycle state; IAM stays authoritative.

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
which render an inert Soccer page for each `soccer` grant state;
`/__preview/account/soccer-linked`, which lets that browser use the Soccer
page and its LPS routes as the granted preview account against an in-process
fake LPS; and
`/__preview/portal/error?fixture=access-denied`, the denial a signed-in account
without the `management` grant receives. Preview mode never enables real site
sign-in.

## Soccer import flow

Anyone can enter Team IDs on `/soccer`, fetch a schedule, and download an ICS
file. Linked-player import requires a site account with the current `soccer`
grant:

1. Choose "Download an .ics file" on `/soccer`, then sign in to the site
   through `/sign-in` with an invited account holding the `soccer` grant.
2. Sign in to Let's Play Soccer in a browser and copy the bearer JWT from its
   authenticated session. The helper extension in `chrome-extension/` can
   capture and copy it.
3. Import the JWT on `/soccer` to reveal the player and team steps.
4. The server calls `/users/check`, filters deleted players, and shows the
   linked players.
5. Choose linked players and their current teams, then fetch schedules. Shared
   games appear once, with upcoming matches ordered soonest first.
6. Deselect unwanted games, review the selected count, and download the selected
   upcoming games as an ICS file. Google Calendar consent and actions are separate.

A new JWT import or explicit logout clears downstream workflow state. Google
connect, reconnect, disconnect, and calendar selection preserve the current
player and team workflow. An expired import or a token LPS rejects asks for
fresh access; manual Team ID lookup remains available. A linked player whose
teams LPS refuses on its own is named and left out while the other players'
teams still load, and an LPS outage keeps the import and the saved schedule.

With Google output selected, either schedule source also shows scored past
games newest first, including older results returned by LPS. Unscored games
stay out of that list. Review and selection make no Calendar changes; Sync
requires a separate Google connection. ICS mode hides past results and exports
selected upcoming games only. Google output needs the `soccer` grant, so a
public Team ID lookup stays upcoming-only.

**Sync selected results** is update-only: it never inserts a past game or
restores a deleted event. For each selected scored game it searches the chosen
calendar for events carrying the game's private `game_id`, reading every page
Google returns (at most five), and claims only a single live event that also
carries `portfolio_app=soccer`, or the full pre-marker provenance described
above. A deleted copy beside that one live event does not block it. It then
patches that event's description alone, conditional on the ETag the search
returned, and changes only the `Result:` line of the block Add wrote. The
block's lines may be separated by newlines or, once the visitor edits the
description in Google Calendar's editor, by HTML `<br>` line breaks; every
other byte of the description stays as it was. The title, time, location,
reminders, and any notes around the block stay as the visitor left them. A
game two followed teams play keeps its result in the words of the team its
event names.

Sync reports how many results it updated, how many were already current, and
why it skipped the rest: unmatched (no event this site added, such as a game
never added or one imported from an .ics file), deleted (Google may keep only
a deleted event's ID, so Sync also reads the site's event ID when its search
finds nothing), more than one matching event, changed in Google Calendar
while Sync ran, an edited description, or an event Google would not let the
account change. A description counts as edited when the block Add wrote is
gone or reshaped, or a result line holds text in the visitor's own words. A
result line in the site's own format, such as `Result: Win (2-1)`,
`Result: Canceled`, `Result: Final`, or an empty slot, is the site's, and Sync
updates it to the current LPS result even if the visitor typed it. Repeating
Sync patches nothing that is already current. Google refusals follow Add: a
rejected connection asks the visitor to reconnect and reports any results
already written, a usage limit asks for a retry, and a calendar that no longer
accepts writes pauses them for a new choice.

## Source layout

```text
portfolio/
├── chrome-extension/       Chrome helper for Soccer JWT import
├── cmd/
│   ├── lambda/             Lambda adapter and SSM secret resolution
│   ├── server/             HTTP server entry point
│   └── web/                Templ, Tailwind, JavaScript, and static assets
├── docs/deployment/        Runtime-specific deployment notes
├── infra/lambda/           OpenTofu roots for the AWS deployment
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
│   └── soccerarchive/      Durable LPS team history, admission, and refresh workers
└── types/                  Shared application models
```

The source-of-truth order is:

1. `Taskfile.yaml` for commands
2. `cmd/server/main.go` and `internal/app/` for application wiring
3. this README for local usage and architecture
4. `DEPLOY-INSTRUCTIONS.md` and `infra/lambda/` for deployment

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
| `POST` | `/sign-out` | Clear site session, imported LPS access, and pending Google consent; end managed login |

HTMX and form endpoints:

| Method | Path |
| --- | --- |
| `GET` | `/skills/filtered` |
| `GET` | `/skills/detail` |
| `POST` | `/soccer/import` |
| `POST` | `/soccer/players/remove` |
| `POST` | `/soccer/logout` |
| `GET` | `/soccer/history` |
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

`GET /soccer/history?player_id=1001&team_id=4101&season_id=77` is the private
JSON read contract for one LPS team season when a durable archive is wired.
It requires the current site `soccer` grant, a valid same-owner LPS import that
confirms the player ID, and exact current or stored authenticated membership for
the requested team and LPS season. A name match, game date, or entered Team ID
never proves membership; an unproven season returns `403` as unverified while
its stored games remain. When no stored proof covers the season, the read asks
LPS for the player's current teams: a token LPS rejects ends the import like
every other Soccer route (`401`, import cookies cleared), a player LPS denies
returns `403`, and an unavailable LPS returns `502` and keeps the import. The
response includes completed games, a team-relative record labeled as
calculated from numeric scores rather than official standings, season coverage
with its fetch time, and the latest team refresh status or failure. A game is
completed once its kickoff has passed; a game dated later or with no readable
kickoff is neither listed nor counted, even when LPS shows a score. The record
places each side by its LPS team ID; canceled, unscored, and unparseable
results of completed games are listed as unclassified and do not change the
totals. `not_fetched` coverage and a retryable refresh failure remain distinct
from a fetched season with zero games.
Until the separate collection activation, the route returns `503` because no
durable archive is wired into the production server.

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
approves the `production` GitHub Environment to apply it. Dated designs and
plans under `docs/superpowers/` are historical records rather than operator
instructions.

The Lambda timeout is 29 seconds. The Google add and result-sync handlers
reserve 24 seconds of that window, which leaves five seconds outside their
application work budget. A Soccer import that collects linked-player history
keeps to the same 24 seconds: 11 for its LPS lookups, then at most 10 for
history writes and 3 for the import record, and its response's Google
connection check gets only what remains of the 24.

At the Lambda boundary, the adapter derives an HTTPS origin from API Gateway's
typed request context. That context controls secure cookies and generated URLs;
client-supplied host and forwarding headers cannot override it. Cold-start
initialization has an eight-second bound. It reads configured SSM paths in one
decrypted batch, validates the complete response before changing the
environment, constructs the application once, and reuses the proxy on warm
invocations.

Both Google Calendar add and result-sync operations have a 24-second request
budget. If the deadline interrupts a batch, the response includes counts for
completed work and tells the user to retry. Add retries match existing games
and update them instead of duplicating completed inserts; Sync retries leave
results already written unchanged.

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
