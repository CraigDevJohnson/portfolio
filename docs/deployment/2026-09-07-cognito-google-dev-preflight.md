# Cognito Google development setup preflight

Date: 2026-09-07

## Design retrieval (resolved)

Craig approved `2026-09-03-dev-cognito-google-auth-design.md` on
`feat/cognito-google-dev` and requested an implementation plan and the start of
development setup. The initial source lookup below was blocked; after Craig
pushed the branch, it was fetched at `71246fe0` and copied unchanged into this
worktree. The implementation plan is now
`docs/superpowers/plans/2026-09-07-dev-cognito-google-auth.md`.

After fetching `origin`, the named branch was absent from both local refs and
`git ls-remote --heads origin '*cognito*'`. No matching file was found in the
portfolio checkouts or the searched Projects, Documents/Codex, and Downloads
directories. GitHub issue and pull request searches for Cognito returned no
matches. Reading the earlier “Cognito Auth Environments” task returned a rate
limit error; the browser fallback did not retrieve its content.

Craig also confirmed `craigdevjohnson@gmail.com` as the sole development
allowlist entry. The historical missing-design blocker no longer applies.

## Workspace preparation

- Preserved the original checkout at `/Users/craigjohnson/repos/portfolio`,
  including its unrelated untracked files and stale local `main`.
- Created `/Users/craigjohnson/repos/portfolio-cognito-google-dev` on
  `codex/cognito-google-dev-setup` from current `origin/main`, commit
  `59fffc8905ac19cc8c029465dc1556d6791bf279`.
- Installed the repository-pinned Tailwind CLI with `task install-tailwind`.
- Ran `task ci` successfully, including generation, formatting, vet, lint,
  tests, and build. Local log: `/tmp/portfolio-cognito-dev-baseline-ci.log`.
- Ran `task infrastructure-ci` successfully, including offline OpenTofu
  initialization/validation, infrastructure tests, 238 Lambda plan contracts,
  release automation contracts, and shared-root checks. Local log:
  `/tmp/portfolio-cognito-dev-baseline-infrastructure.log`.

## Current integration surfaces

These are observations of the current source, not inferred design decisions.

| Surface | Current behavior | Files to reconcile with the design |
| --- | --- | --- |
| Portal configuration | Uses `MGMT_SESSION_KEY`, Cognito domain/client ID, callback/logout URIs, and AWS region. | `internal/config/config.go`, `internal/config/config_portal_test.go` |
| Portal OAuth | Authorization code and PKCE; derives JWKS URL and expected token issuer from the same Hosted UI domain. | `internal/portal/oidc.go`, `internal/portal/oidc_test.go` |
| Session and access | Creates a session from an ID token's email or username; no separate allowlist/group check in the callback. | `internal/portal/auth.go`, `internal/portal/session.go`, `internal/portal/session_test.go` |
| App wiring | Real portal routes require enabled configuration and an initialized handler. The loopback-only local preview has separate route wiring. | `internal/app/app.go`, `internal/app/server.go` |
| Existing Google Calendar OAuth | Direct Google OAuth, `/soccer` callback, Calendar scopes, and durable encrypted connection tokens. | `internal/google/oauth_connection.go`, `internal/google/oauth_flow.go` |
| Lambda secrets | Resolves only `CLIENT_ID_KEY`, `CLIENT_SECRET_KEY`, and `LPS_SESSION_KEY` from SSM. | `cmd/lambda/secrets.go`, `cmd/lambda/secrets_test.go` |
| Development infrastructure | `us-west-2`, `portfolio-lambda-dev`, custom domain activation configured for `dev.craigdevjohnson.com`; no Cognito resources or portal environment variables under `infra/lambda`. | `infra/lambda/environments/dev/`, `infra/lambda/modules/service/` |
| IAM and release checks | Existing policies cover the three SSM paths and current app resources. Automated release checks enforce exact topology and restrict changes to image/version/alias updates. | `infra/lambda/modules/service/iam.tf`, `infra/lambda/bootstrap/`, `scripts/check-lambda-plan.sh`, `tests/lambda-plan-contract.sh` |

Infrastructure setup needs to account for these existing release controls when
the design is translated into tasks. The current automatic release path cannot
be assumed to provision additional auth resources or permissions.

## Live prerequisite evidence

- Current-main [CI run 34082061333](https://github.com/CraigDevJohnson/portfolio/actions/runs/34082061333)
  succeeded. [Release run 34082290068](https://github.com/CraigDevJohnson/portfolio/actions/runs/34082290068)
  also succeeded; neither proves that Cognito has been configured.
- A fresh request to `https://dev.craigdevjohnson.com/healthz` returned
  `{"revision":"9528b784088f71fa39d1d7fce8570278c0d3acaf","status":"ok"}`.
  The running revision differs from current main.
- `aws --profile portfolio-deployer --region us-west-2 sts get-caller-identity`
  failed because the SSO token expired and refresh failed. No AWS resource
  inspection or change followed. Refresh command when live work resumes:
  `aws sso login --profile portfolio-deployer`.
- No AWS/Google configuration changes, infrastructure apply, deployment, merge,
  or push were performed.

## Resume sequence

1. Retrieve the exact approved design and its commit; reconcile its base with
   current `origin/main` without discarding either source.
2. Write the implementation plan in `docs/superpowers/plans/`, citing the
   approved design and mapping each requirement to concrete files, interfaces,
   verification, and development rollout steps.
3. Begin implementation with the design's independent local prerequisites and
   regression tests. Retain separate Calendar OAuth behavior unless the design
   explicitly changes it.
4. Refresh the development SSO session, verify account/role and live prerequisites,
   and follow the design and repository's saved-plan controls for development
   setup. Record any required Google console inputs without putting secrets in
   Git or logs.
