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
`site.cognito_domain` with its own environment's prefix.

The roots each own a reviewed invitation map. Both initially invite Craig with
`soccer` and `management`; identical initial grants do not share configuration
or session authority. Review and deploy a change in the relevant environment
root to change invitations or grants. An uninvited Google-federated Cognito
profile can exist but receives no site session. Production never registers an
HTTP loopback callback; development can explicitly opt in.

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
mock Cognito resources. `task infrastructure-ci` runs it, and
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

After separately approved provisioning, review each root's `site_runtime`
output and supply the complete object as that environment root's optional
`site` input. Do not mix fields between roots. The service module passes the
non-secret fields as `SITE_COGNITO_*`, `SITE_INVITATIONS_JSON`, and
`SITE_ALLOW_LOCAL_CALLBACK`; it passes only the environment-specific path for
`SITE_SESSION_KEY`. Create independently random 64-character lowercase hex
SecureString values at those paths through the approved secret channel. Lambda
resolves that parameter at startup. If it cannot resolve a site key, sign-in
stays disabled and public routes remain available. With `site = null` (the
default, and the value both environments use today), no site settings or site
session path are added to Lambda.

## Activation prerequisites

Activation is a separate, reviewed change in each environment. It needs all of:

1. A dedicated Google OAuth client for the environment.
2. A reviewed plan and apply of that environment's site root. Its plan and
   state hold the Google client secret, so it needs an operator path with the
   same private-input protections as the `cognito-dev-*` tasks; that path is
   not part of this preparation.
3. The environment's `SITE_SESSION_KEY` SecureString.
4. `SITE_SESSION_KEY` added to that environment's parameters in the Lambda
   execution boundary (`infra/lambda/ci-roles/boundary.tf`), applied through the
   account root. Until then the boundary denies the read and sign-in stays off.
5. The reviewed `site` object committed to that environment's `auto.tfvars`, so
   Craig's infrastructure apply and later CI release plans carry the same input.
   CI release plans reject any change other than the image, so Craig applies
   the environment first with `task lambda-<env>-plan` and
   `task lambda-<env>-apply`.

The previous `infra/lambda/auth/dev` root and `MGMT_*` runtime contract are
legacy management-only configuration. They remain separate from these site
roots while Issue #87 migrates the portal to the shared site session. Do not
pass the old management client or `/callback` URL into the new `SITE_*` fields.
Production activation remains subject to the separate deployment decision and
does not follow from a passing local test or mock plan.
