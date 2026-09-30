# Deployment instructions

The portfolio (including Soccer and Google Calendar) runs as a Lambda container
behind an API Gateway HTTP API in the **workloads** AWS account, us-west-2. The
account ID is configured in one place per tool: `AWS_ACCOUNT_ID` in
`Taskfile.yaml` and the `aws_account_id` input of each OpenTofu root. The state
bucket name in each `backend.hcl` also contains it, because backends can't read
variables. Every
ARN is built from `data.aws_caller_identity`, and every provider refuses
credentials for any other account.

aws-setup owns the account itself, the GitHub OIDC provider and the `alerts`
SNS topics. This repository owns everything in the table below.

## OpenTofu roots

All state lives in `portfolio-tofu-state-<AWS_ACCOUNT_ID>` (versioned, SSE-S3,
native lock files).

| Root | State key | Owns |
| --- | --- | --- |
| `infra/lambda/ci-roles` | `portfolio-lambda-http-api/ci-roles/terraform.tfstate` | Account root: the 4 GitHub OIDC CI roles, `PortfolioLambdaExecutionBoundary`, the state bucket ([README](infra/lambda/ci-roles/README.md)) |
| `infra/lambda/artifacts` | `portfolio-lambda-http-api/artifacts/terraform.tfstate` | ECR `portfolio-lambda-releases` (immutable tags, scan on push, Lambda pull policy) |
| `infra/lambda/environments/dev` | `portfolio-lambda-http-api/dev/terraform.tfstate` | `portfolio-lambda-dev`: Lambda, API, tables, logs (14 days), alarms, domain |
| `infra/lambda/environments/prod` | `portfolio-lambda-http-api/prod/terraform.tfstate` | `portfolio-lambda-prod`: as dev, plus PITR, deletion protection, reserved concurrency 10 (temporarily unreserved until the Lambda quota is raised; see `prod.auto.tfvars`), logs (30 days), alarms to `alerts` |
| `infra/lambda/auth/dev` | `portfolio-lambda-http-api/auth/dev/terraform.tfstate` | Planned dev Cognito pool, not provisioned ([runbook](docs/deployment/cognito-google-dev.md)) |
| `infra/lambda/auth/site/dev` | `portfolio-lambda-http-api/auth/site/dev/terraform.tfstate` | Planned dev site sign-in pool, not provisioned ([site identity](docs/deployment/site-identity.md)) |
| `infra/lambda/auth/site/prod` | `portfolio-lambda-http-api/auth/site/prod/terraform.tfstate` | Planned prod site sign-in pool, not provisioned ([site identity](docs/deployment/site-identity.md)) |
| `infra` | `portfolio/terraform.tfstate` | Retired legacy root. Its management-account resources are destroyed from the `management-final` tag. A guard (`infra/retired.tf`) fails every plan here |

Apply order in a new account: account root, artifacts, then dev and prod. The
execution roles attach the boundary by ARN, so the account root comes first.

## Access and approvals

- Craig signs in once with `aws sso login`. The deploy tasks set
  `AWS_PROFILE=workloads-admin` and refuse any identity other than the
  WorkloadsAdmin permission set in the configured account. The account root also
  pins that profile in its provider and backend.
- Agents inspect with `workloads-readonly`, and may run `tofu init`, `validate`,
  `fmt`, `test` and `plan`. Every apply, import, state command and Taskfile
  apply or push wrapper needs Craig's approval of that specific change;
  `.claude/settings.json` asks before them.
- No AWS keys are stored anywhere. CI uses GitHub OIDC roles only.

## Operator workflow

Plans are saved to an absolute `PLAN_FILE`, printed for review, and applied
exactly:

```sh
aws sso login
task lambda-dev-init
task lambda-dev-plan IMAGE_DIGEST=sha256:<digest> PLAN_FILE=/absolute/path/dev.tfplan
# review the printed plan
task lambda-dev-apply PLAN_FILE=/absolute/path/dev.tfplan
```

The same `-init`, `-plan` and `-apply` tasks exist for `lambda-ci-roles`,
`lambda-artifacts` and `lambda-prod`. The dev and prod plans take
`IMAGE_DIGEST` from `portfolio-lambda-releases`. The prod plan always sets
`alarm_action_arns` to the workloads us-west-2 `alerts` topic.

Pass `ACTIVATE_CUSTOM_DOMAIN=false` to plan an environment on its
`execute-api` endpoint while its hostname still belongs to another account,
and pass it to the apply task too:

