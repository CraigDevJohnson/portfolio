# AWS Lambda and API Gateway

> [!NOTE]
> The checked-in `infra/` directory retains shared legacy data, ECR, IAM, and
> SSM references. Replacement commands use `infra/lambda/`; the repository does
> not provide an App Runner deployment or rollback path.

The Lambda image runs the same Go HTTP handler as the regular server. The
`aws-lambda-go-api-proxy` adapter converts API Gateway HTTP API events into
`net/http` requests.

Shared ECR, OpenTofu, secret, deploy, update, and teardown instructions live in
[`DEPLOY-INSTRUCTIONS.md`](../../DEPLOY-INSTRUCTIONS.md). This page covers only
the Lambda path.

## Code path

- `Dockerfile.lambda` builds the image.
- `cmd/lambda/main.go` initializes the API Gateway adapter.
- `cmd/lambda/secrets.go` resolves configured SSM parameter paths during cold
  start.
- `internal/app.NewLambdaHandler` constructs the application routes without a
  TCP listener.
- `infra/lambda/modules/service/` declares the replacement Lambda, API Gateway,
  IAM, data, domain, log, and alarm resources.
- `infra/lambda/artifacts/`, `infra/lambda/environments/dev/`, and
  `infra/lambda/environments/prod/` own three isolated states.
- `infra/lambda/bootstrap/` contains the reviewed non-secret initial deployer
  and root-owned execution-boundary policy inputs.

The replacement environments do not share ECR ownership, DynamoDB tables, IAM
roles, SSM paths, logs, alarms, or state with the legacy stack.

## Replacement release workflow

### Merge-driven automation

The pre-automation observation on 2026-08-29 found that PR #46's merge
(`013b0a6ed10c5fc7ef0a44aa1d72c19cc30b8564`) was not deployed: both the dev
custom domain and direct API Gateway origin reported
`4db774fac83c23af5a872bcf703ba3b021a2e5c4`, and their stylesheet bytes lacked
the merged footer rules. Production still served the separate static Vue
application through CloudFront. This is historical topology evidence, not a
claim about the current live revision.

`.github/workflows/release.yml` reacts only to a successful `CI` push run for a
trusted `main` SHA. It requires one associated merged pull request and classifies
the complete reviewed base-to-merge range. Every non-review release also checks
the backlog since the latest schema-valid successful development deployment;
before the first such deployment, the commit that introduced the release
workflow is the bootstrap epoch. A pending review-class change blocks later
runtime, promotion, and docs/test automation, while a pending runtime change is
carried forward when its earlier CI or release run was canceled. Pull-request
runs, direct pushes, ambiguous associations, malformed deployment records, and
stale runs cannot obtain AWS credentials.
Runtime-only commits are built once under the release-builder OIDC role,
scanned, digest-resolved, and deployed to development through a saved,
policy-checked plan. Docs/test-only commits skip. Explicit Go and Docker build
inputs deploy; infrastructure, workflow, mixed, and unrecognized paths fail
closed.

A review-only merge waits in the protected `release-review` GitHub Environment
before recording a durable review checkpoint. Configure that environment before
merging the workflow that introduces the job: allow only protected branches,
require the designated repository owner, disable administrator bypass, and do
not assign environment variables or secrets. The review job receives no AWS credentials.
It cannot request an OIDC token and does not build, plan, or deploy the application.

The checkpoint is a development backlog cursor only. Its GitHub deployment and
latest success status bind the last trusted development SHA to the reviewed
merge SHA under the exact `portfolio-lambda-release-review` task and
`release-review` environment, the exact first attempt of the Release workflow,
and Craig's recorded approval of that environment. Future authorization
independently verifies the record schema, bot creator, Release workflow identity
and successful conclusion, environment-review history, unique merged pull
request, and Git ancestry. Failed or cancelled run markers are ignored. The
checkpoint normally applies only when its entire covered range has no runtime
or `deploy/production-release.json` change. A narrow recovery exception allows
exactly one standalone production-manifest PR in a bounded chain of uniquely
reviewed merges from the trusted development SHA. Every other merge in that
chain must be review-only or skip-only, and the current PR must be review-only.
The checkpoint does not promote that manifest; a later manifest-only PR must
still independently validate and request the protected production plan. Mixed
or current promotions, pending runtime, multiple promotions, direct commits,
and unplanned work cannot be approved away. Current pull request classification,
production-manifest comparison, development rollback version, and release
source remain based on their existing independent inputs.

