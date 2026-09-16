# Development Cognito Google Authentication Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Enable Google-only sign-in to the development management portal, issuing a portal session only to Craig's verified, allowlisted Google email.

**Architecture:** Keep the existing Go portal, Authorization Code flow, PKCE, and encrypted sessions. Provision development Cognito in an isolated OpenTofu auth root; transfer only reviewed public Cognito settings into the development Lambda root. Keep Google provider credentials out of runtime release state and artifacts.

**Tech Stack:** Go, net/http, golang-jwt/jwt/v5, AWS SDK v2, OpenTofu >= 1.12.0, hashicorp/aws ~> 6.38.0, Cognito managed login, Google OAuth, SSM SecureString.

**Spec:** `docs/superpowers/specs/2026-09-03-dev-cognito-google-auth-design.md`, approved source commit `71246fe0`; copied without content changes onto current-main base `59fffc8905ac19cc8c029465dc1556d6791bf279`.

## Global Constraints

- "Scope: Development Lambda environment, the existing `/mgmt` portal, and Google-federated sign-in. Production remains unchanged."
- "The app client permits the authorization-code OAuth flow with `openid`, `email`, and `profile`."
- "It does not enable Cognito username/password, SRP, administrator password, or self-service sign-up flows."
- "A rejected user does not receive a portal session."
- "`MGMT_ALLOWED_EMAILS` is an environment-owned, comma-separated exact-email allowlist."
- "The config layer normalizes and validates the list, and the callback handler denies an empty, unverified, or non-allowlisted email before creating a portal session."
- "Logs record a reason category but not raw tokens or authorization codes."
- "Cognito token signature, issuer, audience, and expiry validation stays mandatory before the allowlist decision."
- "The session key is a SecureString Parameter Store value. Its path is managed in OpenTofu; the plaintext value is injected separately."
- "The live apply and the Google Cloud OAuth-client creation remain separate, explicit external actions."
- Account `180294223248`, region `us-west-2`, callback `https://dev.craigdevjohnson.com/callback`, logout `https://dev.craigdevjohnson.com/login`.
- Sole initial allowlist entry: `craigdevjohnson@gmail.com`, supplied by Craig on 2026-09-07. Do not infer aliases or treat a configured address as token identity.
- Preserve unrelated checkout changes, Calendar OAuth credentials/scopes/routes, public page design, and the existing production infrastructure and release controls.

---

## Execution stages and review decisions

Tasks 1–3 form the initial development setup: application authorization, cold-start secret resolution, and a tested isolated Cognito stack. They run offline and produce reviewable commits. Tasks 4–6 connect and activate the stack through the existing release controls and explicit external-action gates. Do not claim the portal is deployed from completion of Tasks 1–3.

The approved design leaves state layout unspecified. Use `infra/lambda/auth/dev/` with state key `portfolio-lambda-http-api/auth/dev/terraform.tfstate`, in the existing encrypted state bucket. This is an implementation refinement: existing release and rollback tooling uploads complete runtime plans, including prior state. Putting the Google provider into that state would expose its secret even on an image-only release. Ordinary runtime CI must not read the auth state, receive the Google secret, or use `terraform_remote_state` to retrieve its public outputs. Cost of this choice: a separate state permission grant and a reviewed handoff of public settings before runtime enablement.

The current app uses its Hosted UI origin as both issuer and JWKS location. Add `MGMT_COGNITO_ISSUER` for the actual user-pool issuer. The design's `/callback` also differs from the old `/auth/callback`; update the portal and preview route contracts to the approved path.

Proposed, reviewable development infrastructure values:

| Item | Value |
| --- | --- |
| User pool | `portfolio-lambda-dev-mgmt` |
| Public app client | `portfolio-lambda-dev-mgmt-web` |
| Cognito domain prefix | `portfolio-lambda-dev-mgmt-180294223248` (availability must be checked live) |
| Google provider | `Google` |
| Session parameter | `/portfolio/lambda/dev/MGMT_SESSION_KEY` |
| Opt-in EC2 tag | `PortfolioManagement=dev` (no target is tagged by this work) |
| Optional local callback | `http://localhost:8080/callback`, disabled by default |

