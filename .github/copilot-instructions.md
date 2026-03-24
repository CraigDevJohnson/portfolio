# Portfolio Codebase Instructions

Always use Context7 MCP when you need library or API documentation, code generation, setup steps, or configuration details.

## Source of Truth Order

Use these files in this order when instructions conflict:

1. `PRD.md` and `PROGRESS.md` for the current soccer product behavior and recent task completion.
2. `justfile` for build, test, formatting, lint, and dev commands.
3. `main.go` and `main_test.go` for the actual application flow.
4. `README.md` for high-level architecture and local usage.
5. `DEPLOY-INSTRUCTIONS.md` plus `infra/*.tf` for deployment and infrastructure.

Link to those docs instead of copying long procedures into new instructions or PR notes.

## Architecture

- Go 1.26.1 server-rendered app with Templ and HTMX.
- `main.go` is intentionally the application center: routes, handlers, LPS client code, session helpers, and hardcoded portfolio data live there by design.
- `components/layouts` contains layout wrappers, `components/pages` contains full pages, and `components/partials` contains reusable fragments and HTMX swap targets.
- `types/types.go` holds shared typed models used across handlers and templates.
- `static/js/main.js` owns client-side behavior such as the mobile nav, skills interactions, and soccer modal handling.

Do not refactor this repo into extra packages unless the user explicitly asks for that. The monolithic layout is intentional.

## Daily Commands

- `just generate` regenerates Templ output.
- `just build` regenerates Templ and builds `portfolio-server`.
- `just run` builds and runs locally.
- `just dev` runs `air` for hot reload.
- `just test` runs `go test -v ./...`.
- `just vet` runs `go vet ./...`.
- `just fmt` runs `golangci-lint fmt --config .golangci.toml`.
- `just lint` runs Go lint autofixes and CSS stylelint autofixes.

Prefer the `just` recipes over ad hoc commands. `go fmt ./...` is not this repo's primary formatter entry point.

## Templ Workflow

- Edit `.templ` source files, not generated `*_templ.go` files.
- Run `just generate` after any `.templ` change unless another command already does it.
- Per-page CSS follows `static/css/{page}.css` and is wired through the base layout's `Page` prop.
- Preserve existing HTMX patterns: full pages render layout wrappers, fragment endpoints return partial HTML only.

Useful exemplars:

- `components/layouts/base.templ` for layout structure and asset loading.
- `components/pages/soccer.templ` for a full page that mixes static content with HTMX fragments.
- `components/partials/soccer_login_state.templ` and `components/partials/soccer_table_fragment.templ` for fragment patterns.
- `main_test.go` for handler tests using `httptest` and stub upstream servers.

## Soccer Auth Flow

The current soccer flow is JWT import with server-side player discovery, not manual player-ID import and not a true OAuth popup flow.

- `LPS_SESSION_KEY` is required to enable soccer login. It must be a 64-character hex string.
- Users paste a bearer JWT from their signed-in Let's Play Soccer browser session.
- The server calls `/users/check`, discovers linked players, filters deleted players, stores the session in an encrypted cookie, and renders the player selector.
- Do not instruct users to manually copy or paste player IDs. That guidance is stale.
- If you need current product intent or acceptance criteria for soccer, read `PRD.md` and `PROGRESS.md` first.

Relevant handlers and components:

- `soccerSessionHandler`, `soccerImportHandler`, `soccerLogoutHandler`, `fetchSchedulesHandler`, and `downloadICSHandler` in `main.go`.
- `components/pages/soccer.templ`.
- `components/partials/soccer_login_modal.templ`.
- `components/partials/soccer_login_state.templ`.
- `components/partials/soccer_player_select.templ`.

## Environment And Deployment Notes

- `LPS_SESSION_KEY` is the critical runtime secret for soccer auth.
- `LPS_API_BASE_URL` is optional and overrides the upstream API base URL.
- Google Calendar integration also requires `CLIENT_ID_KEY`, `CLIENT_SECRET_KEY`, and `GOOGLE_CONNECTION_TABLE_NAME`; see `DEPLOY-INSTRUCTIONS.md` for deployment details.
- `docker-compose.yml` expects local env configuration and passes through `LPS_SESSION_KEY`.
- The runtime server listens on port `8080`.
- Use `infra/*.tf` as the source of truth for actual Terraform or OpenTofu defaults; deployment prose may lag behind.

## Known Drift To Avoid

- Older docs still mention manual player-ID import. Ignore that and follow the current code and PRD.
- Some docs still describe Go 1.23+ and older formatter guidance. The repo is on Go 1.26.1 and uses `just fmt`.
- Historical workspace tasks may reference generated Templ files or old commit flows. Follow `justfile`, not old one-off tasks, unless the user explicitly asks otherwise.

## When Updating This Repo

- Keep changes focused and minimal.
- Reuse the existing structs in `types/types.go` before adding new ones.
- Preserve server-rendered and HTMX-first patterns instead of introducing SPA-style client state.
- Validate the smallest relevant surface after changes: `just generate` for Templ edits, `just test` for behavior changes, `just build` before finishing when practical.
