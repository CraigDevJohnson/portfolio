# Site identity configuration

Issue #88 prepares separate Cognito identity for development and production. The
configuration is offline preparation. No user pool, Google OAuth client,
SecureString, Lambda setting, or page grant has been applied to AWS or Google
Cloud by this change, and both environments keep site sign-in switched off.

## Boundaries

| Setting | Development | Production |
| --- | --- | --- |
| Auth root | `infra/lambda/auth/site/dev` | `infra/lambda/auth/site/prod` |
| State key | `portfolio-lambda-http-api/auth/site/dev/terraform.tfstate` | `portfolio-lambda-http-api/auth/site/prod/terraform.tfstate` |
| Cognito pool | `portfolio-lambda-dev-site` | `portfolio-lambda-prod-site` |
| Managed-login prefix | `portfolio-lambda-dev-site-<AWS_ACCOUNT_ID>` | `portfolio-lambda-prod-site-<AWS_ACCOUNT_ID>` |
| Site callback | `https://dev.craigdevjohnson.com/auth/callback` | `https://craigdevjohnson.com/auth/callback` |
| Sign-out return | `https://dev.craigdevjohnson.com/sign-in` | `https://craigdevjohnson.com/sign-in` |
| Session SecureString path | `/portfolio/lambda/dev/SITE_SESSION_KEY` | `/portfolio/lambda/prod/SITE_SESSION_KEY` |
| Invitations and grants | `site.invitations` in `infra/lambda/environments/dev/dev.auto.tfvars` | `site.invitations` in `infra/lambda/environments/prod/prod.auto.tfvars` |

Both roots target the workloads account (`AWS_ACCOUNT_ID` in `Taskfile.yaml`,
`aws_account_id` in each root) in `us-west-2`, and keep their state in
`portfolio-tofu-state-<AWS_ACCOUNT_ID>`. They use different state keys, pools,
clients, callbacks, and session parameter paths. Each root needs its own Google
OAuth client ID and secret through the private deployment channel. The Google
OAuth client's authorized redirect must be that root's `google_redirect_uri`
output, which points to Cognito's `/oauth2/idpresponse`. Neither credential
belongs in a committed `.tfvars` file. Cognito provider credentials enter state
and saved plans, so their storage and review must remain private. The default
domain prefixes need a live availability check before any approved plan or
apply. If one is taken, an override through `cognito_domain_prefix` must still
start with `portfolio-lambda-<env>-site-` and fit in 63 characters. Each root
rejects any other prefix, because the environment root accepts only a
`site.cognito_domain` with its own environment's prefix. The operator tasks
below admit only the default prefix, so an override first needs a reviewed
change to `scripts/check-cognito-site-plan.py`.

Each environment has exactly one reviewed invitation map: `site.invitations`
in `infra/lambda/environments/<env>/<env>.auto.tfvars`. It is the only map
Lambda enforces. The site auth roots hold no grant map, and their
`site_runtime` output carries only Cognito settings. To add or revoke access,
change `site.invitations` in that file, then review and apply that environment
root. Development will initially invite Craig with `soccer` and `management`.
Production invites him with `soccer` only (decision 6, 2026-09-30): its Lambda
role has no EC2 or metric grants, so the portal could only show an error there,
and the production root's `site` validation refuses any `management` grant.
The two maps do not share configuration or session authority. An uninvited
Google-federated Cognito profile can exist but receives no site session. Production never registers an HTTP loopback callback;
development can explicitly opt in.

## Canonical production host

Production serves both `craigdevjohnson.com` and `www.craigdevjohnson.com`,
but Cognito returns only to `https://craigdevjohnson.com/auth/callback`, and
the pending sign-in and session cookies are host-only. The apex is therefore
the one site identity host. While site sign-in is configured, the application
answers every request for the `www.` alias of the `SITE_COGNITO_REDIRECT_URI`
host with a `308` to the same path and query on that host, so a sign-in started
on `www` resumes on the apex with its method and form intact. With `site =
null`, `www` keeps serving pages directly.

## Offline proof and runtime handoff

