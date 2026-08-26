# Deployment instructions

> [!WARNING]
> This file preserves legacy shared-stack and rollback procedures. The
> checked-in `infra/` directory and the existing `task deploy`, `task redeploy`,
> `task deploy-lambda`, and `task redeploy-lambda` commands operate on the
> legacy state that combines App Runner and Lambda. Do not use them for the
> replacement release. The new release must not be deployed to App Runner.

The legacy stack deploys container images from one Amazon ECR repository to
both AWS App Runner and AWS Lambda with API Gateway. OpenTofu manages both
runtimes, their shared DynamoDB tables, and their IAM roles in one state.

> [!IMPORTANT]
> AWS App Runner is closed to new customers. Existing customers can continue
> using it. New AWS accounts should use the Lambda path or choose another
> container service. See the
> [AWS App Runner availability change](https://docs.aws.amazon.com/apprunner/latest/dg/apprunner-availability-change.html).

## Legacy shared-stack infrastructure

`infra/*.tf` remains the source of truth for the legacy shared stack and its
rollback procedures. A full apply manages:

- one ECR repository with mutable `latest` and `lambda-latest` tags;
- an App Runner service using `latest`;
- a Lambda container function and API Gateway HTTP API using `lambda-latest`;
- DynamoDB tables for Google connections and imported Soccer session baselines;
- App Runner and Lambda IAM roles for DynamoDB and SSM Parameter Store access.

The default AWS region is `us-west-2`. The S3 backend in
`infra/versions.tf` is pinned to Craig's existing state bucket and key. Change
that backend before `tofu init` if you are deploying from another AWS account.

AWS charges vary by region and traffic. Check the current
[App Runner pricing](https://aws.amazon.com/apprunner/pricing/) and
[Lambda pricing](https://aws.amazon.com/lambda/pricing/) instead of relying on
a fixed monthly estimate.

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
digest, push time, scan status, and digest-qualified URI. Environment plans
consume only `repository-url@sha256:<64 lowercase hex characters>`.

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

## Legacy shared-stack prerequisites

Install and configure:

- Docker;
- AWS CLI v2;
- OpenTofu 1.6 or newer;
- Task;
- AWS credentials that can manage ECR, App Runner, Lambda, API Gateway,
  DynamoDB, IAM, SSM Parameter Store, KMS, and the configured S3 state backend.

Verify the local tools and identity:

```bash
docker version
aws --version
tofu --version
task --version
aws sts get-caller-identity
```

The OpenTofu default is `us-west-2`. Set `AWS_PROFILE` when you do not want the
AWS CLI's default profile.

## Configure legacy shared-stack runtime secrets

With the default `app_name = "portfolio"`, both deployed runtimes expect these
SecureString parameters:

- `/portfolio/LPS_SESSION_KEY`
- `/portfolio/CLIENT_ID_KEY`
- `/portfolio/CLIENT_SECRET_KEY`

`LPS_SESSION_KEY` must be a 64-character hexadecimal value. Create or
update the parameters in the infrastructure region before the first full apply:

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

Register each deployed `/soccer` URL in the Google OAuth client. Include the
App Runner custom domain, the API Gateway URL if Lambda is used directly, and
`http://localhost:8080/soccer` for local testing.

## Legacy shared-stack first deployment

From the repository root:

```bash
task deploy
```

The task performs these steps:

1. checks AWS, Docker, and OpenTofu access;
2. creates the ECR repository and lifecycle policy if needed;
3. builds and pushes the App Runner image as `latest`;
4. builds and pushes the Lambda image as `lambda-latest`;
5. runs a full `tofu apply --auto-approve`.

Both images must exist before the full apply because the same OpenTofu state
declares both runtimes.

Inspect the deployed endpoints:

```bash
cd infra
tofu output -raw app_runner_service_url
tofu output -raw lambda_api_url
```

Open each URL and confirm the home page loads. Confirm `/soccer` separately if
you configured Google OAuth and Soccer authentication.

## Legacy shared-stack updates

The commands below update the legacy shared stack after its first deployment.
Keep them for rollback only; do not use them for the replacement release.

```bash
# Push latest and trigger App Runner.
task redeploy

# Push lambda-latest and update the Lambda function.
task redeploy-lambda
```

`task deploy-lambda` is a targeted first-deploy helper for Lambda resources. It
does not replace a full infrastructure reconciliation with `task deploy`. Both
commands target the legacy shared OpenTofu state.

## EC2 management portal

The portal routes are disabled unless the session key, Cognito domain, and
client ID are valid. A working sign-in also requires a registered redirect URI.
Local mock review uses:

```bash
task portal-preview
```

The current OpenTofu files do not provision Cognito, portal IAM permissions, or
`MGMT_*` runtime values. To enable the portal on App Runner, configure these
values in the service runtime and add least-privilege permissions to the App
Runner instance role:

- `MGMT_SESSION_KEY`
- `MGMT_COGNITO_DOMAIN`
- `MGMT_COGNITO_CLIENT_ID`
- `MGMT_COGNITO_REDIRECT_URI`, required for sign-in and registered with Cognito
- `MGMT_COGNITO_LOGOUT_URI`, optional
- `MGMT_AWS_REGION`, defaults to `us-east-1`

Required AWS actions are:

- `ec2:DescribeInstances`
- `ec2:StartInstances`
- `ec2:StopInstances`
- `cloudwatch:GetMetricStatistics`
- `logs:FilterLogEvents`

The Terraform-managed Lambda deployment does not pass or resolve `MGMT_*`
values, so the portal is not currently supported on that path.

## Legacy App Runner custom domain and rollback reference

This legacy section applies only to accounts that already have App Runner
access. It is retained for rollback and must not be used to put the new release
on App Runner.

Associate the domain:

```bash
SERVICE_ARN=$(cd infra && tofu output -raw app_runner_service_arn)

aws apprunner associate-custom-domain \
  --service-arn "$SERVICE_ARN" \
  --domain-name craigdevjohnson.com \
  --enable-www-subdomain
```

Use the DNS target and certificate-validation records returned by AWS. Do not
copy a region-specific hostname from an example. In Cloudflare, keep the AWS
certificate-validation records set to DNS only. Keep those records after
activation so ACM can renew the certificate.

Check status with:

```bash
aws apprunner describe-custom-domains --service-arn "$SERVICE_ARN"
```

AWS says activation can take up to 24 to 48 hours. See
[Managing App Runner custom domains](https://docs.aws.amazon.com/apprunner/latest/dg/manage-custom-domains.html).

## Legacy shared-stack teardown

Disassociate an App Runner custom domain before destroying its service:

```bash
SERVICE_ARN=$(cd infra && tofu output -raw app_runner_service_arn)

aws apprunner disassociate-custom-domain \
  --service-arn "$SERVICE_ARN" \
  --domain-name craigdevjohnson.com
```

Then review the destroy plan:

```bash
cd infra
tofu plan -destroy
tofu destroy
```

The ECR repository uses `force_delete = false`. OpenTofu will refuse to delete
it while images remain. Emptying ECR is a separate destructive decision; do
not change that guard or delete images without confirming the exact repository
and recovery impact.

## Legacy shared-stack troubleshooting

### Image not found

Run `task deploy`, not a bare full `tofu apply`, for the first deployment. The
full state needs both `latest` and `lambda-latest` in ECR.

### Legacy App Runner does not become healthy

The service expects an `amd64` container listening on port `8080`. Reproduce the
runtime locally:

```bash
docker build --platform linux/amd64 -t portfolio .
docker run --rm -e APP_BIND_ALL=true -p 8080:8080 portfolio
```

Then inspect App Runner operations:

```bash
SERVICE_ARN=$(cd infra && tofu output -raw app_runner_service_arn)
aws apprunner list-operations --service-arn "$SERVICE_ARN"
```

Follow application logs from the legacy service's CloudWatch Logs group with
`task logs`. This command is legacy App Runner guidance, not a replacement
Lambda log command.

### Legacy Lambda fails during cold start

Confirm all three SSM parameter paths exist in the configured `aws_region`
(`us-west-2` by default) and the Lambda role can call `ssm:GetParameters` and
`kms:Decrypt`. The cold-start resolver treats a missing configured parameter as
an error.

### ECR login expired

Derive the region and registry from OpenTofu instead of hard-coding them:

```bash
ECR_URL=$(cd infra && tofu output -raw ecr_repository_url)
AWS_REGION=$(echo "$ECR_URL" | sed 's/.*\.ecr\.\(.*\)\.amazonaws\.com.*/\1/')
ECR_REGISTRY=${ECR_URL%%/*}

aws ecr get-login-password --region "$AWS_REGION" | \
  docker login --username AWS --password-stdin "$ECR_REGISTRY"
```