Configure GitHub Environments named `development` and `production-plan`. Put
`AWS_DEVELOPMENT_DEPLOYER_ROLE_ARN` in `development` and the read-only
`AWS_PRODUCTION_PLANNER_ROLE_ARN` in `production-plan`; protect both as
appropriate and require reviewers for `production-plan`. This avoids recording
a plan-only run as a production deployment. The isolated role source and its
own remote backend are in `infra/lambda/ci-roles/`; no release workflow may
provision or modify those roles. The existing `portfolio-deployer` SSO checks
remain the local/manual escape hatch, while CI uses an exact assumed-role
identity check.

The development OIDC role assumes that the replacement stack and its remote
state were provisioned through the separately approved SSO bootstrap path. Its
only service mutations are an immutable-image update, version publication, and
the `live` alias update on the existing development function; the plan contract
requires all other managed resources to be no-ops. Missing infrastructure or
state therefore fails closed. Restore a reviewed versioned state object or run
a separately approved bootstrap plan instead of expanding CI authority or
importing resources during a release.

Development and production planning use separate non-cancelling concurrency
groups and the existing remote-state lock files. Every privileged job rechecks
that its workflow SHA is still current `main` immediately before requesting AWS
credentials, and development rechecks once more immediately before applying the
saved plan. Evidence artifacts retain the scan,
saved plan and JSON/text rendering, checksum, policy output, previous and final
alias/version, probes, alarms, and GitHub deployment identity. Verification
failure blocks promotion, records a terminal failure on the same GitHub
deployment with bounded retries, and, when a prior alias exists, saves a
checksum-bound rollback plan only after the strict rollback policy accepts it.
Apply, output, and verification failures all attempt that same plan-only
rollback evidence; applying it remains an explicit operator decision.
An exact, complete converged no-op plan is accepted on retry, but the healthy
revision JSON, live SHA, image digest, published alias, bounded HTTP 200/content
contracts, binary image bytes, and five alarms must all be reverified before a
successful deployment is recorded. Routine development verification observes
those alarms every 30 seconds for five minutes and requires every alarm to be
`OK` at every observation; `ALARM` and `INSUFFICIENT_DATA` both fail closed.
The verifier requests the five exact alarm names so the CI role remains scoped
to those five alarm ARNs; prefix enumeration would require account-wide alarm
read authority and is not permitted.
CI binds the public development hostname and TLS certificate to the recorded
API Gateway custom-domain target with `curl --connect-to`. This verifies the
application origin without making Cloudflare's interactive browser challenge
a release credential. The evidence records both hosts. Cloudflare remains
outside automation authority, so the controlled rollout and public monitoring
must verify the normal proxied hostname separately.
If an unverified runtime release is already pending when a reviewed automation
repair reaches `main`, authorization emits `development-reviewed`. The build is
then blocked on the same protected `release-review` Environment before the
ordinary development job may run. This recovery path cannot include a
production promotion, grants the review job no AWS or deployment-write
authority, and rechecks current `main` after approval.
The trusted success status includes the verified Lambda version. Authorization
uses that status to classify the backlog, then the serialized development job
resolves the status again immediately before mutation. A converged retry thus
prepares rollback evidence against the latest verified version instead of an
unverified alias target or a pre-queue snapshot. Before the first mutation, the
`in_progress` GitHub deployment records that same resolved rollback target;
`alias-before.json` separately retains the validated pre-apply alias version.
Until the first successful automated deployment exists, retries use the oldest
trusted bootstrap coordinate, preserving the original rollback target across a
hard runner loss.

Production promotion changes only `deploy/production-release.json`. Its source
SHA, ECR digest, and successful development deployment ID must agree with live
GitHub/AWS records. The image is never rebuilt. Production automation is
deliberately **plan-only** until custom-domain activation, apex and `www`
routing, certificates/HTTPS, runtime parameters, OAuth callbacks and cookies,
alarms, a verified rollback origin, and the public-cutover procedure have all
been independently rehearsed and approved. Do not claim the Go/Lambda service
is public in production before that cutover evidence exists.

Automation authority excludes legacy deploy tasks, DNS and Cloudflare, App
Runner, Amplify, state bootstrap, and SSM application-data mutation. During an
incident, preserve failed evidence, stop promotion, review the saved rollback
plan/checksum, and use the local SSO path for an approved apply. Initial public
cutover and retirement still use the longer observation gate below; routine
development releases use bounded route, identity, and alarm verification.

The tracked [bootstrap policy inputs](../../infra/lambda/bootstrap/README.md)
are authoritative for their reviewed initial bytes. Checking them in grants no
AWS access. Live provisioning, assignment, use, tightening, and reprovisioning
remain separately approved, with identity ownership evidence kept private.
The deployer input contains temporary grants and is not standing state; never
restore it after an approved removal and reprovisioning gate.

