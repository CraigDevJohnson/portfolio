# Deployment instructions

> [!WARNING]
> App Runner retirement is a destructive, separately approved operation. This
> branch records the saved-plan contract only; it does not authorize a live
> custom-domain change, state-lock write, plan, or apply.

## Retained legacy infrastructure and App Runner retirement

`infra/` retains the legacy Lambda/API Gateway resources, shared DynamoDB data,
the legacy ECR repository, shared IAM policies, and `/portfolio/*` SSM
parameters. Its state also has the pending removal of App Runner-managed
resources until the approved retirement plan is applied. The root is not a path
to deploy, associate, troubleshoot, or recreate App Runner.

The retirement interfaces require exactly
`AWS_PROFILE=portfolio-deployer` and `AWS_REGION=us-west-2`, reject root and
ambient static credentials, and require acknowledgement of this exact lock URI:

```text
s3://portfolio-tofu-state-180294223248/portfolio/terraform.tfstate.tflock
```

Before any retirement plan is created, inventory the live App Runner custom
domains and obtain separate current-session approval for their disassociation.
Perform that provider operation out of band, then verify the association is
absent. It is a prerequisite to apply, is not managed by this OpenTofu state,
and is not authorized by this local branch or these instructions.

After the out-of-band boundary is complete and every live action has its own
approval, use only these interfaces:

```bash
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
export APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio/terraform.tfstate.tflock

task legacy-apprunner-retirement-init

retirement_plan_dir=$(mktemp -d)
retirement_plan="$retirement_plan_dir/legacy-apprunner-retirement.tfplan"
task legacy-apprunner-retirement-plan PLAN_FILE="$retirement_plan"

retirement_plan_sha256=$(shasum -a 256 "$retirement_plan" | awk '{print $1}')
printf 'retirement_plan_sha256=%s\n' "$retirement_plan_sha256"
```

`legacy-apprunner-retirement-plan` requires a new absolute `PLAN_FILE`, saves
the plan, checks its JSON through the App Runner retirement checker, and prints
the reviewed plan. Review its complete action list and checksum, then obtain a
separate approval for the exact saved-plan SHA-256 and the apply lock write.
Only then provide that checksum to the saved-plan-only apply interface:

```bash
: "${APPROVED_PLAN_SHA256:?set the exact reviewed plan SHA-256 checksum}"
task legacy-apprunner-retirement-apply \
  PLAN_FILE="$retirement_plan" \
  APPROVED_PLAN_SHA256="$APPROVED_PLAN_SHA256" \
  APPROVED_STATE_LOCK_URI="$APPROVED_STATE_LOCK_URI"
```

The checker permits only the approved App Runner and dedicated-IAM removals
plus their root outputs; it rejects creates, updates, replacements, unrelated
deletions, scoped operations, unattended approval, and non-saved-plan apply.
Keep Git publication and later cleanup as separate approval boundaries.

## Replacement Lambda deployment

The replacement source is under `infra/lambda/`. It has three independent
OpenTofu roots and never initializes the legacy `infra/` root:

<!-- markdownlint-disable MD013 -->

| Root | State key | Lock acknowledgement |
| --- | --- | --- |
| `artifacts` | `portfolio-lambda-http-api/artifacts/terraform.tfstate` | `s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/artifacts/terraform.tfstate.tflock` |
| `dev` | `portfolio-lambda-http-api/dev/terraform.tfstate` | `s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/dev/terraform.tfstate.tflock` |
| `prod` | `portfolio-lambda-http-api/prod/terraform.tfstate` | `s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/prod/terraform.tfstate.tflock` |

<!-- markdownlint-enable MD013 -->

The reviewed non-secret initial policy inputs are the
[development deployer policy](./infra/lambda/bootstrap/portfolio-deployer-development-bootstrap-policy.json)
and the
[root-owned execution boundary](./infra/lambda/bootstrap/portfolio-lambda-execution-boundary-policy.json).
They are authoritative for their initial policy content, but grant nothing
merely by being checked in. The deployer document contains temporary grants;
never restore it after a removal and reprovisioning gate. Keep exact Identity
Center ownership, assignment, MFA, provisioning results, and live approval
evidence private. Every live create, update, assignment, and use remains
separately approval-gated.

The lock acknowledgement is mechanical evidence that the controller approved
the exact native S3 lock-object write in the current session. It does not
authorize the plan or apply. Every changed lock path needs new approval.

### Replacement preflight status

The 2026-08-22 read-only preflight found:

