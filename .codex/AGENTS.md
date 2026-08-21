# Portfolio Codebase Instructions

Always use Context7 MCP when you need library or API documentation, code generation, setup steps, or configuration details.

## Source of Truth Order

Use these files in this order when instructions conflict:

1. `Taskfile.yaml` for build, test, formatting, lint, and dev commands.
2. `cmd/server/main.go` and `internal/app/*.go` for the actual application flow.
3. `README.md` for high-level architecture and local usage.
4. `DEPLOY-INSTRUCTIONS.md` plus `infra/*.tf` for deployment and infrastructure.

Link to those docs instead of copying long procedures into new instructions or PR notes.

## Architecture

- Go 1.26.6 server-rendered app with Templ and HTMX.
- `cmd/server/main.go` is intentionally thin and delegates to `internal/app` for startup and route wiring.
- `internal/app` is the routing and dependency-injection layer. It constructs the `App` struct, wires domain packages, registers routes, and starts the HTTP server.
- `internal/config` holds env parsing, feature toggles (`LoginEnabled`, `GoogleEnabled`), and shared constants.
- `internal/soccer` contains soccer page rendering, JWT import, player discovery, session management, schedule fetch, and ICS download handlers.
- `internal/google` contains Google OAuth connect/callback/disconnect, Calendar API event sync, DynamoDB connection store, and token management.
- `internal/portal` contains Cognito OAuth + PKCE authentication, encrypted portal
  sessions, EC2 instance actions, CloudWatch metrics, and CloudWatch Logs handlers.
- `internal/lps` contains the LPS API client, typed schedule resolver, player-response decoder, and error classification.
- `internal/schedule` contains game normalization, merge/sort/dedup, time parsing, and ICS building.
- `internal/session` contains AES-GCM cookie encryption and login rate limiting.
- `internal/httpx` contains client IP detection, HTTPS detection, and secure cookie builder.
- `internal/portfolio` contains portfolio page handlers and static data.
- `types/types.go` holds shared typed models used across handlers and templates.
- `cmd/web/layouts` contains layout wrappers, `cmd/web/pages` contains full pages, and `cmd/web/partials` contains reusable fragments and HTMX swap targets.
- `cmd/web/static/js/main.js` owns client-side behavior such as the mobile nav, skills interactions, and soccer modal handling.

Dependencies flow downward from `internal/app` through domain packages to `types`. No circular imports exist. `internal/soccer` and `internal/google` communicate through interfaces (`soccer.GoogleHooks` and `google.SoccerBridge`) wired in `internal/app`.

## Daily Commands

- `task generate` regenerates Templ output.
- `task tailwind-build` compiles `cmd/web/tailwind/app.css` to `/static/css/tailwind.css` using the pinned standalone Tailwind CLI.
- `task build` regenerates Templ and builds `portfolio-server`.
- `task run` builds and runs locally.
- `task dev` runs `air` for hot reload.
- `task tailwind-watch` rebuilds the generated Tailwind stylesheet while you edit Tailwind source files.
- `task test` runs `go test -v ./...`.
- `task vet` runs `go vet ./...`.
- `task fmt` runs `golangci-lint fmt`.
- `task lint` formats first, then runs `golangci-lint run`.

Prefer the `task` recipes over ad hoc commands. `go fmt ./...` is not this repo's primary formatter entry point.

## Templ Workflow

- Edit `.templ` source files, not generated `*_templ.go` files.
- Run `task generate` after any `.templ` change unless another command already does it.
- Tailwind source lives under `cmd/web/tailwind/`; compile it with `task tailwind-build` or `task tailwind-watch`.
- CSS-first component and page rules live in `cmd/web/tailwind/components.css`, `cmd/web/tailwind/pages/`, `cmd/web/tailwind/soccer.css`, and `cmd/web/tailwind/portal.css`; they compile into the generated Tailwind output.
- Preserve existing HTMX patterns: full pages render layout wrappers, fragment endpoints return partial HTML only.

Useful exemplars:

- `cmd/web/layouts/base.templ` for layout structure and asset loading.
- `cmd/web/pages/soccer.templ` for a full page that mixes static content with HTMX fragments.
- `cmd/web/partials/soccer_login_state.templ` and `cmd/web/partials/soccer_table_fragment.templ` for fragment patterns.
- `internal/app/*_test.go` for handler tests using `httptest` and stub upstream servers.

## Soccer Auth Flow

The current soccer flow is JWT import with server-side player discovery, not manual player-ID import and not a true OAuth popup flow.

- `LPS_SESSION_KEY` is required to enable soccer login. It must be a 64-character hex string.
- Users paste a bearer JWT from their signed-in Let's Play Soccer browser session.
- The server calls `/users/check`, discovers linked players, filters deleted players, stores the session in an encrypted cookie, and renders the player selector.
- Do not instruct users to manually copy or paste player IDs. That guidance is stale.
- If you need current product intent or acceptance criteria for soccer, read the current code first.

Relevant handlers and components:

- `SessionHandler`, `ImportHandler`, and `LogoutHandler` in `internal/soccer/auth.go`.
- `FetchSchedulesHandler` and `DownloadICSHandler` in `internal/soccer/schedule.go`.
- `SoccerPage` in `internal/soccer/page.go`.
- `cmd/web/pages/soccer.templ`.
- `cmd/web/partials/soccer_login_modal.templ`.
- `cmd/web/partials/soccer_login_state.templ`.
- `cmd/web/partials/soccer_player_select.templ`.

## Portal Auth Flow

The optional portal uses Cognito's OAuth 2.0 authorization-code flow with PKCE.
It validates the OAuth state, exchanges the authorization code, verifies the ID
token through Cognito JWKS, and stores only an encrypted application session.
Protected `/mgmt` routes redirect unauthenticated requests to `/login`.

Relevant code is in `internal/portal/` and the dashboard templates are
`cmd/web/pages/portal_mgmt.templ` and `cmd/web/pages/portal_error.templ`.

## Environment And Deployment Notes

- `LPS_SESSION_KEY` is the critical runtime secret for soccer auth.
- `LPS_API_BASE_URL` is optional and overrides the upstream API base URL.
- Google Calendar integration also requires `CLIENT_ID_KEY` and `CLIENT_SECRET_KEY`. In App Runner, `infra/*.tf` supplies `GOOGLE_CONNECTION_TABLE_NAME`; local runs still need it set. See `DEPLOY-INSTRUCTIONS.md` for deployment details.
- The EC2 portal requires `MGMT_SESSION_KEY`, `MGMT_COGNITO_DOMAIN`, and
  `MGMT_COGNITO_CLIENT_ID`. `MGMT_COGNITO_REDIRECT_URI` configures the OAuth
  callback, `MGMT_COGNITO_LOGOUT_URI` is optional, and `MGMT_AWS_REGION`
  defaults to `us-east-1`.
- Terraform does not currently provision Cognito or portal-specific IAM
  permissions; follow `DEPLOY-INSTRUCTIONS.md` for least-privilege setup.
- `docker-compose.yml` passes the documented LPS, Google, Soccer store, portal, logging, and local AWS variables from `.env`.
- The runtime server listens on port `8080`.
- Use `infra/*.tf` as the source of truth for actual Terraform or OpenTofu defaults; deployment prose may lag behind.

## Known Drift To Avoid

- Do not reintroduce manual player-ID import. The current flow imports a JWT and discovers linked players.
- The repo is on Go 1.26.6 and uses `task fmt` via golangci-lint with the root `.golangci.toml`.
- Do not reference `cmd/web/tailwind/legacy/` or `tailwindcss/legacy/`; neither directory exists.
- References to `PROGRESS.md` or `PRD.md` at the repo root are stale — those files do not exist.
- Historical workspace tasks may reference generated Templ files or old commit flows. Follow `Taskfile.yaml`, not old one-off commands.

## When Updating This Repo

- Keep changes focused and minimal.
- Reuse the existing structs in `types/types.go` before adding new ones.
- Preserve server-rendered and HTMX-first patterns instead of introducing SPA-style client state.
- Validate the smallest relevant surface after changes: `task generate` for Templ edits, `task test` for behavior changes, `task build` before finishing when practical.