```sh
task lambda-dev-plan IMAGE_DIGEST=sha256:<digest> ACTIVATE_CUSTOM_DOMAIN=false PLAN_FILE=/absolute/path/dev.tfplan
task lambda-dev-apply ACTIVATE_CUSTOM_DOMAIN=false PLAN_FILE=/absolute/path/dev.tfplan
```

When OpenTofu applies a saved plan it re-reads `*.auto.tfvars` and any `-var`
flags, and refuses with `Mismatch between input and plan variable value` if an
input differs from the plan. A `-var` value never matches the same value read
from a file. Without `ACTIVATE_CUSTOM_DOMAIN`, both tasks take
`activate_custom_domain = true` from the environment's `auto.tfvars`; with it,
both pass the same `-var` flag. If you see that error, nothing changed: rerun
the apply task with the `ACTIVATE_CUSTOM_DOMAIN` the plan used, or none if the
plan used none. The apply tasks don't need `IMAGE_DIGEST`.

`task lambda-release-push` builds the current clean commit as
`portfolio-lambda-releases:git-<sha>`, pushes it, waits for the scan, and
prints the digest.

## Runtime secrets

Each environment reads three SecureStrings at cold start:
`/portfolio/lambda/<env>/{CLIENT_ID_KEY,CLIENT_SECRET_KEY,LPS_SESSION_KEY}`.
OpenTofu only references the paths, so values never enter state. Only Craig sets
them, one at a time, without echoing the value:

```sh
read -rs V && aws ssm put-parameter --profile workloads-admin \
  --name /portfolio/lambda/<env>/<NAME> --type SecureString --value "$V" \
  --tags Key=project,Value=portfolio; unset V
```

`LPS_SESSION_KEY` is 64 hexadecimal characters (`openssl rand -hex 32`).
Changing it makes users reconnect Google once. The SecureStrings must exist
before the first environment plan, because the `alias/aws/ssm` key appears only
after the first SecureString.

## Release workflow

`.github/workflows/release.yml` runs after a successful `CI` push to `main`, or
manually with `workflow_dispatch`:

1. **authorize** classifies every change since the commit of the last
   successful `development` deployment, so an unapproved tooling change can't
   ship with a later application change. Docs and tests only skip;
   application changes release; changes to `.github/`, `infra/`, `scripts/`,
   `Taskfile.yaml`, `Dockerfile*` or `deploy/` need release review. A manual
   run, or a commit with no successful `development` deployment in its
   history, always needs release review.
2. **release-review** waits for Craig's approval in the `release-review`
   environment when needed (D23). It has no AWS credentials.
3. **build** builds `git-<sha>` once, pushes it, and requires a completed scan
   with no critical findings.
4. **development** plans the dev root with that digest, applies it, and verifies
   `/healthz` (revision), `/`, `/soccer`, the stylesheet, a JPEG, the `live`
   alias image and the five alarms.
5. **production-plan** saves the prod plan for the same digest and shows it in
   the job summary.
6. **production** waits for Craig's approval in the `production` environment,
   applies exactly that saved plan (checked by SHA-256), and verifies prod the
   same way (D24).

Every job with AWS credentials first checks that its commit is still the tip of
`main`. There are no time windows or observation periods (G11).

The CI roles can only publish a new image version and move the `live` alias.
`scripts/check-lambda-plan.sh` therefore rejects any CI plan that changes
anything else. For an infrastructure change, apply it with the tasks above
(usually from the pull request branch), then let the merge run release, or run
the Release workflow manually.

GitHub configuration:

| Scope | Variable | Protection |
| --- | --- | --- |
| Repository | `AWS_RELEASE_BUILDER_ROLE_ARN` | |
| `release-review` | | Required reviewer: Craig |
| `development` | `AWS_DEVELOPMENT_DEPLOYER_ROLE_ARN`, optional `MANAGEMENT_RUNTIME_JSON` | Protected branches |
| `production-plan` | `AWS_PRODUCTION_PLANNER_ROLE_ARN` | Protected branches. It still requires Craig as a reviewer, so a release asks for approval twice. Removing that reviewer is a separate GitHub change that needs Craig's approval |
| `production` | `AWS_PRODUCTION_DEPLOYER_ROLE_ARN` | Required reviewer: Craig |

`tofu -chdir=infra/lambda/ci-roles output role_arns` prints the four role ARNs.

## Rollback

Release the previous image: revert the commit on `main`, or plan the
environment with the previous digest and apply it after review:

```sh
task lambda-prod-plan IMAGE_DIGEST=sha256:<previous digest> PLAN_FILE=/absolute/path/rollback.tfplan
task lambda-prod-apply PLAN_FILE=/absolute/path/rollback.tfplan
```

The environment roots also accept `live_version_override` to point `live` at an
earlier published version.

