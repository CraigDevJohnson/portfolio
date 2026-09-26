<!-- markdownlint-disable MD013 -->
# Portfolio — Go + Templ + HTMX

## Source of truth order

1. `Taskfile.yaml` for all commands
2. `cmd/server/main.go` + `internal/app/` for app wiring
3. `README.md` for architecture / usage
4. `DEPLOY-INSTRUCTIONS.md` + `infra/lambda/` for deployment

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
- `task infrastructure-ci` — offline OpenTofu fmt/validate/tests and release-script tests (no AWS access)
- `task lambda-release-push` — build and push one immutable full-SHA Lambda release image
- Account root (CI roles, execution boundary, state bucket): `task lambda-ci-roles-init`, `task lambda-ci-roles-plan`, and `task lambda-ci-roles-apply`
- Release artifacts: `task lambda-artifacts-init`, `task lambda-artifacts-plan`, and `task lambda-artifacts-apply`
- Development: `task lambda-dev-init`, `task lambda-dev-plan`, and `task lambda-dev-apply`
- Production: `task lambda-prod-init`, `task lambda-prod-plan`, and `task lambda-prod-apply`
- Deploy tasks run as the `workloads-admin` SSO profile in the workloads account; see `DEPLOY-INSTRUCTIONS.md` before any deployment operation

## Architecture

- `cmd/server/main.go` is ~10 lines; all wiring in `internal/app/`
- `.templ` files in `cmd/web/{layouts,pages,partials}` are source; `*_templ.go` is generated and gitignored
- Tailwind source: `cmd/web/tailwind/*.css` and `cmd/web/tailwind/pages/*.css`; generated output: `cmd/web/static/css/tailwind.css` (gitignored)
- `Dockerfile` builds the local/Compose server image; `Dockerfile.lambda` builds
  the managed Lambda image.
- `internal/portal` contains the optional Cognito-authenticated EC2 management
  portal, including instance actions, CloudWatch metrics, and CloudWatch Logs.
  It is disabled in both environments, and its Lambda role has no EC2 start/stop
  or instance log grants (D22).
- See `.github/instructions/templ.instructions.md` and `.github/instructions/tailwind.instructions.md` for detailed authoring rules

## Gotchas

- `task dev` (air) watches only `.go` files — run `task generate` manually after `.templ` edits
- `LPS_SESSION_KEY` must be a 64-char hex string; without it, soccer auth is disabled
- Google Calendar also needs `CLIENT_ID_KEY`, `CLIENT_SECRET_KEY`, and `GOOGLE_CONNECTION_TABLE_NAME`
- The EC2 portal uses the shared `SITE_*` Cognito session. Every live `/mgmt`
  page and action requires the current `management` grant from
  `SITE_INVITATIONS_JSON`; a `mgmt_session` cookie grants nothing.
- Site OAuth uses `/sign-in`, `/auth/callback`, and `/sign-out`. The old portal
  `/login`, `/callback`, and `/logout` routes are retired in normal and preview
  modes. `MGMT_AWS_REGION` defaults to `us-east-1`.
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

## Agent skills

### Issue tracker

Issues and specs are tracked in this repository's GitHub Issues using the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

The five canonical triage roles use their default, same-named GitHub labels. See `docs/agents/triage-labels.md`.

### Domain docs

This repository uses a single-context layout with root `CONTEXT.md` and `docs/adr/`. See `docs/agents/domain.md`.

## AWS changes

- The portfolio runs in the workloads account. `AWS_ACCOUNT_ID` in `Taskfile.yaml`
  and `aws_account_id` in each OpenTofu root configure the account. The state
  bucket name in each `backend.hcl` (and the test that checks it) also contains
  the ID, because backends can't read variables. Build ARNs from
  `data.aws_caller_identity`.
- Agents may run `tofu init`, `validate`, `fmt`, `test` and `plan`. Applies,
  imports, state commands and the Taskfile apply and push wrappers need Craig's
  approval of that specific change; `.claude/settings.json` asks before them.
- Releases need no time windows or observation periods: CI deploys dev, plans
  prod, and Craig approves the `production` environment to apply (G11).