## Task 1: Validate Cognito identity and authorize portal sessions

**Files:**
- Modify: `internal/config/config.go`, `internal/config/config_portal_test.go`.
- Create: `internal/config/portal.go` for focused portal parsing if extracting the current portal helpers keeps `config.go` small.
- Modify: `internal/portal/oidc.go`, `internal/portal/oidc_test.go`, `internal/portal/auth.go`.
- Create: `internal/portal/auth_test.go` with real signed-token callback tests.
- Modify: `internal/app/app.go`, `internal/app/server.go`, related portal/preview route tests, `.env.example`, `docker-compose.yml`, `AGENTS.md`, `README.md` where portal configuration is described.

**Interfaces:**
- Config adds `PortalCognitoIssuer string`, `PortalAllowedEmails []string`, and `PortalAllowLocalCallback bool`.
- Config exposes `NormalizePortalEmail(raw string) (string, error)` and `(*Config).PortalEmailAllowed(email string) bool`.
- `NewOIDCClient(domain, issuer, clientID, redirectURI, logoutURI string) *OIDCClient` separates browser/token endpoints from issuer/JWKS.
- `Claims` adds `EmailVerified bool`; `ValidateIDToken` accepts only `token_use == "id"`.

- [x] Add failing configuration tests for comma-separated normalization/deduplication, missing list/issuer/callback, malformed addresses, display-name addresses, query/fragment/credential-bearing URLs, invalid boolean flags, and loopback callbacks without the explicit opt-in.

```go
cfg := &Config{PortalAllowedEmails: []string{"craigdevjohnson@gmail.com"}}
if !cfg.PortalEmailAllowed("CRAIGDEVJOHNSON@gmail.com") {
    t.Fatal("normalized exact address should match")
}
for _, email := range []string{"", "craigdevjohnson+dev@gmail.com", "craig.dev.johnson@gmail.com", "other@gmail.com", "Craig <craigdevjohnson@gmail.com>"} {
    if cfg.PortalEmailAllowed(email) { t.Fatalf("unexpected match for %q", email) }
}
```

- [x] Implement configuration parsing: trim and lowercase bare mailbox addresses, reject malformed/empty comma elements, deduplicate; require a nonempty valid allowlist, issuer, valid callback and logout alongside the existing key/domain/client fields for `PortalEnabled`. Require the literal `/callback` path and accept HTTPS callback/logout URLs; permit an HTTP loopback callback only with `MGMT_ALLOW_LOCAL_CALLBACK=true`. Validate issuer as an HTTPS URL with a user-pool path and no credentials/query/fragment. Do not derive it from request or token content.
- [x] Add a signed JWT test fixture whose Hosted UI domain differs from issuer. Serve JWKS from the issuer test server, exchange a code through the Hosted UI test server, and assert the PKCE verifier is sent. Valid tokens require RS256 signature, exact issuer/client audience, future expiry, and ID-token use. Reject missing/invalid expiry, wrong key/signature/algorithm/issuer/audience/token use, and missing subject.
- [x] Implement the issuer correction and verified-email extraction. Keep `email_verified` strictly boolean; strings such as `"true"` are not proof. Add `identity_provider=Google` to the authorization URL while retaining state, S256 challenge and all approved scopes. Remove token-endpoint response bodies from returned errors so they cannot reach callback logs.
- [x] Exercise `CallbackHandler` with encrypted OAuth-state cookies and signed ID tokens. Approved verified email returns `/mgmt` plus a decryptable session cookie. Missing/malformed/unverified/other email, username-only tokens, wrong state and invalid signature return a generic failure and no usable session cookie. Assert logs contain reason categories and no sentinel tokens, codes, response bodies or claimed email. Clear any previous portal session on a rejected callback.
- [x] Apply the allowlist only after `ValidateIDToken` succeeds. Use a fixed failure message for identity rejection and fixed log reason categories. Do not fall back to `cognito:username`. Wire the new issuer argument and `/callback` route into real and local-preview muxes, then update existing fixtures and documented local environment variables.
- [x] Run `task fmt`, `go test ./internal/config ./internal/portal ./internal/app`, then the repository gates at integration. Commit exact paths with `feat(auth): authorize verified Google portal identities`.