Run `task cognito-site-ci` to validate both roots and their shared module with
mock Cognito resources and to run the operator tooling's offline tests
(`task cognito-site-tooling-test`). `task infrastructure-ci` runs it, and
`go test ./infra/lambda` checks each root's backend, account pin and private
inputs, the Lambda service's `SITE_*` contract, and both environment roots
without AWS access. Production accepts only its own callbacks and domain, and
never a loopback callback. The application test
`TestProductionLikeSiteIdentitySeparatesEnvironmentsAndKeepsPublicRoutes`
loads each environment from the same `SITE_*` variables and uses fake OIDC and
LPS endpoints. It proves that only an invited, verified identity signs in, that
cross-environment tokens and sessions fail (even with a reused session key),
that each environment checks its own current page grants, and that public
pages, Team ID lookup, and ICS remain available.
`TestProductionSiteSignInMovesTheWWWAliasToTheCallbackHost` proves that
production sends `www` requests to the apex before any sign-in cookie is set.

After separately approved provisioning, export each root's reviewed
`site_runtime` output with the operator path below. Supply all of its fields,
plus that environment's reviewed `invitations` map, as the `site` object in
`infra/lambda/environments/<env>/<env>.auto.tfvars`. Do not mix fields between
roots. The service module passes the
non-secret fields as `SITE_COGNITO_*`, `SITE_INVITATIONS_JSON`, and
`SITE_ALLOW_LOCAL_CALLBACK`; it passes only the environment-specific path for
`SITE_SESSION_KEY`. Create independently random 64-character lowercase hex
SecureString values at those paths through the approved secret channel. Lambda
resolves that parameter at startup. If it cannot resolve a site key, sign-in
stays disabled and public routes remain available. With `site = null` (the
default, and the value both environments use today), no site settings or site
session path are added to Lambda.

## Private operator path

Craig plans, applies and exports each site root only through its
`cognito-site-<env>-*` tasks, where `<env>` is `dev` or `prod`. They run
`scripts/cognito-site-operator.py` as the `workloads-admin` SSO session and
refuse, before any AWS call, another profile or region, static AWS keys, any
`TF_*`, `TOFU_*` or extra `AWS_*` variable, a `.tfvars` or override file in the
root or module, and an unsafe private directory or input file. They then refuse
any identity other than the WorkloadsAdmin role in the configured account, a
non-default workspace, and a backend without encryption and lock files. Raw
OpenTofu output, plan JSON and provider data stay in new mode `0700` run
directories beneath `COGNITO_PRIVATE_DIR`, and a failure prints only a generic
message. Nothing reaches Git, chat, CI or the release workflow.

1. Create an operator-owned directory outside the checkout with mode `0700`;
   on macOS use its canonical `/private/...` path. Have the credential channel
   write that environment's Google OAuth client into it as a regular mode
   `0600` JSON file with exactly `client_id` and `client_secret`. Never paste
   either value into a command, chat, tfvars or logs. The plan refuses a
   `client_secret` that is not exactly one Google secret (`GOCSPX-` and 28
   more characters): a value pasted more than once would plan cleanly, but
   Google would then reject Cognito's token request and every sign-in would
   fail.
2. Set the inputs as environment variables, not Task variables, and plan:

   ```sh
   aws sso login
   export COGNITO_PRIVATE_DIR=/private/absolute/operator-directory
   export GOOGLE_OAUTH_CREDENTIALS_FILE="$COGNITO_PRIVATE_DIR/google-dev.json"
   export PLAN_FILE="$COGNITO_PRIVATE_DIR/site-dev.tfplan"
   export APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-<AWS_ACCOUNT_ID>/portfolio-lambda-http-api/auth/site/dev/terraform.tfstate.tflock
   task cognito-site-dev-init
   task cognito-site-dev-plan
   ```

   The lock acknowledgement names one environment's state, so the development
   one never unlocks production; it is not approval to plan or apply. The plan
   task passes the Google credentials only to the plan subprocess, saves the
   plan to `PLAN_FILE` (mode `0600`) with an adjacent `.provenance.json` that
   binds it to this environment's backend, and prints the resource actions,
   backend, account and both SHA-256 checksums.
3. Review five create, update or no-op actions under `module.site`: the
   `portfolio-lambda-<env>-site` names, the default domain prefix
   `portfolio-lambda-<env>-site-<AWS_ACCOUNT_ID>`, exactly this environment's
   `/auth/callback` and `/sign-in` URLs with no loopback callback, the Google
   scopes and attribute mapping, and a public code-flow client. The checker
   refuses deletions, replacements, other resources or modules, a
   domain-prefix override, another environment's names or URLs, and a changed
   backend. It also refuses drift, except refresh differences that change
   nothing reviewed: the pool's computed `estimated_number_of_users` or
   `last_modified_date` (once anyone signs in, every later plan reports the
   user count), an unset list or map that AWS now reports as empty, and the
   pool's `domain` becoming its reviewed prefix after the first apply. The
   Google provider declares the endpoint settings and `username` mapping that
   Cognito adds after create, so the plan after an apply is all `no-op`.
   Keep both checksums as the approval record.