Every replacement task requires exactly the `portfolio-deployer` SSO profile in
`us-west-2`, no ambient static AWS credential variables, account
`180294223248`, and an assumed-role ARN containing
`AWSReservedSSO_PortfolioDeployer_`. Initialization and saved-plan creation are
separate from apply.

Each remote plan and apply writes and deletes one native S3 lock object. Before
setting `APPROVED_STATE_LOCK_URI`, the controller must present that exact URI and
obtain current-session approval for the lock write. The value acknowledges only
the lock object. It does not authorize the plan or apply.

The roots use these state and lock objects:

<!-- markdownlint-disable MD013 -->

| Root | State key | Lock object URI |
| --- | --- | --- |
| Artifact | `portfolio-lambda-http-api/artifacts/terraform.tfstate` | `s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/artifacts/terraform.tfstate.tflock` |
| Development | `portfolio-lambda-http-api/dev/terraform.tfstate` | `s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/dev/terraform.tfstate.tflock` |
| Production | `portfolio-lambda-http-api/prod/terraform.tfstate` | `s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/prod/terraform.tfstate.tflock` |
| CI roles | `portfolio-lambda-http-api/ci-roles/terraform.tfstate` | `s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/ci-roles/terraform.tfstate.tflock` |

<!-- markdownlint-enable MD013 -->

The 2026-08-22 preflight found that bucket versioning was not enabled, so no
replacement remote plan may run yet. Enabling versioning on only
`portfolio-tofu-state-180294223248` needs a separate non-root mutation approval,
verification that status is `Enabled`, and removal of any temporary
`s3:PutBucketVersioning` grant. The preflight also found all three runtime
replacement state keys, the CI-role state key, and
`portfolio-lambda-releases` absent. CI repeats the versioning check before any
remote plan or apply and fails closed unless the status is exactly `Enabled`.

Initialize a root first. Then create a new absolute saved-plan path and pass the
approved lock URI. For development:

```bash
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task lambda-artifacts-init
task lambda-dev-init
plan_dir=$(mktemp -d)
dev_plan="$plan_dir/dev.tfplan"
task lambda-dev-plan \
  PLAN_FILE="$dev_plan" \
  IMAGE_DIGEST="$release_digest" \
  APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/dev/terraform.tfstate.tflock
dev_plan_sha256=$(shasum -a 256 "$dev_plan" | awk '{print $1}')
printf 'dev_plan_sha256=%s\n' "$dev_plan_sha256"
```

The plan task writes one saved file, checks the JSON contract, and prints the
human-readable plan. It rejects an existing path, mutable image tags, legacy
resources, delete or replace actions, secret values, missing execution
boundaries, nondeterministic names, protection or retention drift, and alarm
drift. Review and approve the exact plan separately from the lock write, then
apply only that file:

```bash
: "${APPROVED_PLAN_SHA256:?set the exact reviewed plan SHA-256 checksum}"
task lambda-dev-apply \
  PLAN_FILE="$dev_plan" \
  APPROVED_PLAN_SHA256="$APPROVED_PLAN_SHA256" \
  APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/dev/terraform.tfstate.tflock
```

The artifact and production roots use `lambda-artifacts-*` and
`lambda-prod-*`. Artifact plans permit only the immutable release repository,
its untagged-image lifecycle policy, and its Lambda pull repository policy.
Production plans also require the reviewed `ALARM_ACTION_ARNS_JSON`.

`task lambda-release-push` accepts only a clean Git worktree. It builds
`git-<full-source-SHA>`, pushes it to `portfolio-lambda-releases`, and reports
the digest, push time, completed scan state, and severity counts. It uses the
current ECR scan-findings API rather than the deprecated scan fields on
`DescribeImages`. After push, it tolerates only a bounded initial
`ScanNotFoundException` while ECR creates the scan record, then uses the ECR
waiter and fails closed on any other error. Before pushing, it verifies
repository tag immutability and accepts only `ImageNotFoundException` as proof
that the tag is absent. Release records bind that SHA and tag to the
digest-qualified image URI, published Lambda version, and `live` alias.

The replacement runtime resolves exactly these environment-owned SSM paths:

- `/portfolio/lambda/dev/{CLIENT_ID_KEY,CLIENT_SECRET_KEY,LPS_SESSION_KEY}`;
- `/portfolio/lambda/prod/{CLIENT_ID_KEY,CLIENT_SECRET_KEY,LPS_SESSION_KEY}`.

OpenTofu records paths only. Creating or copying SecureString values needs a
separate approval and must not expose decrypted values.

## Direct endpoint and domain proof