- the state bucket uses AES256 encryption, but versioning status was absent;
- legacy state metadata was ETag
  `99f293c374a751614c92f83934ad6a3b`, null `VersionId`, and
  `2026-04-28T10:08:47Z` `LastModified`;
- all three replacement state keys and ECR repository
  `portfolio-lambda-releases` were absent;
- the API Gateway service-linked role existed and the account had zero HTTP
  APIs;
- the account-owned Identity Center organization instance was active; and
- root MFA was enabled, but a root access key still existed.

The 2026-08-24 non-root retry verified the legacy state metadata was unchanged
and consumed the temporary `TJ` legacy-state read grant. Artifact backend
initialization then stopped before any bucket mutation because the initial
policy did not allow `s3:ListBucket` for the absent artifact state key. The
first replacement grant was validated and reprovisioned but still returned
`403` because its retained `s3:max-keys` condition was absent from the
missing-object `HeadObject` authorization context. The approved replacement
removed `TJ` and that incompatible condition while limiting the backend list
grant to `env:/` plus the exact artifacts and development state keys. It was
reprovisioned and verified before retrying this section.

The 2026-08-25 retry initialized the artifact backend successfully under the
non-root deployer, enabled bucket versioning, read it back as `Enabled`, and
verified the artifact state prefix still contained zero objects. The approved
tightening then removed the consumed `T1` `s3:PutBucketVersioning` grant,
reprovisioned `PortfolioDeployer`, and verified the effective role before
creating an artifact plan.

The 2026-08-25 artifact apply created the immutable, scan-on-push ECR
repository, its untagged-image lifecycle policy, and its Lambda pull policy.
Live read-back found zero images, versioned state containing exactly those
three resources, and no remaining lock object. A subsequent saved convergence
plan reported `0 add, 0 change, 0 destroy`. The approved tightening then
removed the consumed `T2` and `T3` repository-administration grants and
reprovisioned `PortfolioDeployer` successfully. The effective role denies all
six retired repository-setup actions, retains artifact-state access, and still
denies production state. Do not restore `T2` or `T3`.

The first immutable image attempt built the exact commit successfully and
uploaded its layers, but ECR rejected manifest resolution because the
then-reviewed deployer policy omitted the documented `ecr:BatchGetImage` push
action. An authoritative `DescribeImages` lookup afterward returned
`ImageNotFoundException`, so the immutable tag was never created and remains
safe to reuse. The replacement candidate adds only `ecr:BatchGetImage` on the
exact `portfolio-lambda-releases` repository and adds a positive contract test
for the complete repository action set. Analyze, review, approve, update, and
reprovision this candidate before retrying the same full-SHA tag; do not restore
any retired repository-administration grant.

Do not create replacement state until bucket versioning reports `Enabled`.
After `portfolio-deployer` exists, the controller must present the exact bucket
and non-root command and obtain separate current-session approval for the one
`s3api put-bucket-versioning` mutation:

```bash
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task lambda-artifacts-init
aws --profile "$AWS_PROFILE" --region "$AWS_REGION" \
  s3api put-bucket-versioning \
  --bucket portfolio-tofu-state-180294223248 \
  --versioning-configuration Status=Enabled
test "$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" \
  s3api get-bucket-versioning \
  --bucket portfolio-tofu-state-180294223248 \
  --query Status --output text)" = "Enabled"
```

`lambda-artifacts-init` runs the full same-session identity guard immediately
before the approved mutation: exact profile, region, account, SSO-role ARN,
non-root principal, and absence of ambient static or session credentials.

If the deployer needs `s3:PutBucketVersioning`, grant only that exact bucket
action for this step and remove it after verification. Never run this mutation
as root.

### Identity and saved-plan rules