4. After Craig approves that exact plan, apply it with the same `PLAN_FILE`:

   ```sh
   export APPROVED_PLAN_SHA256=<reviewed plan checksum>
   export APPROVED_PROVENANCE_SHA256=<reviewed provenance checksum>
   task cognito-site-dev-apply
   ```

   Apply checks both checksums and the provenance before any AWS call, repeats
   the identity and backend checks, re-checks the saved plan's contract and
   applies only that plan with a five-minute lock timeout. It never plans
   again. Afterwards, plan again with a fresh `PLAN_FILE` and confirm that
   every action is `no-op`.
5. Export the handoff:

   ```sh
   task cognito-site-dev-export
   ```

   It reads only the `site_runtime` output (never all outputs or state),
   refuses any field beyond the six reviewed ones or any value outside this
   environment's contract, and prints them as a `site` block. Add the
   environment's reviewed `invitations` to that block and commit it to
   `infra/lambda/environments/<env>/<env>.auto.tfvars`:

   ```hcl
   site = {
     cognito_domain       = "https://portfolio-lambda-dev-site-<AWS_ACCOUNT_ID>.auth.us-west-2.amazoncognito.com"
     cognito_issuer       = "https://cognito-idp.us-west-2.amazonaws.com/us-west-2_<pool>"
     cognito_client_id    = "<app client id>"
     redirect_uri         = "https://dev.craigdevjohnson.com/auth/callback"
     logout_uri           = "https://dev.craigdevjohnson.com/sign-in"
     allow_local_callback = false
     invitations          = { "craigdevjohnson@gmail.com" = ["soccer", "management"] }
   }
   ```

   Production's block has the production URLs and
   `invitations = { "craigdevjohnson@gmail.com" = ["soccer"] }`.

Production uses `task cognito-site-prod-init`, `task cognito-site-prod-plan`,
`task cognito-site-prod-apply` and `task cognito-site-prod-export`, with its
own Google credentials file, `PLAN_FILE` and
`.../auth/site/prod/terraform.tfstate.tflock` acknowledgement. Private run
directories are kept for local audit; remove them once the review no longer
needs them.

## Activation prerequisites

Activation is a separate, reviewed change in each environment. It needs all of:

1. A dedicated Google OAuth client for the environment.
2. A reviewed plan and apply of that environment's site root through the
   private operator path above (`task cognito-site-<env>-plan`, then
   `task cognito-site-<env>-apply` of exactly that plan).
3. The environment's `SITE_SESSION_KEY` SecureString.
4. The account root applied from a commit whose Lambda execution boundary
   (`infra/lambda/ci-roles/boundary.tf`) already allows both environments'
   `SITE_SESSION_KEY`: the reviewed `task lambda-ci-roles-plan` and
   `task lambda-ci-roles-apply` carry it, with no hand edit. One apply covers
   development and production. Until it is applied the boundary denies the
   read and sign-in stays off.
5. The reviewed `site` object from `task cognito-site-<env>-export`, with its
   `invitations` grant map added, committed to
   `infra/lambda/environments/<env>/<env>.auto.tfvars`, so Craig's
   infrastructure apply and later CI release plans carry the same input.
   CI release plans reject any change other than the image, so Craig applies
   the environment first with `task lambda-<env>-plan` and
   `task lambda-<env>-apply`.

The management-only `infra/lambda/auth/dev` root and its `cognito-dev-*`
tooling are retired (decision 8); see the
[retired runbook](cognito-google-dev.md). Issue #87 moved the portal to the
shared site session, and the development `management` input is now an
identity-free switch, `null` or exactly `{ aws_region = "us-west-2" }`, that
only grants the portal's read-only EC2 and metric access and sets
`MGMT_AWS_REGION`. Do not pass the old management client or `/callback` URL
into the `SITE_*` fields.
Production activation remains subject to the separate deployment decision and
does not follow from a passing local test or mock plan.