After the approved development apply, read `api_default_url` and prove:

- `/healthz` returns the full release SHA as JSON;
- `/` and `/soccer` return 200 HTML;
- `/static/css/tailwind.css` returns 200 CSS; and
- a binary image asset returns 200 with its image content type.

Also compare the published version and `live` alias target, verify all five
alarms and both log groups, and run a saved convergence plan with the same dev
lock approval.

Custom-domain work uses two independent source changes and saved plans. First,
set `request_custom_domain = true` to request only the ACM certificate. Add its
exact validation CNAMEs as DNS-only records after separate approval. Once ACM
reports `ISSUED`, set `activate_custom_domain = true` and allow only certificate
validation, Regional API Gateway domain, and mapping creates. Both plans and
both applies need fresh approval for the exact dev lock URI and exact saved
plan. Prove OAuth directly against the API Gateway target before changing a
traffic record, and record the current origin plus complete rollback DNS
coordinates before that change.

## Retained shared resources and retirement record

The shared `infra/` root retains the `portfolio` ECR repository, DynamoDB
tables, IAM policies, and legacy SSM references. It declares no App Runner or
legacy Lambda/API Gateway runtime; every replacement root remains under
`infra/lambda/`.

The accepted 2026-08-29 retirement decision and implementation details remain in
the dated
[retirement design](../superpowers/specs/2026-08-29-development-app-runner-retirement-design.md)
and
[implementation plan](../superpowers/plans/2026-08-29-development-app-runner-retirement.md).
The failed development observation remains evidence of the old rollback-origin
stylesheet mismatch and must not be repaired or represented as passing. These
records are historical; there is no current App Runner retirement task, helper,
or temporary IAM policy. Production observation and separately approved Amplify
cleanup remain unchanged.

## Runtime behavior

The replacement environment roots pass environment-owned SSM paths through
`CLIENT_ID_KEY`, `CLIENT_SECRET_KEY`, and `LPS_SESSION_KEY`. During cold start,
`cmd/lambda/secrets.go` fetches every path-valued setting in one decrypted
`GetParameters` call. It validates the complete response, including missing,
invalid, and unusable values, before it replaces any environment value. A
failed or partial response leaves the original configuration unchanged and
fails handler initialization.

The full cold-start path has an eight-second bound. That window covers SSM
resolution and application construction. The process constructs the API
Gateway proxy once before starting Lambda and reuses it for warm invocations.

The adapter requires API Gateway's typed V2 request context and reads its
domain name as the trusted host with an `https` scheme. Generated callback URLs
and cookie security use this context-backed origin. Client-controlled `Host`
and forwarding headers cannot override it, and a missing typed gateway domain
fails closed before application routing.

OpenTofu also sets the environment-owned Google connection and Soccer
import-baseline table names. Both replacement environment roots set 512 MB of
memory and a 29-second Lambda timeout. The
application's Google Calendar add and result-sync handlers each use a 24-second
child context, leaving five seconds outside their work budget. If a deadline
ends a multi-game batch, the response reports the completed work counts and
recommends a retry. Retries match the existing Google game ID and update
completed events instead of inserting duplicates.

Register the API Gateway HTTPS URL ending in `/soccer` as a Google OAuth
redirect URI. Google returns the callback to that same route.

The Lambda resources do not pass `MGMT_*` settings or grant the EC2 and
CloudWatch permissions used by the optional management portal. The portal is
therefore unavailable on the managed Lambda path.

## Local image build and verification

The supported readiness tasks build and inspect local Linux amd64 images:

```bash
task build-image
task build-lambda-image
task test-images
```

The two build tasks pass the current full Git SHA as `BUILD_REVISION` by
default, or inject the caller's supplied `BUILD_REVISION`. Comparing `/healthz`
with that exact expected value proves the identity of those artifacts.

They do not log in to ECR, push images, apply OpenTofu, or update a running
service.

## Verify locally and for replacement Lambda environments

Run the repository gate before any approved deployment:

```bash
task ci
```

After an approved replacement deployment, verify these paths:

```text
GET /healthz
GET /
GET /soccer
GET /static/css/tailwind.css
```

`GET /healthz` returns `application/json` with the configured,
linker-injected revision in
`{"revision":"<build revision>","status":"ok"}` and `Cache-Control:
no-store`. The handler does not probe SSM, DynamoDB, Google, or Soccer during
request handling. Direct builds that omit `BUILD_REVISION` may report
`development`. Do not use that value as immutable provenance proof.

For a cold-start failure, inspect the environment-owned function's CloudWatch
Logs. Confirm its role can read each configured SSM parameter and decrypt its
KMS key.
