# Portfolio Codebase Instructions

Always use Context7 MCP when you need library or API documentation, code generation, setup steps, or configuration details.

## Source of Truth Order

Use these files in this order when instructions conflict:

1. `Taskfile.yaml` for build, test, formatting, lint, and dev commands.
2. `cmd/server/main.go` and `internal/app/*.go` for the actual application flow.
3. `README.md` for high-level architecture and local usage.
4. `DEPLOY-INSTRUCTIONS.md` plus `infra/*.tf` for deployment and infrastructure.
5. `docs/archive/PRD-2026-internal-app-refactor.md` for historical refactor context.

Link to those docs instead of copying long procedures into new instructions or PR notes.

## Architecture

- Go 1.26.3 server-rendered app with Templ and HTMX.
- `cmd/server/main.go` is intentionally thin and delegates to `internal/app` for startup and route wiring.
- `internal/app` is a thin routing and dependency-injection layer (~260 lines). It constructs the `App` struct, wires domain packages, registers routes, and starts the HTTP server. It does not contain any business logic.
- `internal/config` holds env parsing, feature toggles (`LoginEnabled`, `GoogleEnabled`), and shared constants.
- `internal/soccer` contains soccer page rendering, JWT import, player discovery, session management, schedule fetch, ICS download, and subscribe handlers.
- `internal/google` contains Google OAuth connect/callback/disconnect, Calendar API event sync, DynamoDB connection store, and token management.
- `internal/lps` contains the LPS API client, schedule resolver, JSON decode helpers, and error classification.
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
- `task fmt` runs `golangci-lint fmt --config .golangci.toml`.
- `task lint` runs Go lint autofixes.

Prefer the `task` recipes over ad hoc commands. `go fmt ./...` is not this repo's primary formatter entry point.

## Templ Workflow

- Edit `.templ` source files, not generated `*_templ.go` files.
- Run `task generate` after any `.templ` change unless another command already does it.
- Tailwind source lives under `cmd/web/tailwind/`; compile it with `task tailwind-build` or `task tailwind-watch`.
- CSS-first component and page rules live in `cmd/web/tailwind/components.css` and `cmd/web/tailwind/soccer.css`; they are compiled into the generated Tailwind output.
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

## Environment And Deployment Notes

- `LPS_SESSION_KEY` is the critical runtime secret for soccer auth.
- `LPS_API_BASE_URL` is optional and overrides the upstream API base URL.
- Google Calendar integration also requires `CLIENT_ID_KEY` and `CLIENT_SECRET_KEY`. In App Runner, `infra/*.tf` supplies `GOOGLE_CONNECTION_TABLE_NAME`; local runs still need it set. See `DEPLOY-INSTRUCTIONS.md` for deployment details.
- `docker-compose.yml` expects local env configuration and passes through `LPS_SESSION_KEY`.
- The runtime server listens on port `8080`.
- Use `infra/*.tf` as the source of truth for actual Terraform or OpenTofu defaults; deployment prose may lag behind.

## Known Drift To Avoid

- Older docs still mention manual player-ID import. Ignore that and follow the current code.
- Some docs still describe older Go versions or `go fmt`. The repo is on Go 1.26.3 and uses `task fmt` via golangci-lint.
- References to `cmd/web/tailwind/legacy/` or `tailwindcss/legacy/` are stale — that directory no longer exists.
- References to `PROGRESS.md` or `PRD.md` at the repo root are stale — those files do not exist.
- Historical workspace tasks may reference generated Templ files or old commit flows. Follow `Taskfile.yaml`, not old one-off commands.

## When Updating This Repo

- Keep changes focused and minimal.
- Reuse the existing structs in `types/types.go` before adding new ones.
- Preserve server-rendered and HTMX-first patterns instead of introducing SPA-style client state.
- Validate the smallest relevant surface after changes: `task generate` for Templ edits, `task test` for behavior changes, `task build` before finishing when practical.