A release before #93 reads only the browser-wide `google_connection` cookie, so
while it serves traffic, Google Calendar connections made on #93 or later look
disconnected. It neither reads nor rewrites them, and they return once the newer
release is live again. A connection made during the rollback has no verified
Google account; after rolling forward, its owner sees **Reconnect needed** and
must reconnect.

## Custom domains and DNS

Cloudflare is the registrar and DNS for `craigdevjohnson.com`. The records are
set by hand (D19) and listed in the
[Cloudflare runbook](docs/deployment/cloudflare-dns.md), including the
DNS-only ACM validation records and the proxied traffic records.

An API Gateway custom domain name is unique within a Region across all
accounts, so a hostname can exist in only one account at a time. Moving one
means: delete it in the old account, plan and apply the new environment
without `ACTIVATE_CUSTOM_DOMAIN` (so `activate_custom_domain=true` from its
`auto.tfvars`), then repoint Cloudflare.

## Alarms

Each environment has five alarms (Lambda errors, throttles and p95 duration;
API 5xx and p95 latency). Prod alarms notify the workloads us-west-2 `alerts`
topic; dev alarms notify nothing.

## Local image verification

These Linux amd64 tasks are local-only and do not push or deploy images:

```bash
task build-image
task build-lambda-image
task test-images
```

The build tasks accept optional `IMAGE_TAG` and `BUILD_REVISION` values. They
default to local tags and the current Git revision.

## Site sign-in

The site sign-in code accepts independent `SITE_*` runtime settings and a
reviewed `SITE_INVITATIONS_JSON` map as described in README. No environment
supplies them yet, so deployed pages show no sign-in entry. Separate offline
development and production Cognito roots and their Lambda handoff contract are
documented in [site identity configuration](./docs/deployment/site-identity.md).
They have not provisioned or activated a site pool. Register `/auth/callback`
and `/sign-in` with the environment's Cognito app client before supplying those
settings to a deployment; the existing management-only client does not register
the site callback.

### Releasing the Soccer page grant

The Soccer page grant (#86) turns off private Soccer features in every
deployed environment until that environment has site sign-in:

- Until an environment supplies complete `SITE_*` settings and a
  `SITE_INVITATIONS_JSON` that gives Craig the `soccer` grant, Soccer offers
  only Team ID lookup and .ics file downloads. LPS import, linked-player
  discovery and every Google Calendar route return `401`, even though the
  environment still supplies `LPS_SESSION_KEY` and the Google client settings.
  The page says site sign-in is not available there.
- Private state saved before the release is rejected, not migrated. An LPS
  import cookie without an owner is cleared on the next Soccer request. A
  Google connection row without an owner is never used; a granted visitor who
  still holds its cookie deletes it by disconnecting or reconnecting. Rows whose
  cookie is gone keep their encrypted tokens in `GOOGLE_CONNECTION_TABLE_NAME`,
  which has no TTL. Plan a one-time cleanup that revokes each token with Google
  and then deletes the rows with an empty `owner_subject`. That cleanup deletes
  live data and needs Craig's approval.
- Team ID lookup and .ics file downloads stay public throughout.
- Google Calendar consent (#93) also requests the basic `openid` and `email`
  scopes so the page can show the Google account that actually connected. If
  the Google OAuth consent screen lists its scopes, add those two alongside the
  Calendar scopes; the `/soccer` redirect URIs are unchanged. The server now
  also calls Google's UserInfo endpoint after consent. An owner-bound
  connection saved before #93 has no verified Google account, so it is not used
  until its owner reconnects; the Soccer page marks it **Reconnect needed** and
  offers Disconnect. Connections made on #93 or later are kept in a cookie per
  site owner instead of the browser-wide `google_connection` cookie.

Record the release order as an explicit decision before approving the next
`production` apply that contains #86. Either accept that production LPS import
and Google Calendar stay unavailable until production site identity (#88) is
provisioned and configured, or hold the approval until then. Check that order
against the first-launch criteria in #75 first.

## EC2 management portal

The portal routes use the shared `SITE_*` session and the current `management`
grant in `SITE_INVITATIONS_JSON`; they are registered only where site sign-in is
configured, and no environment supplies those settings yet. The former
management-only `MGMT_*` identity settings, `mgmt_session` cookie and
`/callback` registration no longer authorize portal access. `MGMT_AWS_REGION`
still selects the AWS region for portal operations. Its runtime role has only
read-only EC2 and metric grants: no EC2 start/stop and no `/ec2/i-*` log reads
(D22). The planned Foundry backend replaces direct EC2 control. Local mock
review uses `task portal-preview`.
