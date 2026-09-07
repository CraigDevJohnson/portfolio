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

## Baseline integration surfaces

These observations describe the pre-implementation source at `59fffc89`, not
inferred design decisions. Tasks 1–3 of the implementation plan now address
portal identity validation, cold-start management-key resolution and the
isolated Cognito root. Runtime integration and permissions remain later work.

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

## Initial live prerequisite evidence

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

## Approved continuation preflight

Craig approved proceeding after the initial setup report. The next read-only
preflight on 2026-09-07 found:

- `origin/main` remains `59fffc8905ac19cc8c029465dc1556d6791bf279`; the
  implementation worktree is current with that base and the original dirty
  checkout is preserved.
- The `portfolio-deployer` SSO session now succeeds in account `180294223248`
  as the expected `AWSReservedSSO_PortfolioDeployer_` role. No SSO refresh or
  alternate principal was needed.
- State-bucket versioning reports `Enabled`. The current role denies encryption
  and public-access-block metadata reads and listing the new exact auth-state
  prefix. Current encryption/access controls still need verification with the
  scoped setup permissions; their historical status is insufficient.
- `cognito-idp:DescribeUserPoolDomain` is denied, so the proposed domain's
  availability remains unverified. This is an effective-permission gap, not an
  expired-session failure.
- Development alias `live` points to version `4`; `/healthz` returns revision
  `9528b784088f71fa39d1d7fce8570278c0d3acaf` with status `ok`.
- The live function has no `MGMT_*` environment variables. Its existing role is
  `portfolio-lambda-dev-execution`, with inline policy
  `portfolio-lambda-dev-runtime` and boundary
  `arn:aws:iam::180294223248:policy/portfolio/boundaries/PortfolioLambdaExecutionBoundary`.
- The Google project selection is pending. Existing Calendar OAuth credentials
  were not accessed or reused.
- Reading the boundary policy document and live IAM Access Analyzer validation
  of all three new candidate policies are also denied to `portfolio-deployer`.
  Static checks cannot replace those administrator-side verification steps.
- Prepared the operator-owned mode `0700` directory
  `/Users/craigjohnson/.config/portfolio/cognito-dev` for the private credential
  and plan channel. No credential file, session key or live plan was created.

Task 4 and the offline part of Task 5 are implemented through `a18869ae`:

- Nullable development runtime configuration supplies only public settings and
  the SSM session-key path. Enabled IAM is limited to the reviewed reads and
  tagged start/stop actions; disabled development and production retain their
  previous contracts.
- Runtime plans require the same reviewed public object as both the OpenTofu
  input and checker expectation. Automatic image rollout and alias rollback
  cannot change auth settings or IAM. The development workflow defaults its
  public `MANAGEMENT_RUNTIME_JSON` input to `null`.
- Private credential/plan/apply/export tools enforce exact identity/backend,
  private files, guarded reads, saved-plan/provenance checksums, five-resource
  contracts and public-only output. The local private directory is ready.
- Three separately named [policy candidates](../../infra/lambda/bootstrap/candidates/README.md)
  include reviewed hashes, scope and remaining live-validation requirements.
  Previously approved artifacts and their hashes are unchanged.

`task ci` passed at `559381e6`; application and Go code are unchanged after
that check. The final `task infrastructure-ci` passed at `a18869ae`, including
16 private-tooling tests, 254 runtime plan contracts, 42 rejected-input checks
and release automation. Regression tests cover both diagnostic redaction and
AWS's default Cognito email configuration in converged plans. Per-task, final
integration and scoped fix reviews have no remaining findings. Verification logs:

- `/tmp/portfolio-cognito-dev-continuation-ci.log`
- `/tmp/portfolio-cognito-dev-final-infrastructure.log`

These checks did not initialize live state, provision Cognito, alter IAM or
activate the portal. Google project selection, non-root administrator policy
validation/installation, private Google client delivery, saved live plans and
their explicit applies remain pending. The Google Cloud browser session also
requires sign-in.

## Initial setup result

Tasks 1–3 are implemented and committed locally on
`codex/cognito-google-dev-setup`, through implementation commit `15568e51`:

- Portal callbacks validate the signed Cognito identity before requiring a
  verified, exactly allowlisted email. Configuration separates the user-pool
  issuer from the hosted-login domain and uses the approved `/callback` route.
- Lambda cold starts resolve `MGMT_SESSION_KEY` through the existing atomic SSM
  secret-loading path.
- The isolated `infra/lambda/auth/dev/` root defines the Google-only Cognito
  resources and public runtime settings, with `craigdevjohnson@gmail.com` as
  the sole development allowlist entry. Its separate encrypted backend keeps
  Google provider credentials out of ordinary runtime release artifacts.

The combined implementation passed `task ci` and `task infrastructure-ci`,
including five Cognito mock-provider cases, 238 Lambda plan contracts, release
automation contracts and shared-root checks. Infrastructure validation was
rerun successfully after correcting nested test-file formatting and making
the Cognito formatting check recursive. Per-task and combined implementation
reviews have no remaining actionable findings. Local verification logs:

- `/tmp/portfolio-cognito-dev-implementation-ci.log`
- `/tmp/portfolio-cognito-dev-implementation-infrastructure.log`

These results establish the initial source setup only. Runtime environment and
IAM integration, private provisioning tooling and live activation remain
unchecked in Tasks 4–6. The Cognito stack has not been applied.

## Resume sequence

1. Review the exact candidate policy hashes and use the non-root administrator
   route to validate their effective scope before separately approved
   installation. Preserve the ordinary release role's separation from auth state.
2. Select the Google project, sign in and separately authorize the dedicated web
   OAuth client. Deliver credentials to the private channel in the
   [provisioning runbook](cognito-google-dev.md).
3. Recheck identity, bucket security, auth-state permissions and domain
   availability, then create and review a private saved auth plan. Obtain the
   separate approvals specified in the design for SecureString injection and
   exact saved-plan applies. No real credentials are needed for offline tests.
4. Follow Task 6 for reviewed source release, runtime activation and browser/EC2 proof. Do not interpret
   passing local tests or configured Terraform resources as a live deployment.
