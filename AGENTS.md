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
- `task cognito-site-ci` — offline checks of the separate dev and prod site identity roots and their operator tooling (part of `infrastructure-ci`)
- Site identity (Craig's private operator path; see `docs/deployment/site-identity.md`): `task cognito-site-dev-init`, `task cognito-site-dev-plan`, `task cognito-site-dev-apply`, `task cognito-site-dev-export`, and `task cognito-site-prod-init`, `task cognito-site-prod-plan`, `task cognito-site-prod-apply`, `task cognito-site-prod-export`
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
- `internal/portal` contains the optional EC2 management portal, including
  instance actions, CloudWatch metrics, and CloudWatch Logs. It follows site
  sign-in and admits only the current `management` grant. Both environments
  supply site sign-in. Development invites `soccer` and `management` and has
  the portal switch on, which grants read-only EC2 inventory and metrics;
  production invites only `soccer` and has no portal grants. Neither Lambda
  role has EC2 start/stop or instance log grants (D22).
- See `.github/instructions/templ.instructions.md` and `.github/instructions/tailwind.instructions.md` for detailed authoring rules

## Gotchas

- `task dev` (air) watches only `.go` files — run `task generate` manually after `.templ` edits
- `LPS_SESSION_KEY` must be a 64-char hex string; without it LPS import is off
  (config logs "soccer auth disabled")
- Google Calendar also needs `CLIENT_ID_KEY`, `CLIENT_SECRET_KEY`, and `GOOGLE_CONNECTION_TABLE_NAME`
- Site sign-in (`internal/config/site.go`, `internal/siteauth`) stays off while
  `SITE_SESSION_KEY` is empty. Once it is set, every setting below except
  `SITE_ALLOW_LOCAL_CALLBACK` is required and must be valid; otherwise config
  logs a warning, sign-in stays off, and public pages keep working.
  - `SITE_SESSION_KEY`: exactly 64 lowercase hex characters
    (`openssl rand -hex 32`); uppercase hex is refused. On Lambda it holds the
    path of the `/portfolio/lambda/<env>/SITE_SESSION_KEY` SecureString,
    resolved at cold start; a failed read disables only sign-in.
  - `SITE_COGNITO_DOMAIN` and `SITE_COGNITO_ISSUER` are separate settings. The
    domain is the hosted-UI HTTPS origin (no path) for the OAuth authorize,
    token and logout endpoints. The issuer is the user pool's
    `https://cognito-idp.<region>.amazonaws.com/<pool-id>`, used to validate
    ID tokens and fetch JWKS.
  - `SITE_COGNITO_CLIENT_ID`: the public app client.
  - `SITE_COGNITO_REDIRECT_URI`: path exactly `/auth/callback`, HTTPS. An HTTP
    loopback callback also needs `SITE_ALLOW_LOCAL_CALLBACK=true`; that
    setting is optional, and any value other than `true` or `false` disables
    sign-in. Production never registers a loopback callback.
  - `SITE_COGNITO_LOGOUT_URI`: path exactly `/sign-in`, always HTTPS.
  - `SITE_INVITATIONS_JSON`: a nonempty JSON object mapping lowercase bare
    emails to grants, each `soccer` or `management`, for example
    `{"craigdevjohnson@gmail.com":["soccer","management"]}`.
- Site routes: `GET /sign-in` renders the signed-out page, `POST /sign-in`
  starts Google sign-in through Cognito, Cognito returns to
  `GET /auth/callback`, and `POST /sign-out` ends the session and returns to
  `/sign-in`. Both POSTs accept only same-origin requests. Only a verified,
  invited email receives a site session.
- Grants are read from the current `SITE_INVITATIONS_JSON` on every request,
  never from the session cookie. On Lambda that map is `site.invitations` in
  `infra/lambda/environments/<env>/<env>.auto.tfvars`. Production site
  invitations grant only `soccer`; the production root refuses a `management`
  grant because production has no portal EC2 grants.
- Private Soccer needs a signed-in visitor holding the current `soccer` grant:
  LPS import, linked players (team discovery, their schedules, downloads and
  history) and Google Calendar. Owner-bound state, such as imported LPS access
  or a Google connection, serves only the owner who created it, while that
  owner holds the grant. Private routes answer `401` when signed out and `403`
  without the grant. Team ID lookup, Team ID schedules and `.ics` downloads
  stay public.
- The EC2 portal uses the same site session: every live `/mgmt` page and action
  requires the current `management` grant. The development `management` input
  is an identity-free switch, `null` or exactly `{ aws_region = "us-west-2" }`;
  it adds only `MGMT_AWS_REGION` (the app defaults to `us-east-1`) and the
  portal's read-only EC2 and metric grants.
- For Docker Compose: `cp .env.example .env`, set `LPS_SESSION_KEY` (`openssl rand -hex 32`)
- `task fmt` uses `golangci-lint fmt`, not `go fmt ./...` — do not suggest `go fmt`

## Known drift to avoid

- Ignore references to `cmd/web/tailwind/legacy/` — that directory no longer exists
- Ignore references to manual player-ID import — the soccer flow is JWT import + auto-discovery
- Ignore references to `PROGRESS.md` or `PRD.md` at repo root — they don't exist
- Ignore references to the retired management-only identity: the
  `MGMT_SESSION_KEY`, `MGMT_COGNITO_*`, `MGMT_ALLOWED_EMAILS` and
  `MGMT_ALLOW_LOCAL_CALLBACK` settings, the `mgmt_session` cookie, the portal's
  `/login`, `/callback` and `/logout` routes (retired in normal and preview
  modes), the `infra/lambda/auth/dev` root and the `cognito-dev-*` tasks.
  Config logs one warning and ignores those settings, infrastructure no longer
  passes them, and no role may read `MGMT_SESSION_KEY`. The portal uses the
  `SITE_*` settings and site routes above; `MGMT_AWS_REGION` and
  `MGMT_LOCAL_PREVIEW` are still current.
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
  `data.aws_caller_identity`, or from the configured account ID where no
  provider is available.
- The Lambda execution boundary (`infra/lambda/ci-roles/boundary.tf`) is a
  ceiling that already lets each environment's execution role read its four
  SecureStrings, including `SITE_SESSION_KEY`; the role's own policy reads
  that one only once the environment's `site` input is set. A boundary change
  goes through `boundary.tf`, `ci-roles/tests/policies.tftest.hcl` (which also
  checks IAM's 6,144-character limit) and a reviewed account-root plan and
  apply, never a hand edit during a release.
- Agents may run `tofu init`, `validate`, `fmt`, `test` and `plan`. Applies,
  imports, state commands and the Taskfile apply and push wrappers need Craig's
  approval of that specific change; `.claude/settings.json` asks before them.
- Releases need no time windows or observation periods: CI deploys dev, plans
  prod, and Craig approves the `production` environment to apply (G11).
