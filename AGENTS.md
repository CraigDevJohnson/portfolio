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
- `task run` — build + run on `:8080`
- `task dev` — `air` hot-reload (does NOT regenerate Templ)
- `task tailwind-watch` — rebuild Tailwind on file changes
- `task test` — `go test -v ./...`
- `task vet` — `go vet ./...`
- `task fmt` — `qlty fmt --all` (NOT `go fmt ./...`)
- `task lint` — `qlty check --all`
- `task ci` — clean → fmt → vet → lint → test → build
- `task deploy` — build+push Docker image + `tofu apply`
- `task redeploy` — push image + `aws apprunner start-deployment`
- `task deploy-lambda` / `task redeploy-lambda` — Lambda variant

## Architecture

- `cmd/server/main.go` is ~10 lines; all wiring in `internal/app/`
- `.templ` files in `cmd/web/{layouts,pages,partials}` are source; `*_templ.go` is generated and gitignored
- Tailwind source: `cmd/web/tailwind/*.css`; generated output: `cmd/web/static/css/tailwind.css` (gitignored)
- Docker image built from `Dockerfile` (App Runner) or `Dockerfile.lambda`
- See `.github/instructions/templ.instructions.md` and `tailwind.instructions.md` for detailed authoring rules

## Gotchas

- `task dev` (air) watches only `.go` files — run `task generate` manually after `.templ` edits
- `LPS_SESSION_KEY` must be a 64-char hex string; without it, soccer auth is disabled
- Google Calendar also needs `CLIENT_ID_KEY`, `CLIENT_SECRET_KEY`, and `GOOGLE_CONNECTION_TABLE_NAME`
- For Docker Compose: `cp .env.example .env`, set `LPS_SESSION_KEY` (`openssl rand -hex 32`)
- `task fmt` uses `qlty`, not `go fmt ./...` — do not suggest `go fmt`

## Known drift to avoid

- Ignore references to `cmd/web/tailwind/legacy/` — that directory no longer exists
- Ignore references to manual player-ID import — the soccer flow is JWT import + auto-discovery
- Ignore references to `PROGRESS.md` or `PRD.md` at repo root — they don't exist
- `Taskfile.yaml` is authoritative, not ad hoc command suggestions in old docs