Every replacement command requires exactly `AWS_PROFILE=portfolio-deployer`
and `AWS_REGION=us-west-2`. The private guard rejects ambient
`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, and `AWS_SESSION_TOKEN`, any other
account, root, and any assumed role that does not contain
`AWSReservedSSO_PortfolioDeployer_`.

Initialization, planning, and apply are separate commands. Initialization uses
the root's `backend.hcl`, reconfigures the backend without interactive input,
and refuses any workspace other than `default`. A plan requires a new absolute
`PLAN_FILE`, writes only that saved plan, runs the offline contract checker,
and prints the human-readable plan. An apply accepts only an existing absolute
saved plan whose SHA-256 digest equals the separately approved
`APPROVED_PLAN_SHA256`. Replacement commands contain no `--auto-approve`,
`-target`, or mutable image tag.

For example, create and inspect the artifact plan only after the controller
approves the exact artifact lock write:

```bash
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task lambda-artifacts-init
plan_dir=$(mktemp -d)
artifact_plan="$plan_dir/artifacts.tfplan"
export APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/artifacts/terraform.tfstate.tflock
task lambda-artifacts-plan PLAN_FILE="$artifact_plan"
artifact_plan_sha256=$(shasum -a 256 "$artifact_plan" | awk '{print $1}')
printf 'artifact_plan_sha256=%s\n' "$artifact_plan_sha256"
```

The artifact checker permits exactly the immutable ECR repository, its
untagged-image lifecycle policy, and the Lambda pull repository policy. Before
applying, present the absolute saved-plan path, checksum, complete action list,
and exact lock URI. Obtain separate current-session apply and lock-write
approval, then run:

```bash
: "${APPROVED_PLAN_SHA256:?set the exact reviewed plan SHA-256 checksum}"
task lambda-artifacts-apply \
  PLAN_FILE="$artifact_plan" \
  APPROVED_PLAN_SHA256="$APPROVED_PLAN_SHA256" \
  APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/artifacts/terraform.tfstate.tflock
