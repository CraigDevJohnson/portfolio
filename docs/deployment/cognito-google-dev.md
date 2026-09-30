# Development Cognito and Google provisioning (retired)

**Retired on 2026-09-30 (decision 8).** This runbook described the
management-only development identity: the `infra/lambda/auth/dev` root, its
`cognito-dev-*` tasks, and the nine-field `management` input they exported.
The root, tasks, scripts and tests are removed. Do not follow the procedure
below from Git history; it no longer matches the repository.

What replaced it:

- The EC2 portal signs in through site identity (`/sign-in`,
  `/auth/callback`, `/sign-out`) and admits only the `management` grant in
  `SITE_INVITATIONS_JSON` (#87).
- The private-input tooling now serves the site identity roots
  `infra/lambda/auth/site/{dev,prod}` as the `cognito-site-<env>-*` tasks; see
  [site identity](site-identity.md#private-operator-path).
- The development `management` input is an identity-free portal switch:
  `null` or exactly `{"aws_region":"us-west-2"}`. See
  [DEPLOY-INSTRUCTIONS.md](../../DEPLOY-INSTRUCTIONS.md#retired-management-only-identity).

## What existed

- Cognito pool `portfolio-lambda-dev-mgmt` with Google federation, a public
  code-flow client registering `https://dev.craigdevjohnson.com/callback` and
  `/login`, and managed-login prefix
  `portfolio-lambda-dev-mgmt-<AWS_ACCOUNT_ID>`. It was **never provisioned in
  the workloads account**: no plan was applied, no state was written at
  `portfolio-lambda-http-api/auth/dev/terraform.tfstate`, and
  `/portfolio/lambda/dev/MGMT_SESSION_KEY` was never created. Nothing in AWS
  needs removing.
- Google Cloud project `portoflio-dev-508000` (spelling intentional) holds the
  web OAuth client `portfolio-lambda-dev-mgmt-google`, with Craig as the sole
  test user and only the basic identity scopes. Nothing uses it now. Craig may
  delete it, or reuse it as the development site client by replacing its
  redirect URI with the development site root's `google_redirect_uri` output.
- The GitHub **development** environment variable `MANAGEMENT_RUNTIME_JSON`
  carried the exported nine-field object. The Release workflow now refuses
  that shape; see DEPLOY-INSTRUCTIONS.md for the change.

## History

The dated records remain as history of the earlier management-account setup
and review:

- [Preflight](2026-09-07-cognito-google-dev-preflight.md)
- [External setup review](2026-09-07-cognito-external-setup-review.md)
- [Initial plan review](2026-09-07-cognito-initial-plan-review.md)
- [PR #71 review fixes](2026-09-07-cognito-pr71-review-fixes.md)