## Task 2: Resolve the management session key during Lambda cold start

**Files:** `cmd/lambda/secrets.go`, `cmd/lambda/secrets_test.go`.

**Interfaces:** `ssmSecretEnvVars` lists required secrets only. `resolveSSMSecretsWithClient(ctx, client) error` preserves their atomic validation-before-application behavior, then resolves the optional `MGMT_SESSION_KEY` independently. PR #71 review corrected the original shared-batch design to preserve site availability.

- [x] Add failing tests that set `MGMT_SESSION_KEY=/portfolio/lambda/dev/MGMT_SESSION_KEY` and prove the resolver requests it with decryption, installs the returned value, and clears only the portal key if it is missing, inaccessible or invalid. Required secrets still resolve atomically, and optional-key failures cannot fail Lambda startup.

```go
t.Setenv("CLIENT_ID_KEY", "")
t.Setenv("CLIENT_SECRET_KEY", "")
t.Setenv("LPS_SESSION_KEY", "")
t.Setenv("MGMT_SESSION_KEY", "/portfolio/lambda/dev/MGMT_SESSION_KEY")
client := &fakeSSMGetter{output: &ssm.GetParametersOutput{Parameters: []types.Parameter{
    ssmParameter("/portfolio/lambda/dev/MGMT_SESSION_KEY", strings.Repeat("ab", 32)),
}}}
if err := resolveSSMSecretsWithClient(t.Context(), client); err != nil { t.Fatal(err) }
assertSSMRequest(t, client, "/portfolio/lambda/dev/MGMT_SESSION_KEY")
if os.Getenv("MGMT_SESSION_KEY") != strings.Repeat("ab", 32) {
    t.Fatal("management session key was not resolved")
}
```

- [x] Resolve only the exact optional management variable without treating arbitrary `MGMT_*` values as SSM paths. Keep unset/literal management values unchanged and avoid logging plaintext values. Isolate environment variables in every resolver test.
- [x] Run `go test ./cmd/lambda`; commit `feat(lambda): resolve the management session key from SSM`.

## Task 3: Build and test the isolated development Cognito stack

**Files:**
- Create: `infra/lambda/auth/dev/{versions.tf,providers.tf,backend.hcl,variables.tf,main.tf,outputs.tf,.terraform.lock.hcl}`.
- Create: `infra/lambda/auth/dev/tests/auth_contract.tftest.hcl` and a focused `infra/lambda/auth_test.go` if needed for backend/secret isolation tests.
- Modify: `Taskfile.yaml`, `infra/lambda/layout_test.go`, `.gitignore` for offline validation and lockfile coverage only.

**Interfaces:**
- Inputs: sensitive `google_client_id` and `google_client_secret` (required, no default); `enable_local_callback` boolean defaults false. Region/account/domain/pool/client/allowlist/tag use the exact development values above. A domain-prefix override is allowed only after live availability review and must be validated.
- Public outputs: `cognito_user_pool_id`, `cognito_domain`, `cognito_issuer`, `cognito_client_id`, `google_redirect_uri`, `session_parameter_path`, and `management_runtime` with the object shape in Task 4. None contains Google client credentials or a session value.
- New offline task: `cognito-dev-ci`, included by `lambda-infrastructure-ci`.

- [x] Add mock-provider tests that assert Google-only code flow, public client, no password/SRP/custom auth defaults, verified-email mapping, exact callback/logout, default absence of localhost, explicit local opt-in, required provider credentials, deterministic account/region, and only public output names. Include a sentinel provider credential in mocks and prove it is absent from all declared outputs.
- [x] Configure the S3 backend with `bucket = "portfolio-tofu-state-180294223248"`, the dedicated auth/dev key, `region = "us-west-2"`, `encrypt = true`, and `use_lockfile = true`. Pin the existing OpenTofu/provider versions and lockfile; constrain provider account with `allowed_account_ids = ["180294223248"]`.
- [x] Define exactly the user pool, Google identity provider, public app client, managed-login domain and branding. No IAM, SSM plaintext/dummy value, Lambda, production, or remote-state resources belong in this root.

