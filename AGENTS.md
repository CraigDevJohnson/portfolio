<!-- markdownlint-disable MD013 -->
# Portfolio — Go + Templ + HTMX

## Source of truth order

1. `Taskfile.yaml` for all commands
2. `cmd/server/main.go` + `internal/app/` for app wiring
3. `README.md` for architecture / usage
4. `DEPLOY-INSTRUCTIONS.md` + `infra/*.tf` for deployment

## Commands

- `task generate` — regenerate Templ output (always after editing `.templ` files)
- `task tailwind-build` — compile `cmd/web/tailwind/app.css` → `cmd/web/static/css/tailwind.css`
- `task build` — generate + tailwind-build + `go build -o portfolio-server ./cmd/server`
- `task run` — build + run on `127.0.0.1:8080` by default; `HOST`, `PORT`, and `APP_BIND_ALL` can override the listener
- `task dev` — `air` hot-reload (does NOT regenerate Templ)
- `task tailwind-watch` — rebuild Tailwind on file changes
- `task test` — generate + `go test -v ./...`
- `task vet` — generate + `go vet ./...`
- `task fmt` — `golangci-lint fmt` (NOT `go fmt ./...`)
- `task lint` — generate, format, then `golangci-lint run`
- `task ci` — clean → generate → fmt → vet → lint → test → build
- `task deploy` — build+push App Runner and Lambda images + `tofu apply`
- `task redeploy` — push image + `aws apprunner start-deployment`
- `task deploy-lambda` / `task redeploy-lambda` — Lambda variant

## Architecture

- `cmd/server/main.go` is ~10 lines; all wiring in `internal/app/`
- `.templ` files in `cmd/web/{layouts,pages,partials}` are source; `*_templ.go` is generated and gitignored
- Tailwind source: `cmd/web/tailwind/*.css` and `cmd/web/tailwind/pages/*.css`; generated output: `cmd/web/static/css/tailwind.css` (gitignored)
- Docker image built from `Dockerfile` (App Runner) or `Dockerfile.lambda`
- `internal/portal` contains the optional Cognito-authenticated EC2 management
  portal, including instance actions, CloudWatch metrics, and CloudWatch Logs.
- See `.github/instructions/templ.instructions.md` and `.github/instructions/tailwind.instructions.md` for detailed authoring rules

## Gotchas

- `task dev` (air) watches only `.go` files — run `task generate` manually after `.templ` edits
- `LPS_SESSION_KEY` must be a 64-char hex string; without it, soccer auth is disabled
- Google Calendar also needs `CLIENT_ID_KEY`, `CLIENT_SECRET_KEY`, and `GOOGLE_CONNECTION_TABLE_NAME`
- The EC2 portal requires `MGMT_SESSION_KEY`, `MGMT_COGNITO_DOMAIN`, and
  `MGMT_COGNITO_CLIENT_ID`; an invalid or incomplete configuration disables it
  without affecting portfolio or soccer routes.
- `MGMT_COGNITO_REDIRECT_URI` must be a registered OAuth callback URI for sign-in;
  `MGMT_COGNITO_LOGOUT_URI` is optional and `MGMT_AWS_REGION` defaults to
  `us-east-1`.
- For Docker Compose: `cp .env.example .env`, set `LPS_SESSION_KEY` (`openssl rand -hex 32`)
- `task fmt` uses `golangci-lint fmt`, not `go fmt ./...` — do not suggest `go fmt`

## Known drift to avoid

- Ignore references to `cmd/web/tailwind/legacy/` — that directory no longer exists
- Ignore references to manual player-ID import — the soccer flow is JWT import + auto-discovery
- Ignore references to `PROGRESS.md` or `PRD.md` at repo root — they don't exist
- `Taskfile.yaml` is authoritative, not ad hoc command suggestions in old docs

## Always run the below when making changes to ensure consistency

- `task generate` after editing `.templ` files
- `task build` after any changes to verify the app compiles successfully
- `task test` after changes to verify tests pass
- `task fmt` after editing Go code to maintain consistent formatting
- `task lint` after changes to catch lint issues
