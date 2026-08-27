# AWS Lambda and API Gateway

> [!WARNING]
> The checked-in `infra/` directory and the `deploy`, `redeploy`,
> `deploy-lambda`, and `redeploy-lambda` tasks target legacy shared state. Keep
> them for rollback only. Replacement commands use `infra/lambda/` and must
> never deploy the new release to App Runner.

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
- `infra/lambda.tf` remains the legacy shared-stack Lambda source.

The replacement environments do not share ECR ownership, DynamoDB tables, IAM
roles, SSM paths, logs, alarms, or state with the legacy stack.

## Replacement release workflow

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

<!-- markdownlint-enable MD013 -->

The 2026-08-22 preflight found that bucket versioning was not enabled, so no
replacement remote plan may run yet. Enabling versioning on only
`portfolio-tofu-state-180294223248` needs a separate non-root mutation approval,
verification that status is `Enabled`, and removal of any temporary
`s3:PutBucketVersioning` grant. The preflight also found all three replacement
state keys and `portfolio-lambda-releases` absent.

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
plan. Prove OAuth directly against the API Gateway target before changing the
traffic record, and commit the legacy origin plus complete rollback DNS record
before cutover.

## Legacy deployment and rollback reference

`task deploy` and `task redeploy` operate on the legacy shared stack, including
App Runner. `task deploy-lambda` can bootstrap the legacy targeted Lambda path
by creating the ECR repository and initializing OpenTofu first, but it does not
reconcile the full shared infrastructure. `task redeploy-lambda` updates that
legacy function. Preserve these commands for rollback; do not use any of them
to deploy the replacement release.

Read the API endpoint after deployment:

```bash
cd infra
tofu output -raw lambda_api_url
```

## Runtime behavior

OpenTofu passes SSM paths through `CLIENT_ID_KEY`, `CLIENT_SECRET_KEY`, and
`LPS_SESSION_KEY` in the legacy stack. During cold start,
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

OpenTofu also sets the managed Google connection and Soccer import-baseline
table names. The legacy shared Terraform defaults to 512 MB and a 30-second
timeout. `lambda_memory_mb` and `lambda_timeout_seconds` control those legacy
values.

Both replacement environment roots set a 29-second Lambda timeout. The
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

## Verify locally and for legacy rollback

Run the repository gate before any approved deployment:

```bash
task ci
```

After a legacy rollback deployment, verify these paths:

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
request handling. Legacy deployment helpers and direct builds that omit
`BUILD_REVISION` may report `development`. Do not use that value as immutable
provenance proof.

For a legacy cold-start failure, inspect that function's CloudWatch Logs.
Confirm its role can read each configured SSM parameter and decrypt its KMS key.