```hcl
resource "aws_cognito_user_pool" "management" {
  name           = "portfolio-lambda-dev-mgmt"
  user_pool_tier = "ESSENTIALS"
  admin_create_user_config { allow_admin_create_user_only = true }
  username_configuration { case_sensitive = false }
}

resource "aws_cognito_user_pool_client" "management" {
  name = "portfolio-lambda-dev-mgmt-web"
  user_pool_id = aws_cognito_user_pool.management.id
  generate_secret = false
  allowed_oauth_flows_user_pool_client = true
  allowed_oauth_flows = ["code"]
  allowed_oauth_scopes = ["openid", "email", "profile"]
  supported_identity_providers = [aws_cognito_identity_provider.google.provider_name]
  explicit_auth_flows = ["ALLOW_REFRESH_TOKEN_AUTH"]
  callback_urls = concat(["https://dev.craigdevjohnson.com/callback"],
    var.enable_local_callback ? ["http://localhost:8080/callback"] : [])
  logout_urls = ["https://dev.craigdevjohnson.com/login"]
  read_attributes = ["email", "email_verified", "name"]
  write_attributes = ["email", "name"]
}
```

- [x] Configure provider mappings `email=email`, `email_verified=email_verified`, `name=name`; scopes `openid email profile`; Google credentials only through sensitive variables. Use managed-login version 2 and `aws_cognito_managed_login_branding` with `use_cognito_provided_values=true`, ordered after domain creation. Do not set client write access to `email_verified`.
- [x] Export the issuer from the user-pool endpoint and Google callback from the Cognito domain plus `/oauth2/idpresponse`. Own the session parameter's name through a constant/local output; its value is injected outside OpenTofu. Export only the public `management_runtime` object and individual public outputs listed above.
- [x] Add `cognito-dev-ci`: format check, `init -backend=false -lockfile=readonly -input=false`, validate, mock test. Include this root in layout/ignore checks without weakening existing production tests. Run `task cognito-dev-ci`, `go test ./infra/lambda`, then full infrastructure validation at integration. Commit `feat(infra): define isolated development Cognito authentication`.

## Task 4: Wire reviewed public settings into the development runtime

**Files:** `infra/lambda/modules/service/{variables.tf,locals.tf,lambda.tf,iam.tf,outputs.tf}`, development root variables/module call, existing module/environment tests, `scripts/check-lambda-plan.sh`, `tests/lambda-plan-contract.sh`, `tests/release-automation.sh`, `tests/fixtures/release-fake-cli.sh`, new candidate files under `infra/lambda/bootstrap/candidates/` and candidate tests.

**Interfaces:** A nullable `management` variable, default null, is supported only by the dev root and shared service. The production root passes no management value.

```hcl
type = object({
  cognito_domain           = string
  cognito_issuer           = string
  cognito_client_id        = string
  redirect_uri             = string
  logout_uri               = string
  allowed_emails           = set(string)
  allow_local_callback     = bool
  ec2_management_tag_key   = string
  ec2_management_tag_value = string
})
default = null
```