```

Development and production use the same split with `lambda-dev-*` and
`lambda-prod-*`. Development requires `IMAGE_DIGEST`; production also requires
the reviewed `ALARM_ACTION_ARNS_JSON`. The checker rejects delete or replace
actions, legacy resources, mutable tags, sensitive values, wrong execution
boundaries or deterministic names, protection and retention drift, and alarm
action drift.

### Immutable image and runtime parameters

After the artifact root exists, obtain a separate approval for the exact ECR
push. `task lambda-release-push` requires a clean worktree, builds with the full
`git rev-parse HEAD`, and pushes only
`portfolio-lambda-releases:git-<40-character-SHA>`. It first requires repository
tag immutability and an authoritative `ImageNotFoundException`; every other
lookup failure and every existing tag stops before push. Record the returned
digest, push time, completed scan status, severity counts, and digest-qualified
URI. The task waits with ECR's image-scan waiter and reads findings through
`DescribeImageScanFindings`; current Basic Scanning does not populate the old
scan fields on `DescribeImages`. ECR can briefly return `ScanNotFoundException`
before it creates a new image's scan record; the task retries only that exact
condition for a bounded interval and fails closed on every other error.
Environment plans consume only `repository-url@sha256:<64 lowercase hex
characters>`.

The environment-owned SecureString paths are:

- development: `/portfolio/lambda/dev/CLIENT_ID_KEY`,
  `/portfolio/lambda/dev/CLIENT_SECRET_KEY`, and
  `/portfolio/lambda/dev/LPS_SESSION_KEY`;
- production: `/portfolio/lambda/prod/CLIENT_ID_KEY`,
  `/portfolio/lambda/prod/CLIENT_SECRET_KEY`, and
  `/portfolio/lambda/prod/LPS_SESSION_KEY`.

Copying or creating their values is a separate approved mutation. OpenTofu
stores only these paths, never decrypted values. The legacy `/portfolio/*`
parameters remain unchanged.

### Development proof and custom domain

The initial development deployer policy excludes ACM and custom-domain
authority. Before either custom-domain stage, review and approve the exact
just-in-time development-only permission-set candidate and reprovisioning.

For the first development plan, set the approved dev lock URI and pass the
recorded digest:

```bash
task lambda-dev-init
dev_plan_dir=$(mktemp -d)
dev_plan="$dev_plan_dir/dev.tfplan"
task lambda-dev-plan \
  PLAN_FILE="$dev_plan" \
  IMAGE_DIGEST="$release_digest" \
  APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/dev/terraform.tfstate.tflock
```

After separate plan and lock approval, apply that exact file with
`lambda-dev-apply`. Before any custom-domain work, prove the direct API endpoint
returns the release SHA at `/healthz` and returns 200 with the expected content
types for `/`, `/soccer`, `/static/css/tailwind.css`, and a binary image asset.
Also verify the published version, `live` alias target, exactly five alarms,
logs, and a no-change saved convergence plan.

Custom-domain setup has two saved-plan stages:

1. Set only `request_custom_domain = true`, plan and approve the ACM certificate
   create, apply it, then add the exact DNS-only validation CNAMEs after a
   separate Cloudflare approval.
2. After ACM reports `ISSUED`, set only `activate_custom_domain = true`, plan and
   approve certificate validation, Regional API Gateway domains, and API
   mappings, then apply that exact file.

Each stage requires the dev lock URI and a fresh current-session lock-write and
apply approval. Prove OAuth against the API Gateway target before changing the
traffic record. Record the legacy origin and complete DNS rollback coordinates
before that traffic mutation.

Observation commands reinitialize artifact and environment roots before reading
outputs. They append sanitized samples and workflow request IDs to the release
record's evidence path. Production stays blocked unless the first sample and
every later gap are within 26 hours, all timestamps are current and internally
consistent, and the window spans seven full days. It also requires stable
release coordinates, distinct request IDs for two complete workflows, five
non-ALARM alarm states, no blocker, and a strict HTTPS rollback origin.

The application behavior and direct-endpoint checks are also documented in
[`docs/deployment/aws-lambda-api-gateway.md`](./docs/deployment/aws-lambda-api-gateway.md).

## Local image verification

These Linux amd64 tasks are local-only and do not push or deploy images:

```bash
task build-image
task build-lambda-image
task test-images
```

The build tasks accept optional `IMAGE_TAG` and `BUILD_REVISION` values. They
default to local tags and the current Git revision.

## Retained legacy Lambda reference

Install and configure:

- Docker;
- AWS CLI v2;
- OpenTofu 1.6 or newer;
- Task;
- the explicitly selected `portfolio-deployer` identity for approved legacy
  Lambda work.

Verify the local tools and identity:

```bash
docker version
aws --version
tofu --version
task --version
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task _lambda-identity-check
```

The retained legacy root is in `us-west-2`. Do not substitute an ambient AWS
profile or static credentials.

### Retained legacy Lambda parameters

The retained legacy Lambda runtime reads these SecureString parameters:

- `/portfolio/LPS_SESSION_KEY`
- `/portfolio/CLIENT_ID_KEY`
- `/portfolio/CLIENT_SECRET_KEY`

`LPS_SESSION_KEY` must be a 64-character hexadecimal value. Create or update
the parameters in the infrastructure region only with separate approval:

```bash
AWS_REGION=us-west-2
APP_NAME=portfolio

aws ssm put-parameter \
  --region "$AWS_REGION" \
  --name "/$APP_NAME/LPS_SESSION_KEY" \
  --type SecureString \
  --value "$(openssl rand -hex 32)" \
  --overwrite

aws ssm put-parameter \
  --region "$AWS_REGION" \
  --name "/$APP_NAME/CLIENT_ID_KEY" \
  --type SecureString \
  --value "YOUR_GOOGLE_CLIENT_ID" \
  --overwrite

aws ssm put-parameter \
  --region "$AWS_REGION" \
  --name "/$APP_NAME/CLIENT_SECRET_KEY" \
  --type SecureString \
  --value "YOUR_GOOGLE_CLIENT_SECRET" \
  --overwrite
```

The infrastructure supplies `GOOGLE_CONNECTION_TABLE_NAME` and
`SOCCER_SESSION_TABLE_NAME`. Do not duplicate those values in SSM.

Register only the applicable Lambda/API Gateway `/soccer` URL and
`http://localhost:8080/soccer` for local testing in the Google OAuth client.

`task deploy-lambda` and `task redeploy-lambda` remain legacy-Lambda helpers
pending that runtime's own retirement decision. They are not replacement-release
commands. After an approved use, read `lambda_api_url` from `infra/` and verify
`/healthz`, `/`, `/soccer`, and `/static/css/tailwind.css`.

## EC2 management portal

The portal routes are disabled unless the session key, Cognito domain, and
client ID are valid. A working sign-in also requires a registered redirect URI.
Local mock review uses:

```bash
task portal-preview
```

The current OpenTofu files do not provision Cognito, portal IAM permissions, or
`MGMT_*` runtime values. The retained Lambda deployment does not pass or resolve
those values, so the portal is not currently supported on that path.

### Retained Lambda troubleshooting

#### Legacy Lambda fails during cold start

Confirm all three SSM parameter paths exist in the configured `aws_region`
(`us-west-2` by default) and the Lambda role can call `ssm:GetParameters` and
`kms:Decrypt`. The cold-start resolver treats a missing configured parameter as
an error.

#### ECR login expired

Derive the region and registry from OpenTofu instead of hard-coding them:

```bash
ECR_URL=$(cd infra && tofu output -raw ecr_repository_url)
AWS_REGION=$(echo "$ECR_URL" | sed 's/.*\.ecr\.\(.*\)\.amazonaws\.com.*/\1/')
ECR_REGISTRY=${ECR_URL%%/*}

aws ecr get-login-password --region "$AWS_REGION" | \
  docker login --username AWS --password-stdin "$ECR_REGISTRY"
```