- [x] Test disabled dev and prod plans against the existing exact environment/IAM/output contracts before adding an enabled-dev mock plan. Reject non-null management for prod, missing/changed email, wrong HTTPS callbacks, wrong region/account, broadened action/tag/log scope and credentials in runtime variables.
- [x] When non-null, derive `MGMT_SESSION_KEY` from `/portfolio/lambda/dev/MGMT_SESSION_KEY`; add the issuer/domain/client/callback/logout/allowlist/local-opt-in/region variables. Add the exact parameter ARN to SSM and KMS encryption-context permissions. Do not create a second runtime role/policy or put Cognito resources in the shared module.
- [x] Add only `ec2:DescribeInstances`, `cloudwatch:GetMetricStatistics`, `logs:FilterLogEvents`, `ec2:StartInstances`, and `ec2:StopInstances`. Start/stop use `arn:aws:ec2:us-west-2:180294223248:instance/*` with `ec2:ResourceTag/PortfolioManagement = dev`. No tagging permission. Restrict log reads to `/ec2/i-*` log-group ARNs and unavoidable wildcard read actions by region. Existing restart uses stop/start; do not add reboot permission.
- [x] Prepare separately named, unapproved bootstrap/boundary candidates. Preserve each existing `Prod*` statement exactly. Add dev-only management statements and matching KMS context; separately scope human setup-role access to the auth state and Cognito resources. Do not change the tracked hashes or approval status of previously approved bootstrap artifacts.
- [x] Extend the runtime plan checker to validate only the new exact dev env/IAM shape against reviewed public inputs. Keep secret rejection, sensitive-marker rejection, resource topology, image/alias-only automatic rollout, alias-only rollback, and production checks. Auth configuration/allowlist/IAM changes must require review, never flow through an ordinary image-only release.
- [x] Test enabled and disabled paths and mutations with `task lambda-infrastructure-ci`. Keep Google credentials and auth backend reads out of release/rollback workflows and CI roles. Commit `feat(infra): gate development portal runtime permissions` only when both existing and new contract cases pass.

## Task 5: Prepare private auth plans and perform separately approved provisioning

**Files:** `Taskfile.yaml`, `scripts/create-cognito-dev-plan.py`, `scripts/check-cognito-dev-plan.py`, associated offline script tests, `docs/deployment/cognito-google-dev.md`.

**Interfaces:** `GOOGLE_OAUTH_CREDENTIALS_FILE` points to an operator-owned private JSON file outside the repository containing `client_id` and `client_secret`. The wrapper reads it without printing and supplies sensitive `TF_VAR_google_client_id`/`TF_VAR_google_client_secret` only to the auth-root subprocess. `PLAN_FILE` is an absolute private saved-plan path; apply requires its reviewed SHA-256.

- [x] Implement offline-tested plan wrappers before live execution. Reject missing/malformed/symlinked or group/world-readable credential files and existing/relative plan paths. Use private directory/file modes, no shell evaluation, no credential CLI arguments, and no raw plan or provider-error output to stdout. Capture raw plan/JSON/logs only in the private directory. Emit only allowlisted resource-action summaries, account/region/backend identity and checksums.
- [x] Auth plan checks admit only the five exact auth resources, approved Google/code/public-client/URL/mapping settings, and reviewed create/update/no-op actions. Reject delete/replace, unrelated resources or backend/account/region changes. Add sentinel-secret tests proving no leak to stdout/stderr/public artifacts on success or failure.
- [x] Implement guarded `cognito-dev-init`, `cognito-dev-plan`, and checksum-protected `cognito-dev-apply` tasks using the repository's exact identity, state-lock and saved-plan conventions. Keep these out of automatic release jobs. Add a public-output export that selects only `management_runtime`, never all outputs or raw state.
- [x] Refresh `portfolio-deployer` SSO; verify STS account/role, encrypted backend controls and no conflicting Cognito domain. Install the three separately approved policies and verify permission-set documents, effective role policies and restored read-only administrator access. Regular deployer identity, bucket controls, auth-prefix listing and domain reads passed September 7; see the [external setup record](../../deployment/2026-09-07-cognito-external-setup-review.md).
- [ ] Verify effective backend write/lock and Cognito provisioning permissions during the separately approved live plan/apply; successful prerequisite reads and document checks do not prove those operations.
- [x] Select and verify project `portoflio-dev-508000`; complete the separately approved Google consent, dedicated `portfolio-lambda-dev-mgmt-google` web client, sole test-user entry and private credential delivery. The client has only `https://portfolio-lambda-dev-mgmt-180294223248.auth.us-west-2.amazoncognito.com/oauth2/idpresponse` and no JavaScript origins. External/Testing, app name and support/developer email were verified. Saved scopes are `openid`, `https://www.googleapis.com/auth/userinfo.email` and `https://www.googleapis.com/auth/userinfo.profile`; sensitive/restricted lists are empty. Private `google.json` is a regular mode `0600` file with exactly `client_id` and `client_secret`; the raw download is private. Google's [basic-identity Testing exemption](https://developers.google.com/identity/protocols/oauth2/production-readiness/overview) means the application verified-email gate remains authoritative.
- [ ] Obtain separate initialization, saved-plan and exact state-lock approval from the [initial plan review](../../deployment/2026-09-07-cognito-initial-plan-review.md). No auth-state write or Cognito apply has occurred.
- [ ] Create the private auth plan and review its resource actions, exact names, Google configuration, backend, state sensitivity, and checksum. Stop for the separate live-apply approval required by the design. Apply that exact saved plan after approval, then verify resources and a converged plan without printing provider details/secrets.

## Task 6: Activate development and prove allowed and rejected sign-in

**Files:** reviewed non-secret development inputs, deployment runbook, sanitized evidence under `docs/deployment/evidence/`.

**Interfaces:** Use only the Task 3 `management_runtime` output to populate the Task 4 input in a reviewable change. Record image digest/revision and auth resource identifiers; record no cookies, codes, credentials, or token bodies.

- [ ] Deploy the tested application revision through the existing reviewed release process before runtime enablement. Verify `/healthz`, immutable image digest and live alias; current main's CI success alone is insufficient.
- [ ] Generate a fresh 32-byte session key and inject its 64-character lowercase hex representation as the exact SecureString through a separately authorized action. Never print or commit it. Verify metadata/access, not the plaintext. The reviewed development boundary is installed and verified; review target EC2 instances before separately authorizing opt-in tagging.
- [ ] Commit/review the public management settings, then make a saved development runtime plan that changes only the expected env/IAM/version/alias fields. Review exact existing resources and runtime settings; apply only after explicit authorization. Production and the direct Google Calendar OAuth flow remain unchanged.
- [ ] In a browser, verify `/login` → explicit sign-in button → Google → Cognito → `/callback` → `/mgmt`; approved verified email receives a Secure/HttpOnly encrypted cookie. A second, unapproved Google identity must fail without a usable portal cookie. Also verify wrong state and disabled configuration fail safely, logout clears session and OAuth-state cookies and stays signed out, and public routes remain anonymous.
- [ ] Verify cold-start SSM resolution, describe/metrics/log reads, tagged start/stop permission and untagged denial using authorized targets. Record restart behavior separately: its current immediate stop/start sequence may fail while EC2 is stopping; fix only with a scoped test and a reviewed behavior change if live proof exposes that issue.
- [ ] Run a converged infrastructure plan and preserve sanitized outcome evidence. Record rollback as removing management settings/redeploying the previous tested alias under review, without deleting Cognito identities or production resources.

## Verification and authoritative references

Initial integration gates: `task ci`, `task infrastructure-ci`, `git diff --check`. Tests use fake credentials, mock AWS providers, and local signed tokens; no live auth or EC2 operation is part of those gates.

- [AWS federation endpoints](https://docs.aws.amazon.com/cognito/latest/developerguide/federation-endpoints.html): Hosted UI and issuer/JWKS serve different purposes.
- [AWS ID tokens](https://docs.aws.amazon.com/cognito/latest/developerguide/amazon-cognito-user-pools-using-the-id-token.html): issuer, audience, verified email, token use and signature.
- [AWS Google federation setup](https://docs.aws.amazon.com/cognito/latest/developerguide/cognito-user-pools-social-idp.html): dedicated Google client, scopes and Cognito provider callback.
- [AWS app client API](https://docs.aws.amazon.com/cognito-user-identity-pools/latest/APIReference/API_CreateUserPoolClient.html): explicit authentication flow configuration.
- [AWS managed login](https://docs.aws.amazon.com/cognito/latest/developerguide/cognito-user-pools-managed-login.html): managed-login branding for API-created clients.
