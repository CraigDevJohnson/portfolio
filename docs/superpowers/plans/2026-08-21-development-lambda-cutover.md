# Development Lambda Cutover Implementation Plan

<!-- markdownlint-disable MD013 MD010 -->

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Provision an isolated, observable Lambda development environment, prove the complete application and OAuth workflow, and move `dev.craigdevjohnson.com` off App Runner with a tested rollback.

**Architecture:** Preserve legacy `infra/` unchanged and create an immutable artifact root, a reusable Lambda service module, and dedicated development and production roots under `infra/lambda/`. This plan applies only artifacts and development; the reviewed production source remains offline until the development observation gate passes. API Gateway invokes a published Lambda version through the `live` alias, while environment-owned IAM, SSM paths, DynamoDB tables, logs, and alarms prevent shared-state coupling.

**Tech Stack:** OpenTofu 1.12, AWS provider 6.38, ECR, Lambda container images, API Gateway HTTP API, ACM, CloudWatch, DynamoDB, SSM Parameter Store, Cloudflare DNS, Google OAuth

**Spec:** `docs/superpowers/specs/2026-08-21-aws-lambda-platform-migration-design.md`

## Global Constraints

- The integration/runtime plan must pass before infrastructure is exposed.
- Never execute replacement commands from legacy `infra/`.
- Never use OpenTofu workspaces, `-target`, `--auto-approve`, `tofu import`, `tofu state mv`, or `-migrate-state`.
- Never run AWS mutation commands as an ARN ending in `:root`.
- OpenTofu never manages or reads decrypted SSM values.
- Every apply consumes the exact saved plan that was reviewed.
- Every remote-backed plan and apply writes and deletes its native S3
  `.tflock` object. Before either command, the controller presents the exact
  root-specific `APPROVED_STATE_LOCK_URI` and obtains current-session approval
  for that lock write. The variable is a mechanical acknowledgement, not plan
  or apply authorization. A changed lock path requires new approval.
- Every Lambda image URI contains `@sha256:` and every release is tied to a full Git SHA.
- App Runner, `/portfolio/*`, both legacy DynamoDB tables, and `portfolio/terraform.tfstate` remain unchanged.
- The account-root-owned execution boundary is
  `arn:aws:iam::180294223248:policy/portfolio/boundaries/PortfolioLambdaExecutionBoundary`.
  Replacement roots may reference it, but never create, edit, or remove it.
- The reviewed initial development deployer and root-owned boundary inputs are
  tracked under [`infra/lambda/bootstrap/`](../../../infra/lambda/bootstrap/README.md).
  A separately approved Identity Center permission set created from that
  development-only input may create or pass only deterministic
  boundary-constrained execution roles. It cannot create unbounded roles,
  attach managed policies, manage the boundary, or mutate legacy resources.
  Tracked policy presence authorizes no assignment, root command, secret copy,
  plan, or apply; exact live identity and approval evidence stays private.
  Never restore the initial deployer document after a temporary SID is removed.
  The boundary's production statements are a dormant runtime ceiling, not
  production deployment authority.
- Cloudflare traffic is DNS-only during initial custom-domain proof.
- Approval of this plan does not approve a future live mutation. At every
  **Approval gate**, stop and present the exact commands, resource identifiers,
  record changes, or GitHub text; continue only after approval in that execution
  session.
- Never rely on `AWS_PROFILE` exported in an earlier task. Every block that
  accesses AWS sets `AWS_PROFILE=portfolio-deployer`, sets
  `AWS_REGION=us-west-2`, rejects ambient `AWS_ACCESS_KEY_ID`,
  `AWS_SECRET_ACCESS_KEY`, and `AWS_SESSION_TOKEN`, and reruns the identity
  guard. The guard requires account `180294223248` and an assumed-role ARN
  containing `AWSReservedSSO_PortfolioDeployer_` and rejects root.

---

### Task 1: Establish the non-root identity and state safety gate

**Files:**

- No repository changes
- Read-only AWS evidence retained in the execution log

**Interfaces:**

- Consumes: an existing configured AWS profile named `portfolio-deployer`
- Produces: verified non-root ARN, account, region, state-bucket versioning, and legacy-state object metadata

- [ ] **Step 1: Select the deployment profile without changing the default profile**

```bash
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
test -z "${AWS_ACCESS_KEY_ID+x}${AWS_SECRET_ACCESS_KEY+x}${AWS_SESSION_TOKEN+x}"
identity_json=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" \
  sts get-caller-identity --output json)
identity_arn=$(printf '%s' "$identity_json" | jq -r .Arn)
identity_account=$(printf '%s' "$identity_json" | jq -r .Account)

test "$identity_account" = "180294223248"
case "$identity_arn" in
  *:root)
    echo "Refusing to deploy with the AWS root identity" >&2
    exit 1
    ;;
  *:assumed-role/AWSReservedSSO_PortfolioDeployer_*) ;;
  *)
    echo "Refusing unexpected deployment role" >&2
    exit 1
    ;;
esac
echo "$identity_arn"
```

Expected: the `AWSReservedSSO_PortfolioDeployer_` assumed-role ARN in the
intended account. Reject any ambient static credential variable. If the profile
does not exist, stop. Establishing IAM Identity Center or a deployment role is
a separately reviewed account-access mutation.

- [ ] **Step 2: Verify state bucket versioning**

```bash
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
test -z "${AWS_ACCESS_KEY_ID+x}${AWS_SECRET_ACCESS_KEY+x}${AWS_SESSION_TOKEN+x}"
identity_arn=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" sts get-caller-identity \
  --query Arn --output text)
test "$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" sts get-caller-identity \
  --query Account --output text)" = "180294223248"
case "$identity_arn" in
  *:root) echo "Refusing AWS root identity" >&2; exit 1 ;;
  *:assumed-role/AWSReservedSSO_PortfolioDeployer_*) ;;
  *) echo "Refusing unexpected deployment role" >&2; exit 1 ;;
esac
test "$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" s3api get-bucket-versioning \
  --bucket portfolio-tofu-state-180294223248 \
  --query Status --output text)" = "Enabled"
```

The 2026-08-22 read-only preflight returned no versioning status. The bucket
uses AES256 encryption, but no replacement state may be created until
versioning reports `Enabled`.

**Approval gate:** After `portfolio-deployer` exists, present the exact bucket,
the non-root `s3api put-bucket-versioning` command, and any temporary exact
`s3:PutBucketVersioning` permission. Obtain current-session approval before the
one-bucket mutation. Verify `Enabled`, then remove that temporary permission.
Never perform this change as root.

After that exact approval only:

```bash
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
test -z "${AWS_ACCESS_KEY_ID+x}${AWS_SECRET_ACCESS_KEY+x}${AWS_SESSION_TOKEN+x}"
task lambda-artifacts-init
aws --profile "$AWS_PROFILE" --region "$AWS_REGION" s3api put-bucket-versioning \
  --bucket portfolio-tofu-state-180294223248 \
  --versioning-configuration Status=Enabled
test "$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" \
  s3api get-bucket-versioning \
  --bucket portfolio-tofu-state-180294223248 \
  --query Status --output text)" = "Enabled"
```

`lambda-artifacts-init` reruns the full exact profile, region, account,
SSO-role ARN, non-root, and ambient-credential guard in this same shell
immediately before the approved bucket mutation.

- [ ] **Step 3: Record legacy-state metadata without fetching its contents**

```bash
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
test -z "${AWS_ACCESS_KEY_ID+x}${AWS_SECRET_ACCESS_KEY+x}${AWS_SESSION_TOKEN+x}"
identity_arn=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" sts get-caller-identity \
  --query Arn --output text)
test "$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" sts get-caller-identity \
  --query Account --output text)" = "180294223248"
case "$identity_arn" in
  *:root) echo "Refusing AWS root identity" >&2; exit 1 ;;
  *:assumed-role/AWSReservedSSO_PortfolioDeployer_*) ;;
  *) echo "Refusing unexpected deployment role" >&2; exit 1 ;;
esac
aws --profile "$AWS_PROFILE" --region "$AWS_REGION" s3api head-object \
  --bucket portfolio-tofu-state-180294223248 \
  --key portfolio/terraform.tfstate \
  --query '{ETag:ETag,VersionId:VersionId,LastModified:LastModified}' \
  --output json
```

The 2026-08-22 preflight recorded ETag
`99f293c374a751614c92f83934ad6a3b`, null `VersionId`, and
`2026-04-28T10:08:47Z` `LastModified`. It also confirmed that all three
replacement state keys and `portfolio-lambda-releases` were absent, the API
Gateway service-linked role existed, the HTTP API count was zero, the
account-owned Identity Center organization instance was active, root MFA was
enabled, and a root access key still existed. No state body or secret value was
printed. Recheck the legacy metadata before the first remote plan.

---

### Task 2: Create the replacement root layout and recursive safety ignores

**Files:**

- Modify: `.gitignore`
- Create: `infra/lambda/README.md`
- Create: `infra/lambda/bootstrap/README.md`
- Create: `infra/lambda/bootstrap/portfolio-deployer-development-bootstrap-policy.json`
- Create: `infra/lambda/bootstrap/portfolio-lambda-execution-boundary-policy.json`
- Create: `infra/lambda/bootstrap/policy_contract_test.go`
- Create directories and later files under: `infra/lambda/artifacts/`, `infra/lambda/modules/service/`, `infra/lambda/environments/dev/`, `infra/lambda/environments/prod/`

**Interfaces:**

- Produces: isolated root locations and tracked provider lock files
- Preserves: all legacy `infra/*.tf` addresses and backend configuration

- [ ] **Step 1: Write a failing repository-layout contract test**

Create `infra/lambda/layout_test.go` as a small Go test that reads relative paths and asserts:

```go
required := []string{
	"artifacts/backend.hcl",
	"artifacts/main.tf",
}
```

It must also read `../../.gitignore` and require recursive `.terraform`, state, and plan patterns while rejecting a pattern that ignores `.terraform.lock.hcl`.

- [ ] **Step 2: Run and prove failure**

Run: `go test ./infra/lambda -run TestLambdaInfrastructureLayout -count=1`

Expected: missing-path failure.

- [ ] **Step 3: Replace legacy-only ignore rules with recursive rules**

Use:

```gitignore
# OpenTofu / Terraform
infra/**/.terraform/
infra/**/.tofu/
infra/**/*.tfstate
infra/**/*.tfstate.backup
infra/**/*.tfplan
tfplan
```

Remove `infra/.terraform.lock.hcl` from `.gitignore`; lock files for each new root are committed.

- [ ] **Step 4: Create the README contract**

`infra/lambda/README.md` must state:

- `artifacts` owns only immutable release storage;
- `environments/dev` and `environments/prod` have distinct backend keys;
- `modules/service` contains no backend or provider configuration;
- legacy `infra/` is not initialized by replacement commands;
- saved plans can contain sensitive configuration and stay in private temporary directories; and
- plan and apply are always separate commands.

- [ ] **Step 5: Rerun the initial artifact-layout test after Task 3 creates the named files**

Run: `go test ./infra/lambda -run TestLambdaInfrastructureLayout -count=1`

Expected after Task 3: pass. Task 6 extends the same test with module and development-root requirements.

---

### Task 3: Implement the immutable artifact root

**Files:**

- Create: `infra/lambda/artifacts/backend.hcl`
- Create: `infra/lambda/artifacts/versions.tf`
- Create: `infra/lambda/artifacts/providers.tf`
- Create: `infra/lambda/artifacts/main.tf`
- Create: `infra/lambda/artifacts/outputs.tf`
- Create: `infra/lambda/artifacts/tests/artifact_contract.tftest.hcl`
- Generate and commit: `infra/lambda/artifacts/.terraform.lock.hcl`

**Interfaces:**

- Produces: the repository, lifecycle policy, repository policy,
  `ecr_repository_name`, `ecr_repository_arn`, and `ecr_repository_url`
- Consumed by: image push task and both environment roots

- [ ] **Step 1: Define the exact backend and provider floor**

`backend.hcl`:

```hcl
bucket       = "portfolio-tofu-state-180294223248"
key          = "portfolio-lambda-http-api/artifacts/terraform.tfstate"
region       = "us-west-2"
encrypt      = true
use_lockfile = true
```

`versions.tf`:

```hcl
terraform {
  required_version = ">= 1.12.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.38.0"
    }
  }

  backend "s3" {}
}
```

`providers.tf` configures `us-west-2` and default tags `Project=portfolio`, `Platform=lambda-http-api`, `ManagedBy=opentofu`.

- [ ] **Step 2: Define the immutable repository and non-destructive lifecycle**

```hcl
resource "aws_ecr_repository" "lambda_releases" {
  name                 = "portfolio-lambda-releases"
  image_tag_mutability = "IMMUTABLE"
  force_delete         = false

  encryption_configuration {
    encryption_type = "AES256"
  }

  image_scanning_configuration {
    scan_on_push = true
  }
}

resource "aws_ecr_lifecycle_policy" "lambda_releases" {
  repository = aws_ecr_repository.lambda_releases.name
  policy = jsonencode({
    rules = [{
      rulePriority = 1
      description  = "Expire untagged images after 30 days"
      selection = {
        tagStatus   = "untagged"
        countType   = "sinceImagePushed"
        countUnit   = "days"
        countNumber = 30
      }
      action = { type = "expire" }
    }]
  })
}

resource "aws_ecr_repository_policy" "lambda_releases" {
  repository = aws_ecr_repository.lambda_releases.name
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid    = "LambdaPull"
      Effect = "Allow"
      Principal = {
        Service = "lambda.amazonaws.com"
      }
      Action = [
        "ecr:BatchGetImage",
        "ecr:GetDownloadUrlForLayer",
      ]
      Condition = {
        StringEquals = {
          "aws:SourceAccount" = "180294223248"
        }
        ArnLike = {
          "aws:SourceArn" = "arn:aws:lambda:us-west-2:180294223248:function:portfolio-lambda-*"
        }
      }
    }]
  })
}
```

Do not add a tagged-image expiration rule. The artifact mock-provider contract
must fail if the repository policy is absent, uses another principal, grants
another action, or loses either source condition. Provisioning the access
package itself must not create or import the repository, lifecycle policy, or
repository policy. Temporary deployer SIDs `T2` and `T3` authorize only the
later separately approved Terraform artifact saved plan and are removed after
its verified convergence.

- [ ] **Step 3: Add non-sensitive outputs**

Output the repository name, ARN, and URL directly from `aws_ecr_repository.lambda_releases`.

- [ ] **Step 4: Initialize without touching the backend and validate**

```bash
tofu -chdir=infra/lambda/artifacts init -backend=false -input=false
tofu -chdir=infra/lambda/artifacts fmt -check
tofu -chdir=infra/lambda/artifacts validate
```

Expected: provider lock file generated and validation successful.

- [ ] **Step 5: Commit the artifact root and ignore changes**

```bash
git add -- .gitignore infra/lambda/README.md infra/lambda/layout_test.go infra/lambda/artifacts
git diff --cached --check
git commit -m "feat(infra): add immutable Lambda release registry"
```

---

### Task 4: Implement environment-owned data and least-privilege IAM

**Files:**

- Create: `infra/lambda/modules/service/versions.tf`
- Create: `infra/lambda/modules/service/variables.tf`
- Create: `infra/lambda/modules/service/locals.tf`
- Create: `infra/lambda/modules/service/dynamodb.tf`
- Create: `infra/lambda/modules/service/iam.tf`

**Interfaces:**

- Consumes: environment, name prefix, AWS region, ECR URL, image digest, retention and protection settings
- Produces internally: table names/ARNs, SSM paths/ARNs, Lambda execution-role ARN

- [ ] **Step 1: Define exact module inputs and validation**

Include these inputs:

```hcl
variable "environment" {
  type = string
  validation {
    condition     = contains(["dev", "prod"], var.environment)
    error_message = "environment must be dev or prod"
  }
}

variable "name_prefix" { type = string }
variable "aws_region" { type = string }
variable "ecr_repository_url" { type = string }

variable "image_digest" {
  type = string
  validation {
    condition     = can(regex("^sha256:[0-9a-f]{64}$", var.image_digest))
    error_message = "image_digest must be a sha256 digest"
  }
}

variable "lambda_memory_mb" { type = number }
variable "lambda_timeout_seconds" { type = number }
variable "reserved_concurrency" { type = number }
variable "log_retention_days" { type = number }
variable "enable_pitr" { type = bool }
variable "enable_deletion_protection" { type = bool }
variable "alarm_action_arns" { type = list(string) }
variable "domain_names" { type = set(string) }
variable "request_custom_domain" { type = bool }
variable "activate_custom_domain" { type = bool }
variable "live_version_override" {
  type    = number
  default = null
}
```

Add cross-variable preconditions so activation requires certificate request and at least one domain. Validate `lambda_timeout_seconds <= 29` and `reserved_concurrency >= 1`.

- [ ] **Step 2: Define deterministic names and SSM paths**

```hcl
locals {
  function_name = var.name_prefix
  google_table  = "${var.name_prefix}-google-connections"
  soccer_table  = "${var.name_prefix}-soccer-sessions"
  ssm_base      = "/portfolio/lambda/${var.environment}"
  ssm_names     = toset(["CLIENT_ID_KEY", "CLIENT_SECRET_KEY", "LPS_SESSION_KEY"])
  ssm_paths     = { for name in local.ssm_names : name => "${local.ssm_base}/${name}" }
  image_uri     = "${var.ecr_repository_url}@${var.image_digest}"
}
```

- [ ] **Step 3: Create environment-owned on-demand tables**

The Google table uses string hash key `connection_id`. The Soccer table uses
string hash key `session_id` and TTL attribute `ttl`. Both set
`billing_mode = "PAY_PER_REQUEST"`, `deletion_protection_enabled` from the input,
PITR from the input, and explicit server-side encryption.

The execution role name is exactly `${local.function_name}-execution`; its
single inline runtime policy is `${local.function_name}-runtime`. Set
`permissions_boundary` to the ARN formed from
`data.aws_partition.current.partition` and
`data.aws_caller_identity.current.account_id` with the fixed policy path
`portfolio/boundaries/PortfolioLambdaExecutionBoundary`. The deployer does not
manage this root-owned policy.

- [ ] **Step 4: Write a single role-specific inline policy**

Use `data.aws_caller_identity.current`, `data.aws_partition.current`, and
`data.aws_kms_alias.ssm` for `alias/aws/ssm`. The inline policy grants only:

```hcl
statement {
  actions   = ["dynamodb:GetItem", "dynamodb:PutItem", "dynamodb:DeleteItem"]
  resources = [aws_dynamodb_table.google_connections.arn]
}

statement {
  actions   = ["dynamodb:PutItem"]
  resources = [aws_dynamodb_table.soccer_sessions.arn]
}

statement {
  actions   = ["ssm:GetParameters"]
  resources = [for path in values(local.ssm_paths) : "arn:${data.aws_partition.current.partition}:ssm:${var.aws_region}:${data.aws_caller_identity.current.account_id}:parameter${path}"]
}

statement {
  actions   = ["kms:Decrypt"]
  resources = [data.aws_kms_alias.ssm.target_key_arn]
}
```

Task 5 adds the exact log-group actions after creating their ARNs. The trust
policy allows only `lambda.amazonaws.com` to call `sts:AssumeRole`. Keep the
runtime actions exactly as listed plus the two Task 5 log actions. Do not add
legacy paths, deployment privileges, managed-policy attachments,
`logs:CreateLogGroup`, or boundary-management actions.

- [ ] **Step 5: Format the module files**

Run: `tofu fmt -check -recursive infra/lambda/modules/service`

Expected: success after running `tofu fmt -recursive infra/lambda/modules/service` once if formatting changed.

- [ ] **Step 6: Commit the data and IAM boundary**

```bash
git add -- infra/lambda/modules/service/versions.tf infra/lambda/modules/service/variables.tf infra/lambda/modules/service/locals.tf infra/lambda/modules/service/dynamodb.tf infra/lambda/modules/service/iam.tf
git diff --cached --check
git commit -m "feat(infra): define Lambda data and IAM boundaries"
```

---

### Task 5: Implement the published Lambda, HTTP API, logs, alarms, and staged domain

**Files:**

- Create: `infra/lambda/modules/service/observability.tf`
- Create: `infra/lambda/modules/service/lambda.tf`
- Create: `infra/lambda/modules/service/api.tf`
- Create: `infra/lambda/modules/service/domain.tf`
- Create: `infra/lambda/modules/service/outputs.tf`

**Interfaces:**

- Produces: published function version, `live` alias, execute-api URL, custom-domain targets, log groups, alarm ARNs, OAuth redirect URIs
- Consumed by: development root outputs and verification commands

- [ ] **Step 1: Precreate finite-retention log groups**

```hcl
resource "aws_cloudwatch_log_group" "lambda" {
  name              = "/aws/lambda/${local.function_name}"
  retention_in_days = var.log_retention_days
}

resource "aws_cloudwatch_log_group" "api_access" {
  name              = "/aws/apigateway/${local.function_name}/access"
  retention_in_days = var.log_retention_days
}
```

Extend the inline IAM policy with `logs:CreateLogStream` and
`logs:PutLogEvents` on `${aws_cloudwatch_log_group.lambda.arn}:*`. Do not grant
`logs:CreateLogGroup`.

- [ ] **Step 2: Create a digest-pinned published function and alias**

```hcl
resource "aws_lambda_function" "app" {
  function_name                  = local.function_name
  role                           = aws_iam_role.lambda.arn
  package_type                   = "Image"
  architectures                  = ["x86_64"]
  image_uri                      = local.image_uri
  memory_size                    = var.lambda_memory_mb
  timeout                        = var.lambda_timeout_seconds
  reserved_concurrent_executions = var.reserved_concurrency
  publish                        = true

  environment {
    variables = {
      CLIENT_ID_KEY                = local.ssm_paths.CLIENT_ID_KEY
      CLIENT_SECRET_KEY            = local.ssm_paths.CLIENT_SECRET_KEY
      GOOGLE_CONNECTION_TABLE_NAME = aws_dynamodb_table.google_connections.name
      LOG_ADD_SOURCE               = "false"
      LOG_FORMAT                   = "json"
      LOG_LEVEL                    = "info"
      LPS_SESSION_KEY              = local.ssm_paths.LPS_SESSION_KEY
      SOCCER_SESSION_TABLE_NAME    = aws_dynamodb_table.soccer_sessions.name
    }
  }

  depends_on = [aws_cloudwatch_log_group.lambda, aws_iam_role_policy.lambda]
}

locals {
  live_version = var.live_version_override == null ? aws_lambda_function.app.version : tostring(var.live_version_override)
}

resource "aws_lambda_alias" "live" {
  name             = "live"
  function_name    = aws_lambda_function.app.function_name
  function_version = local.live_version
}
```

- [ ] **Step 3: Route the HTTP API only through the alias**

Create HTTP API `${local.function_name}-http`, an `AWS_PROXY` integration using
`aws_lambda_alias.live.invoke_arn`, payload version `2.0`, a `$default` route,
and a `$default` stage with `auto_deploy=true`.

The access-log format is `jsonencode` of request ID, source IP, method, path,
route key, status, response length, response latency, integration status,
integration latency, and integration error message. It does not include query
strings, headers, cookies, or bodies.

The Lambda permission uses the function name plus `qualifier = aws_lambda_alias.live.name` and the API execution ARN.

- [ ] **Step 4: Add core alarms**

Create alarms with `treat_missing_data = "notBreaching"` and the supplied action ARNs:

- Lambda `Errors`, Sum `>= 1` over five minutes;
- Lambda `Throttles`, Sum `>= 1` over five minutes;
- Lambda `Duration`, p95 `>= 24000` milliseconds;
- API Gateway `5xx`, Sum `>= 1` over five minutes; and
- API Gateway `Latency`, p95 `>= 25000` milliseconds.

Use the exact Lambda function-name and API-ID dimensions.

The alarm names are exactly `${local.function_name}-lambda-errors`,
`${local.function_name}-lambda-throttles`,
`${local.function_name}-lambda-duration`, `${local.function_name}-api-5xx`, and
`${local.function_name}-api-latency`.

- [ ] **Step 5: Stage certificate request separately from activation**

When `request_custom_domain` is true, request one DNS-validated ACM certificate
for the sorted domain set, using the first as `domain_name` and the remainder as
SANs. Output each `domain_validation_options` record name, type, and value.

When `activate_custom_domain` is true, create
`aws_acm_certificate_validation`, one Regional `aws_apigatewayv2_domain_name`
per domain, and one `$default` API mapping per domain. The certificate is in
`us-west-2`, the same region as the Regional API.

- [ ] **Step 6: Add all non-secret outputs**

Define these exact output names and stable types so later commands do not invent
an interface:

- strings: `environment`, `image_uri`, `lambda_function_name`,
  `lambda_function_arn`, `lambda_published_version`, `lambda_alias_name`,
  `lambda_alias_arn`, `lambda_execution_role_name`,
  `lambda_execution_permissions_boundary_arn`, `lambda_runtime_policy_name`,
  `api_id`, `api_default_url`, `api_name`,
  `lambda_log_group_name`, `api_access_log_group_name`,
  `google_connection_table_name`, `google_connection_table_arn`,
  `soccer_session_table_name`, and `soccer_session_table_arn`;
- `ssm_parameter_paths`: `map(string)` keyed by the three application variable
  names;
- `alarm_arns`: a sorted `list(string)` containing exactly the five named alarm
  ARNs;
- `alarm_names`: a sorted `list(string)` containing exactly the five fixed
  alarm names;
- `certificate_arn`: a nullable string;
- `acm_validation_records`: a sorted list of objects with `domain_name`,
  `resource_record_name`, `resource_record_type`, and
  `resource_record_value` string fields;
- `api_gateway_domain_targets`: `map(string)` from requested hostname to the
  Regional API Gateway target; and
- `oauth_redirect_uris`: a sorted `list(string)` containing each
  `https://domain/soccer` URI.

Extend `infra/lambda/layout_test.go` with exact-name and type-shape assertions
over the module and environment output declarations. The environment roots must
forward the module outputs under the same names without changing their types.

- [ ] **Step 7: Format and commit the runtime module**

```bash
tofu fmt -recursive infra/lambda/modules/service
tofu fmt -check -recursive infra/lambda/modules/service
git add -- infra/lambda/layout_test.go infra/lambda/modules/service/observability.tf infra/lambda/modules/service/lambda.tf infra/lambda/modules/service/api.tf infra/lambda/modules/service/domain.tf infra/lambda/modules/service/outputs.tf infra/lambda/modules/service/iam.tf
git diff --cached --check
git commit -m "feat(infra): define the published Lambda service"
```

---

### Task 6: Create both environment roots and validate the module

**Files:**

- Create: `infra/lambda/environments/dev/backend.hcl`
- Create: `infra/lambda/environments/dev/versions.tf`
- Create: `infra/lambda/environments/dev/providers.tf`
- Create: `infra/lambda/environments/dev/variables.tf`
- Create: `infra/lambda/environments/dev/main.tf`
- Create: `infra/lambda/environments/dev/outputs.tf`
- Create: `infra/lambda/environments/dev/dev.auto.tfvars`
- Generate and commit: `infra/lambda/environments/dev/.terraform.lock.hcl`
- Create: `infra/lambda/environments/prod/backend.hcl`
- Create: `infra/lambda/environments/prod/versions.tf`
- Create: `infra/lambda/environments/prod/providers.tf`
- Create: `infra/lambda/environments/prod/variables.tf`
- Create: `infra/lambda/environments/prod/main.tf`
- Create: `infra/lambda/environments/prod/outputs.tf`
- Create: `infra/lambda/environments/prod/prod.auto.tfvars`
- Generate and commit: `infra/lambda/environments/prod/.terraform.lock.hcl`

**Interfaces:**

- Consumes: `TF_VAR_ecr_repository_url` and `TF_VAR_image_digest`
- Produces: isolated development and production roots in distinct state keys;
  only development is eligible to apply in this plan

- [ ] **Step 1: Define the dedicated backend**

```hcl
bucket       = "portfolio-tofu-state-180294223248"
key          = "portfolio-lambda-http-api/dev/terraform.tfstate"
region       = "us-west-2"
encrypt      = true
use_lockfile = true
```

Production uses the same backend settings with key
`portfolio-lambda-http-api/prod/terraform.tfstate`. Use the same OpenTofu and
AWS provider constraints as the artifact root.

- [ ] **Step 2: Configure development values explicitly**

`dev.auto.tfvars`:

```hcl
environment                = "dev"
name_prefix                = "portfolio-lambda-dev"
aws_region                 = "us-west-2"
lambda_memory_mb           = 512
lambda_timeout_seconds     = 29
reserved_concurrency       = 5
log_retention_days         = 14
enable_pitr                = false
enable_deletion_protection = false
alarm_action_arns          = []
domain_names               = ["dev.craigdevjohnson.com"]
request_custom_domain      = false
activate_custom_domain     = false
```

`ecr_repository_url` and `image_digest` are required variables with no defaults
and are supplied at plan time. Do not put a mutable tag in this root.

Create `prod.auto.tfvars` in the same reviewed PR with:

```hcl
environment                = "prod"
name_prefix                = "portfolio-lambda-prod"
aws_region                 = "us-west-2"
lambda_memory_mb           = 512
lambda_timeout_seconds     = 29
reserved_concurrency       = 10
log_retention_days         = 90
enable_pitr                = true
enable_deletion_protection = true
domain_names               = ["craigdevjohnson.com", "www.craigdevjohnson.com"]
request_custom_domain      = false
activate_custom_domain     = false
```

The production root requires `alarm_action_arns` with no default and validates
that it contains at least one ARN. It is validated offline here but is not
initialized against its backend, planned, or applied until the production plan.

- [ ] **Step 3: Instantiate the service module**

Pass every root variable explicitly to `module "service"` at
`../../modules/service`. Add root outputs as direct aliases of the module's
non-secret outputs.

Extend `infra/lambda/layout_test.go` to require:

```go
	required = append(required,
	"modules/service/variables.tf",
	"modules/service/lambda.tf",
	"environments/dev/backend.hcl",
	"environments/dev/main.tf",
	"environments/prod/backend.hcl",
	"environments/prod/main.tf",
)
```

- [ ] **Step 4: Initialize locally without backend and validate**

```bash
export TF_VAR_ecr_repository_url=180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases
export TF_VAR_image_digest=sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
export TF_VAR_alarm_action_arns='["arn:aws:sns:us-west-2:180294223248:portfolio-lambda-prod-alerts"]'

tofu -chdir=infra/lambda/environments/dev init -backend=false -input=false
tofu -chdir=infra/lambda/environments/dev fmt -check
tofu -chdir=infra/lambda/environments/dev validate
tofu -chdir=infra/lambda/environments/prod init -backend=false -input=false
tofu -chdir=infra/lambda/environments/prod fmt -check
tofu -chdir=infra/lambda/environments/prod validate
go test ./infra/lambda -run TestLambdaInfrastructureLayout -count=1
```

The syntactically valid test digest is used only for offline validation and is never applied.

- [ ] **Step 5: Commit the module and development root**

```bash
git add -- infra/lambda/layout_test.go infra/lambda/environments/dev infra/lambda/environments/prod
git diff --cached --check
git commit -m "feat(infra): add isolated Lambda environment roots"
```

---

### Task 7: Add safe plan/apply and release commands plus infrastructure CI

**Files:**

- Modify: `Taskfile.yaml`
- Modify: `.github/workflows/ci.yml`
- Modify: `DEPLOY-INSTRUCTIONS.md`
- Modify: `docs/deployment/aws-lambda-api-gateway.md`
- Create: `scripts/record-lambda-observation.sh`
- Create: `scripts/record-lambda-workflow-proof.sh`
- Create: `scripts/check-lambda-observation-window.sh`
- Create: `scripts/check-lambda-plan.sh`
- Create: `tests/lambda-observation-window.sh`
- Create: `tests/lambda-plan-contract.sh`

**Interfaces:**

- Produces: `lambda-artifacts-init`, `lambda-artifacts-plan`, `lambda-artifacts-apply`, `lambda-release-push`, `lambda-dev-init`, `lambda-dev-plan`, `lambda-dev-apply`, `lambda-prod-init`, `lambda-prod-plan`, `lambda-prod-apply`, `lambda-plan-check`, `lambda-dev-observation-sample`, `lambda-dev-observation-workflow`, `lambda-dev-observation-gate`, `lambda-prod-observation-sample`, `lambda-prod-observation-workflow`, `lambda-prod-observation-gate`
- Consumes: named `PLAN_FILE`, `APPROVED_PLAN_SHA256`, `IMAGE_DIGEST`, and
  non-root `AWS_PROFILE`

- [ ] **Step 1: Add one reusable non-root identity guard**

The private Task command runs:

```sh
set -eu
test "$AWS_PROFILE" = "portfolio-deployer"
test "$AWS_REGION" = "us-west-2"
test -z "${AWS_ACCESS_KEY_ID+x}${AWS_SECRET_ACCESS_KEY+x}${AWS_SESSION_TOKEN+x}"
arn=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" \
  sts get-caller-identity --query Arn --output text)
account=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" \
  sts get-caller-identity --query Account --output text)
test "$account" = "180294223248"
case "$arn" in
  *:root) echo "Refusing AWS root identity" >&2; exit 1 ;;
  *:assumed-role/AWSReservedSSO_PortfolioDeployer_*) ;;
  *) echo "Refusing unexpected deployment role" >&2; exit 1 ;;
esac
```

Every artifact, ECR, development, and production mutation command depends on this guard.

- [ ] **Step 2: Keep initialization and planning separate from apply**

Each init command uses the root's `backend.hcl`, `-reconfigure`, `-input=false`,
and verifies `tofu workspace show` equals `default`.

Each plan command requires an absolute `PLAN_FILE`, refuses an existing file,
uses `-lock-timeout=5m -input=false -out="$PLAN_FILE"`, runs the JSON plan
checker, then runs `tofu show -no-color "$PLAN_FILE"`.

Before every artifact, development, or production plan and apply, a reusable
guard compares `APPROVED_STATE_LOCK_URI` with the exact root lock URI. The
controller obtains current-session approval for the exact S3 lock write before
setting it. Initialization does not set this acknowledgement and never
implicitly plans or applies.

The only accepted values are:

- `s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/artifacts/terraform.tfstate.tflock`;
- `s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/dev/terraform.tfstate.tflock`;
- `s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/prod/terraform.tfstate.tflock`.

Each apply command requires an existing `PLAN_FILE`, requires the exact
separately approved `APPROVED_PLAN_SHA256`, recomputes the file's SHA-256
digest immediately before apply, and runs only:

```sh
tofu -chdir="$root" apply -input=false "$PLAN_FILE"
```

No command contains `--auto-approve`, `-target`, or an implicit plan-and-apply sequence.

- [ ] **Step 3: Add the immutable release push command**

Require a clean Git worktree and full `BUILD_REVISION=$(git rev-parse HEAD)`.
Derive `release_tag=git-${BUILD_REVISION}`. Read the repository URL from the
artifact root output, build with `task build-lambda-image`, log into the derived
registry, push the immutable tag, wait for ECR's scan to complete, and print a
release record built from these current APIs:

```bash
: "${release_tag:?derive the immutable release tag first}"
aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ecr describe-images \
  --repository-name portfolio-lambda-releases \
  --image-ids imageTag="$release_tag" \
  --query 'imageDetails[0].{Digest:imageDigest,PushedAt:imagePushedAt}' \
  --output json
aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ecr wait image-scan-complete \
  --repository-name portfolio-lambda-releases \
  --image-id imageTag="$release_tag"
aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ecr describe-image-scan-findings \
  --repository-name portfolio-lambda-releases \
  --image-id imageTag="$release_tag" \
  --no-paginate \
  --query '{ScanStatus:imageScanStatus.status,FindingSeverityCounts:imageScanFindings.findingSeverityCounts}' \
  --output json
```

Before build or push, the task verifies repository tag immutability and proves
the tag is absent. It fails closed on every lookup error except the specific
`ImageNotFoundException`, stops without pushing if the tag already exists, and
fails if the current scan cannot be read or does not complete successfully.
Immediately after push, it may retry only the exact `ScanNotFoundException`
from `DescribeImageScanFindings` for a bounded interval while ECR creates the
scan record; every other scan lookup error fails immediately.
Both metadata-only `DescribeImageScanFindings` reads use `--no-paginate` so the
AWS CLI does not try to reconstruct optional nested finding lists while the
scan record is still incomplete. The native waiter remains responsible for
polling the scan to `COMPLETE` or failing on `FAILED`.

- [ ] **Step 4: Extend CI with offline infrastructure validation**

Add an `infrastructure` job using `opentofu/setup-opentofu` at its reviewed
current major and OpenTofu `1.12.6`. Run recursive formatting, artifact
`init -backend=false` plus validate, and both environment roots with
`init -backend=false` plus validate. Supply the same syntactic digest and a
syntactically valid production SNS ARN used in Task 6. Do not configure AWS
credentials in GitHub Actions.

- [ ] **Step 5: Implement an offline-tested observation recorder and gate**

`record-lambda-observation.sh` requires explicit `RELEASE_RECORD` and
`EVIDENCE_FILE` paths and the non-root deployment profile. It resolves the
environment's `rollback_evidence` path from the release JSON and loads the
recorded legacy origin from that machine-readable file. The URL must be a
strict HTTPS origin without user info, a path, a query, a fragment, or encoded
delimiters. Production also validates the confirmed-subscription count,
message ID, ordered timestamps within five minutes and before cutover, and
64-hex receipt-token hash from `alarm_delivery_evidence`. It proves the topic
ARN is the sole action on all five live alarms; no caller supplies an
unrecorded origin or notification claim. Each JSONL sample re-derives and
records health revision, ECR digest,
published version, alias target, primary route/CSS/binary-asset status and
content types, exactly five alarm states, Lambda Errors/Throttles/Duration p95,
API 5xx/Latency p95, and App Runner rollback-origin health. It must never record
cookies, secrets, bodies, query strings, OAuth tokens, or subscriber endpoints.

`record-lambda-workflow-proof.sh` accepts the same paths plus the environment,
public hostname, sanitized Google connect/add/sync request IDs, and explicit
OAuth, Secure-cookie, add, and sync pass flags. It revalidates the live release
coordinates, rejects a missing request ID or false flag, and appends a
`kind=workflow` JSONL record without cookies, calendar names, event bodies, JWTs,
OAuth codes, or tokens. The two environment-specific workflow Task commands
wrap it. Every observation sample, workflow, and gate Task command depends on
`_lambda-identity-check`; callers still select `AWS_PROFILE` and `AWS_REGION`
explicitly so each invocation is safe in a fresh shell. Each command also runs
the artifact init and its environment-specific init before reading OpenTofu
outputs, so an observation does not depend on a prior local `.terraform` cache.

`check-lambda-observation-window.sh` exits nonzero unless:

- at least eight passing samples exist;
- the first sample is no more than 26 hours after DNS cutover;
- the last sample is at least 604800 seconds after DNS cutover;
- no sample gap exceeds 26 hours;
- every recorded timestamp matches its epoch and is not in the future;
- every sample has the same SHA, digest, version, and alias target;
- no sample has an alarm in `ALARM` or an unresolved blocker;
- full OAuth, Secure-cookie, add, and sync checks passed at cutover and after
  seven full days with distinct request IDs; and
- the App Runner rollback origin still passes.

The same scripts support production through the two production Task commands and
its Amplify rollback origin. `tests/lambda-observation-window.sh` uses temporary
release, rollback, alarm-delivery, and JSONL fixture files
to prove too-short, missing-sample, long-gap, coordinate-drift, alarm, workflow,
and rollback-origin failures plus one exact seven-day pass. It performs no
network or AWS calls. Wire the scripts through the two named Task commands and
run the offline test in hosted infrastructure CI.

`check-lambda-plan.sh` requires a plan JSON path, environment, exact name prefix,
digest-qualified image URI, and expected alarm-action JSON. It exits nonzero on
delete/replace actions, legacy/App Runner/Amplify addresses or names, mutable
tags, secret values, incorrect table protection/retention, wrong name-bearing
resource prefixes, an address/type outside the exact environment allowlist, or
an alarm action mismatch. The allowlist includes only the reviewed conditional
domain addresses. `tests/lambda-plan-contract.sh` uses synthetic plan JSON to
prove every rejection and one dev/prod pass without AWS access.

- [ ] **Step 6: Rewrite deployment docs around the new roots**

Mark legacy `task deploy`, `deploy-lambda`, `redeploy`, and `redeploy-lambda` as
legacy-state commands. Document the exact replacement plan/apply split, all
three replacement state keys, SSM path names, immutable tag/digest record,
direct-endpoint proof, and custom-domain two-stage workflow.

- [ ] **Step 7: Run the full non-mutating gate and commit**

```bash
task ci
tofu fmt -check -recursive infra/lambda
go test ./infra/lambda -count=1
sh tests/lambda-observation-window.sh
sh tests/lambda-plan-contract.sh
git diff --check
```

```bash
git add -- Taskfile.yaml .github/workflows/ci.yml DEPLOY-INSTRUCTIONS.md \
  docs/deployment/aws-lambda-api-gateway.md \
  docs/superpowers/plans/2026-08-21-development-lambda-cutover.md \
  scripts/record-lambda-observation.sh \
  scripts/record-lambda-workflow-proof.sh \
  scripts/check-lambda-observation-window.sh scripts/check-lambda-plan.sh \
  tests/lambda-observation-window.sh tests/lambda-plan-contract.sh
git commit -m "build(infra): separate Lambda plans from applies"
```

---

### Task 8: Self-review the infrastructure contract and update the draft PR

**Files:**

- Review: all `infra/lambda/**/*.tf`, lock files, Taskfile, workflow, and deployment docs
- Modify: draft PR branch through a normal push

**Interfaces:**

- Consumes: Tasks 2-7
- Produces: locally valid, convergent infrastructure source ready for an AWS plan

- [ ] **Step 1: Scan for forbidden deployment patterns**

```bash
test -z "$(rg -n -- '--auto-approve|-target=|lambda-latest|:latest' infra/lambda || true)"
test -z "$(rg -n 'aws_apprunner|aws_amplify' infra/lambda || true)"
test -z "$(rg -n 'value *=.*(secret|password|token)|with_decryption *= *true' infra/lambda -i || true)"
rg -n 'portfolio-lambda-http-api/(artifacts|dev|prod)/terraform\.tfstate' \
  infra/lambda/artifacts/backend.hcl \
  infra/lambda/environments/dev/backend.hcl \
  infra/lambda/environments/prod/backend.hcl
rg -n -- '--auto-approve|-target=|lambda-latest|:latest' Taskfile.yaml
```

Expected: all three forbidden scans are empty; exactly the three approved backend
keys are found; Taskfile matches only legacy commands explicitly labeled as such
in deployment docs.

- [ ] **Step 2: Validate every root from a fresh plugin directory**

```bash
plugin_cache=$(mktemp -d)
export TF_PLUGIN_CACHE_DIR="$plugin_cache"
export TF_VAR_ecr_repository_url=180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases
export TF_VAR_image_digest=sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
export TF_VAR_alarm_action_arns='["arn:aws:sns:us-west-2:180294223248:portfolio-lambda-prod-alerts"]'

tofu fmt -check -recursive infra/lambda
tofu -chdir=infra/lambda/artifacts init -backend=false -input=false
tofu -chdir=infra/lambda/artifacts validate
tofu -chdir=infra/lambda/environments/dev init -backend=false -input=false
tofu -chdir=infra/lambda/environments/dev validate
tofu -chdir=infra/lambda/environments/prod init -backend=false -input=false
tofu -chdir=infra/lambda/environments/prod validate
go test ./infra/lambda -count=1

rmdir "$plugin_cache" 2>/dev/null || true
```

- [ ] **Step 3: Run application, race, and image gates at the exact PR head**

```bash
task ci
go test -race ./cmd/lambda ./internal/httpx ./internal/google ./internal/soccer ./internal/app
task test-images
git status --short
```

Expected: all pass and status is clean.

- [ ] **Step 4: Push and wait for the draft PR checks**

**Approval gate:** Stop and present the exact branch, commit SHA, repository,
and hosted changes. Push only after the user approves that GitHub mutation in
the current execution session.

```bash
git push
pr_url=$(gh pr view --json url --jq .url)
gh pr checks "$pr_url" --watch --interval 10
```

Expected: application and infrastructure jobs succeed.

---

### Task 9: Plan and apply the immutable artifact root

**Files:**

- No repository changes
- Create temporary saved plan outside the repository
- Create AWS resources only after the plan receives exact approval

**Interfaces:**

- Produces: ECR repository `portfolio-lambda-releases`, its lifecycle policy,
  and its Lambda pull repository policy
- Preserves: all legacy ECR repositories and state

- [ ] **Step 1: Re-run identity and legacy-state gates**

Run the exact identity guard from Task 1, state-bucket versioning verification,
and the legacy-state `head-object` query. Compare its metadata with the recorded
pre-work value. Stop unless versioning reports `Enabled`.

- [ ] **Step 2: Initialize the artifact backend**

Run: `task lambda-artifacts-init`

Expected: default workspace and backend key `portfolio-lambda-http-api/artifacts/terraform.tfstate`.

- [ ] **Step 3: Create and inspect a saved artifact plan**

**Approval gate:** Present the artifact lock URI and obtain current-session
approval for its create/delete write before setting
`APPROVED_STATE_LOCK_URI`. This approval does not authorize applying the plan.

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
tofu -chdir=infra/lambda/artifacts show -json "$artifact_plan" | \
  jq -r '.resource_changes[] | [.address, (.change.actions | join(","))] | @tsv'
```

Expected: create actions only for `aws_ecr_repository.lambda_releases`,
`aws_ecr_lifecycle_policy.lambda_releases`, and
`aws_ecr_repository_policy.lambda_releases`; no delete, replace, App Runner,
Amplify, DynamoDB, IAM, Lambda, or API address.

- [ ] **Step 4: Apply only the approved saved plan**

**Approval gate:** Stop and present the saved plan's absolute path, checksum,
complete human-readable create list, repository name, legacy-state metadata,
and exact artifact lock URI. Obtain separate current-session approval for the
saved-plan apply and its lock write. Then reload the reviewed absolute path in a
fresh shell and apply only it:

```bash
: "${APPROVED_ARTIFACT_PLAN:?set the exact approved artifact plan path}"
: "${APPROVED_PLAN_SHA256:?set the exact approved artifact plan checksum}"
test -f "$APPROVED_ARTIFACT_PLAN"
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task lambda-artifacts-init
task lambda-artifacts-apply \
  PLAN_FILE="$APPROVED_ARTIFACT_PLAN" \
  APPROVED_PLAN_SHA256="$APPROVED_PLAN_SHA256" \
  APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/artifacts/terraform.tfstate.tflock
```

- [ ] **Step 5: Verify convergence and repository settings**

**Approval gate:** A convergence plan also writes the native lock object.
Present the same exact artifact lock URI and obtain fresh current-session
approval before running it.

```bash
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task lambda-artifacts-init
convergence_dir=$(mktemp -d)
convergence_plan="$convergence_dir/artifacts-convergence.tfplan"
export APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/artifacts/terraform.tfstate.tflock
task lambda-artifacts-plan PLAN_FILE="$convergence_plan"
tofu -chdir=infra/lambda/artifacts show -json "$convergence_plan" | \
  jq -e '[.resource_changes[]? | select(.change.actions != ["no-op"])] | length == 0'
aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ecr describe-repositories \
  --repository-names portfolio-lambda-releases \
  --query 'repositories[0].{Name:repositoryName,Mutability:imageTagMutability,Scan:imageScanningConfiguration.scanOnPush,Encryption:encryptionConfiguration.encryptionType}' \
  --output json
rm -- "$convergence_plan"
rmdir -- "$convergence_dir"
```

Expected: convergence plan has no changes; ECR reports `IMMUTABLE`, scan on push, and AES256.

- [ ] **Step 6: Remove only the private temporary plans**

```bash
: "${APPROVED_ARTIFACT_PLAN:?set the exact applied artifact plan path}"
test "$(basename "$APPROVED_ARTIFACT_PLAN")" = "artifacts.tfplan"
rm -- "$APPROVED_ARTIFACT_PLAN"
rmdir -- "$(dirname "$APPROVED_ARTIFACT_PLAN")"
```

---

### Task 10: Push one immutable image and provision development secrets

**Files:**

- No repository changes
- Create three environment-specific SSM SecureStrings

**Interfaces:**

- Produces: an image tagged with `git-` plus the full 40-character source SHA, its digest, and `/portfolio/lambda/dev/*` values
- Preserves: `/portfolio/*` source parameters without updating them

- [ ] **Step 1: Build, test, push, and capture the release digest**

**Approval gate:** First run the non-mutating image tests and present the full
source SHA, exact `git-` plus full-SHA tag, destination repository, and push command.
Run the ECR login and image push only after the user approves that exact image
mutation in the current execution session.

```bash
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task test-images
task lambda-release-push

release_sha=$(git rev-parse HEAD)
release_tag="git-${release_sha}"
release_digest=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ecr describe-images \
  --repository-name portfolio-lambda-releases \
  --image-ids imageTag="$release_tag" \
  --query 'imageDetails[0].imageDigest' --output text)
test "$(printf '%s' "$release_digest" | grep -Ec '^sha256:[0-9a-f]{64}$')" = "1"
echo "$release_sha $release_tag $release_digest"
```

Expected: one scan-on-push image with a full commit tag and a SHA-256 digest.

- [ ] **Step 2: Create a new development session key without printing it**

**Approval gate:** Stop and present the three exact target parameter paths, that
`LPS_SESSION_KEY` is newly generated, and that the OAuth values are copied from
the two named legacy paths without exposing them. Continue only after approval
of the new session-key SecureString create in the current execution session.

```bash
set -euo pipefail
set +x
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task lambda-artifacts-init
assert_aws_cli_history_disabled() {
  cli_history_value=
  if cli_history_value=$(aws --profile "$AWS_PROFILE" configure get cli_history 2>/dev/null); then
    test "$cli_history_value" = disabled || {
      echo "AWS CLI history must be unset or disabled before handling secrets" >&2
      return 1
    }
  else
    cli_history_rc=$?
    test "$cli_history_rc" -eq 1 && test -z "$cli_history_value" || {
      echo "Unable to prove AWS CLI history is disabled" >&2
      return 1
    }
  fi
}
assert_aws_cli_history_disabled
test "$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ssm get-parameter \
  --name /portfolio/lambda/dev/LPS_SESSION_KEY --query Parameter.Name --output text 2>/dev/null || true)" = ""
openssl rand -hex 32 | \
  tr -d '\n' | \
  aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ssm put-parameter \
    --name /portfolio/lambda/dev/LPS_SESSION_KEY \
    --type SecureString \
    --no-overwrite \
    --value file:///dev/stdin >/dev/null
```

- [ ] **Step 3: Copy the existing public OAuth client ID and secret through a pipe**

**Approval gate:** Present the two exact source-to-target mappings and confirm
both targets are absent. Copy them only after separate current-session approval.
If any earlier write partially succeeded, inventory exact targets and request
fresh approval for only the missing path; never enable overwrite.

```bash
set -euo pipefail
set +x
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task lambda-artifacts-init
assert_aws_cli_history_disabled() {
  cli_history_value=
  if cli_history_value=$(aws --profile "$AWS_PROFILE" configure get cli_history 2>/dev/null); then
    test "$cli_history_value" = disabled || {
      echo "AWS CLI history must be unset or disabled before handling secrets" >&2
      return 1
    }
  else
    cli_history_rc=$?
    test "$cli_history_rc" -eq 1 && test -z "$cli_history_value" || {
      echo "Unable to prove AWS CLI history is disabled" >&2
      return 1
    }
  fi
}
assert_aws_cli_history_disabled
for name in CLIENT_ID_KEY CLIENT_SECRET_KEY; do
  test "$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ssm get-parameter \
    --name "/portfolio/lambda/dev/$name" --query Parameter.Name --output text 2>/dev/null || true)" = ""
  aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ssm get-parameter \
    --name "/portfolio/$name" \
    --with-decryption \
    --output json | \
  jq -je '.Parameter.Value | strings' | \
  aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ssm put-parameter \
    --name "/portfolio/lambda/dev/$name" \
    --type SecureString \
    --no-overwrite \
    --value file:///dev/stdin >/dev/null
done
```

No command writes a decrypted value to the terminal, a file, process argument,
OpenTofu state, or Git. If a target already exists, stop and inspect metadata
instead of adding `Overwrite=true`.
Each secret block fails before generation or decryption unless the selected
profile's AWS CLI history setting is either unset or explicitly `disabled`, so
request and response data cannot be persisted to the CLI history database.
The deployer allow uses `StringEqualsIfExists` for `ssm:Overwrite=false`
because the first create may omit that request context key; a separate explicit
deny still rejects `ssm:Overwrite=true`, and all three commands also pass
`--no-overwrite`.

- [ ] **Step 4: Verify parameter metadata without values**

```bash
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task lambda-artifacts-init
aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ssm describe-parameters \
  --parameter-filters Key=Name,Option=BeginsWith,Values=/portfolio/lambda/dev/ \
  --query 'Parameters[].{Name:Name,Type:Type,KeyId:KeyId,LastModifiedDate:LastModifiedDate}' \
  --output table
```

Expected: exactly three SecureStrings.

---

### Task 11: Plan, apply, and verify the direct development endpoint

**Files:**

- No repository changes
- Create AWS resources only from the approved saved development plan

**Interfaces:**

- Consumes: artifact repository URL and recorded digest
- Produces: development Lambda/API/tables/IAM/logs/alarms and execute-api endpoint

- [ ] **Step 1: Initialize the development backend and export release inputs**

```bash
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task lambda-artifacts-init
task lambda-dev-init
TF_VAR_ecr_repository_url=$(tofu -chdir=infra/lambda/artifacts output -raw ecr_repository_url)
export TF_VAR_ecr_repository_url
release_sha=$(git rev-parse HEAD)
release_tag="git-${release_sha}"
release_digest=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ecr describe-images \
  --repository-name portfolio-lambda-releases \
  --image-ids imageTag="$release_tag" \
  --query 'imageDetails[0].imageDigest' --output text)
test "$(printf '%s' "$release_digest" | grep -Ec '^sha256:[0-9a-f]{64}$')" = "1"
export TF_VAR_image_digest="$release_digest"
```

- [ ] **Step 2: Create and mechanically inspect the saved plan**

**Approval gate:** Present the exact development lock URI and obtain
current-session approval for its create/delete write before setting
`APPROVED_STATE_LOCK_URI`. This approval does not authorize applying the saved
plan.

```bash
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task lambda-artifacts-init
task lambda-dev-init
release_sha=$(git rev-parse HEAD)
release_digest=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ecr describe-images \
  --repository-name portfolio-lambda-releases \
  --image-ids imageTag="git-${release_sha}" \
  --query 'imageDetails[0].imageDigest' --output text)
repository_url=$(tofu -chdir=infra/lambda/artifacts output -raw ecr_repository_url)
plan_dir=$(mktemp -d)
dev_plan="$plan_dir/dev.tfplan"
dev_plan_json="$dev_plan.json"
task lambda-dev-plan \
  PLAN_FILE="$dev_plan" \
  IMAGE_DIGEST="$release_digest" \
  APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/dev/terraform.tfstate.tflock
dev_plan_sha256=$(shasum -a 256 "$dev_plan" | awk '{print $1}')
printf 'dev_plan_sha256=%s\n' "$dev_plan_sha256"

tofu -chdir=infra/lambda/environments/dev show -json "$dev_plan" > "$dev_plan_json"
jq -r '.resource_changes[] | [.address, (.change.actions | join(","))] | @tsv' "$dev_plan_json"
task lambda-plan-check \
  PLAN_JSON="$dev_plan_json" \
  ENVIRONMENT=dev \
  NAME_PREFIX=portfolio-lambda-dev \
  IMAGE_URI="$repository_url@$release_digest" \
  EXPECTED_ALARM_ACTIONS_JSON='[]'
```

Expected: the checker enforces the no-legacy/no-delete contract, exact
digest-qualified image, development table/log settings, empty alarm actions,
and prefixes only on name-bearing workload fields.

- [ ] **Step 3: Review the human-readable plan and apply exactly it**

**Approval gate:** Stop and present the saved plan's absolute path and checksum,
the complete action list, the digest-qualified image URI, mechanical safety
assertions, and exact development lock URI. Obtain separate current-session
approval for the saved-plan apply and its lock write. Then reload the reviewed
path in a fresh shell and apply only it:

```bash
: "${APPROVED_DEV_PLAN:?set the exact approved development plan path}"
: "${APPROVED_PLAN_SHA256:?set the exact approved development plan checksum}"
test -f "$APPROVED_DEV_PLAN"
test -f "$APPROVED_DEV_PLAN.json"
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task lambda-artifacts-init
task lambda-dev-init
task lambda-dev-apply \
  PLAN_FILE="$APPROVED_DEV_PLAN" \
  APPROVED_PLAN_SHA256="$APPROVED_PLAN_SHA256" \
  APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/dev/terraform.tfstate.tflock
```

- [ ] **Step 4: Wait for the function and verify version/alias coordinates**

```bash
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task lambda-dev-init
function_name=$(tofu -chdir=infra/lambda/environments/dev output -raw lambda_function_name)

aws --profile "$AWS_PROFILE" --region "$AWS_REGION" lambda wait function-active-v2 --function-name "$function_name"
aws --profile "$AWS_PROFILE" --region "$AWS_REGION" lambda get-alias --function-name "$function_name" --name live \
  --query '{Alias:Name,Version:FunctionVersion,RevisionId:RevisionId}' --output json
```

- [ ] **Step 5: Exercise public application and binary assets**

```bash
base_url=$(tofu -chdir=infra/lambda/environments/dev output -raw api_default_url)
release_sha=$(git rev-parse HEAD)
test "$(curl --fail --show-error --silent "$base_url/healthz" | jq -er .revision)" = "$release_sha"

check_content() {
  url=$1
  expected=$2
  test "$(curl --show-error --silent --output /dev/null --write-out '%{http_code}' "$url")" = "200"
  content_type=$(curl --show-error --silent --output /dev/null --write-out '%{content_type}' "$url")
  case "$content_type" in
    "$expected"*) ;;
    *) echo "unexpected content type for $url: $content_type" >&2; return 1 ;;
  esac
}

check_content "$base_url/healthz" "application/json"
check_content "$base_url/" "text/html"
check_content "$base_url/soccer" "text/html"
check_content "$base_url/static/css/tailwind.css" "text/css"
check_content "$base_url/static/images/backgrounds/home-hero.jpg" "image/jpeg"
```

Expected: `/healthz` revision equals `release_sha`; every other request is 2xx with the expected content type.

- [ ] **Step 6: Verify logs, alarms, and convergence**

Inspect the latest Lambda and API access records without cookies, headers, query
strings, or bodies. Confirm all five alarms exist and are `OK` or
`INSUFFICIENT_DATA`, not `ALARM`. Create and inspect the convergence plan:

**Approval gate:** Obtain fresh current-session approval for the exact
development lock URI before the convergence plan.

```bash
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task lambda-artifacts-init
task lambda-dev-init
function_name=$(tofu -chdir=infra/lambda/environments/dev output -raw lambda_function_name)
live_version=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" lambda get-alias \
  --function-name "$function_name" --name live --query FunctionVersion --output text)
live_image_uri=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" lambda get-function \
  --function-name "$function_name" --qualifier "$live_version" --query Code.ImageUri --output text)
release_digest=${live_image_uri##*@}
convergence_dir=$(mktemp -d)
convergence_plan="$convergence_dir/dev-convergence.tfplan"
task lambda-dev-plan \
  PLAN_FILE="$convergence_plan" \
  IMAGE_DIGEST="$release_digest" \
  APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/dev/terraform.tfstate.tflock
tofu -chdir=infra/lambda/environments/dev show -json "$convergence_plan" | \
  jq -e '[.resource_changes[]? | select(.change.actions != ["no-op"])] | length == 0'
rm -- "$convergence_plan"
rmdir -- "$convergence_dir"
```

- [ ] **Step 7: Remove only temporary plan artifacts**

```bash
: "${APPROVED_DEV_PLAN:?set the exact applied development plan path}"
test "$(basename "$APPROVED_DEV_PLAN")" = "dev.tfplan"
rm -- "$APPROVED_DEV_PLAN" "$APPROVED_DEV_PLAN.json"
rmdir -- "$(dirname "$APPROVED_DEV_PLAN")"
```

---

### Task 12: Prove OAuth, attach the dev domain, merge, and cut DNS

**Files:**

- Modify: `infra/lambda/environments/dev/dev.auto.tfvars`
- Commit: domain request and activation state
- Create: `docs/deployment/evidence/development-precutover.json`
- Create under `docs/deployment/evidence/releases/`: one JSON file named with
  the actual 40-character source SHA
- Modify externally: Google OAuth redirect allowlist and exact Cloudflare records

**Interfaces:**

- Consumes: accepted direct endpoint and recorded App Runner rollback target
- Produces: `dev.craigdevjohnson.com` on the Lambda custom domain, a merged
  replacement PR, and one durable immutable release record

- [ ] **Step 1: Add the direct execute-api Soccer callback to Google OAuth**

Re-derive the endpoint in this task:

```bash
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task lambda-dev-init
base_url=$(tofu -chdir=infra/lambda/environments/dev output -raw api_default_url)
direct_oauth_uri="${base_url%/}/soccer"
printf '%s\n' "$direct_oauth_uri"
```

**Approval gate:** Stop and present the complete current redirect list and the
single exact URI to add. Use the authenticated Google Cloud console only after
the user approves that OAuth mutation in the current execution session. Preserve
all existing redirect URIs and record the sanitized before/after lists.

- [ ] **Step 2: Complete the direct-endpoint workflow**

In a real browser, verify home and all primary navigation routes, import one LPS
JWT, complete Google connect/callback, and select a calendar. Before writing the
calendar, present the exact calendar, chosen game, canonical event ID, and sync
scope. **Approval gate:** add and sync only after current-session approval.
Confirm all auth cookies are `Secure`, logs contain the
request IDs, and the source URL recorded in Google is HTTPS.

- [ ] **Step 3: Request the certificate through a reviewed plan**

Change only `request_custom_domain = true` and commit
`feat(infra): request the development API certificate`. Create the saved plan
locally with the digest already serving behind the `live` alias, not a tag
derived from the now-advanced Git `HEAD`:

**Approval gate:** Present the exact development lock URI and obtain fresh
current-session approval for its create/delete write before the certificate
plan. This approval does not authorize the later apply.

```bash
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task lambda-artifacts-init
task lambda-dev-init
function_name=$(tofu -chdir=infra/lambda/environments/dev output -raw lambda_function_name)
live_version=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" lambda get-alias \
  --function-name "$function_name" --name live --query FunctionVersion --output text)
image_uri=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" lambda get-function \
  --function-name "$function_name" --qualifier "$live_version" --query Code.ImageUri --output text)
release_digest=${image_uri##*@}
test "$(printf '%s' "$release_digest" | grep -Ec '^sha256:[0-9a-f]{64}$')" = "1"
certificate_plan_dir=$(mktemp -d)
certificate_plan="$certificate_plan_dir/dev-certificate.tfplan"
task lambda-dev-plan \
  PLAN_FILE="$certificate_plan" \
  IMAGE_DIGEST="$release_digest" \
  APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/dev/terraform.tfstate.tflock
certificate_plan_sha256=$(shasum -a 256 "$certificate_plan" | awk '{print $1}')
printf 'certificate_plan_sha256=%s\n' "$certificate_plan_sha256"
tofu -chdir=infra/lambda/environments/dev show -json "$certificate_plan" > \
  "$certificate_plan_dir/dev-certificate.json"
jq -e '[.resource_changes[] | select(.change.actions != ["no-op"])] as $changes |
  ($changes | length) == 1 and
  ($changes[0].address | test("aws_acm_certificate")) and
  $changes[0].change.actions == ["create"]' \
  "$certificate_plan_dir/dev-certificate.json"
```

**Approval gate:** Present the exact commit, branch, and push before changing
GitHub. After hosted CI passes, separately present the saved plan path, checksum,
full change list, and exact development lock URI. Apply that exact plan only
after the user separately approves the saved-plan apply and lock write in the
current execution session. No Lambda replacement or alias movement is allowed.

```bash
: "${APPROVED_CERTIFICATE_PLAN:?set the exact approved certificate plan path}"
: "${APPROVED_PLAN_SHA256:?set the exact approved certificate plan checksum}"
test -f "$APPROVED_CERTIFICATE_PLAN"
test -f "$(dirname "$APPROVED_CERTIFICATE_PLAN")/dev-certificate.json"
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task lambda-dev-init
task lambda-dev-apply \
  PLAN_FILE="$APPROVED_CERTIFICATE_PLAN" \
  APPROVED_PLAN_SHA256="$APPROVED_PLAN_SHA256" \
  APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/dev/terraform.tfstate.tflock
```

- [ ] **Step 4: Add ACM validation records to Cloudflare**

Read `acm_validation_records` from OpenTofu and prepare an exact list containing
record type, name, target, TTL, and `proxied=false` for every CNAME.

**Approval gate:** Create only that reviewed list after current-session approval.
Preserve the records permanently for ACM renewal. Use the explicit non-root AWS
profile when waiting for `aws acm describe-certificate` to report `ISSUED`.

- [ ] **Step 5: Activate the API Gateway custom domain**

Change only `activate_custom_domain = true` and commit
`feat(infra): activate the development API domain`. Create the saved plan and
require only certificate validation, the Regional API Gateway domain, and API
mapping; no function replacement. Re-derive the digest from the live version in
this fresh step:

**Approval gate:** Present the exact development lock URI and obtain fresh
current-session approval for its create/delete write before the activation
plan. This approval does not authorize the later apply.

```bash
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task lambda-artifacts-init
task lambda-dev-init
function_name=$(tofu -chdir=infra/lambda/environments/dev output -raw lambda_function_name)
live_version=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" lambda get-alias \
  --function-name "$function_name" --name live --query FunctionVersion --output text)
image_uri=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" lambda get-function \
  --function-name "$function_name" --qualifier "$live_version" --query Code.ImageUri --output text)
release_digest=${image_uri##*@}
test "$(printf '%s' "$release_digest" | grep -Ec '^sha256:[0-9a-f]{64}$')" = "1"
activation_plan_dir=$(mktemp -d)
activation_plan="$activation_plan_dir/dev-domain-activation.tfplan"
task lambda-dev-plan \
  PLAN_FILE="$activation_plan" \
  IMAGE_DIGEST="$release_digest" \
  APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/dev/terraform.tfstate.tflock
activation_plan_sha256=$(shasum -a 256 "$activation_plan" | awk '{print $1}')
printf 'activation_plan_sha256=%s\n' "$activation_plan_sha256"
tofu -chdir=infra/lambda/environments/dev show -json "$activation_plan" > \
  "$activation_plan_dir/dev-domain-activation.json"
jq -e '[.resource_changes[] | select(.change.actions != ["no-op"])] as $changes |
  ($changes | length) >= 3 and
  all($changes[];
    (.address | test("aws_acm_certificate_validation|aws_apigatewayv2_domain_name|aws_apigatewayv2_api_mapping")) and
    .change.actions == ["create"])' "$activation_plan_dir/dev-domain-activation.json"
```

**Approval gate:** Present and obtain separate approvals for the GitHub push and
the exact saved AWS plan before either mutation. Present the exact development
lock URI and obtain separate current-session lock-write approval. Wait for
hosted CI before the apply.

```bash
: "${APPROVED_ACTIVATION_PLAN:?set the exact approved domain-activation plan path}"
: "${APPROVED_PLAN_SHA256:?set the exact approved domain-activation plan checksum}"
test -f "$APPROVED_ACTIVATION_PLAN"
test -f "$(dirname "$APPROVED_ACTIVATION_PLAN")/dev-domain-activation.json"
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task lambda-dev-init
task lambda-dev-apply \
  PLAN_FILE="$APPROVED_ACTIVATION_PLAN" \
  APPROVED_PLAN_SHA256="$APPROVED_PLAN_SHA256" \
  APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/dev/terraform.tfstate.tflock
```

- [ ] **Step 6: Add and test the custom-domain OAuth callback before traffic cutover**

**Approval gate:** Present the entire current redirect list and the one addition
`https://dev.craigdevjohnson.com/soccer`. Add it only after current-session
approval. Resolve the API Gateway target without public DNS by using
`curl --connect-to` or a temporary local hosts mapping, then prove the OAuth
redirect construction before changing traffic.

- [ ] **Step 7: Capture the current App Runner DNS rollback record**

Export the `dev.craigdevjohnson.com` Cloudflare traffic record including record
ID, type, name, content, TTL, and proxy state. Also record the App Runner default
service URL. Verify that rollback origin returns 200 at `/`, `/soccer`, and one
static asset. Before any development traffic-record mutation, use `apply_patch` to create
`docs/deployment/evidence/development-precutover.json` with:

```bash
mkdir -p docs/deployment/evidence
```

- `schema_version: 1`, `environment: "development"`, the RFC3339 `captured_at`,
  and `public_hostname: "dev.craigdevjohnson.com"`;
- `rollback_origin_url`: the exact App Runner default service URL; and
- `dns_record`: exact `id`, `type`, `name`, `content`, numeric `ttl`, and boolean
  `proxied` fields.

Validate the types, hostname, nonempty origin, and that the recorded origin
still passes all three probes. Commit only that file as
`docs: record development rollback coordinates`.

- [ ] **Step 8: Mark the replacement PR ready and merge the tested branch**

**Approval gate:** Present the exact rollback-evidence commit, branch, remote,
and push command. Push it only after current-session approval, then wait for the
hosted checks before evaluating merge readiness.

```bash
git push
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task lambda-dev-init
pr_url=$(gh pr view --json url --jq .url)
pr_head_sha=$(git rev-parse HEAD)
base_url=$(tofu -chdir=infra/lambda/environments/dev output -raw api_default_url)
release_sha=$(curl --fail --show-error --silent "$base_url/healthz" | jq -er .revision)
git merge-base --is-ancestor "$release_sha" "$pr_head_sha"
git diff --exit-code "$release_sha" "$pr_head_sha" -- \
  cmd internal types go.mod go.sum Dockerfile Dockerfile.lambda
for path in \
  infra/lambda/environments/prod/backend.hcl \
  infra/lambda/environments/prod/main.tf \
  infra/lambda/environments/prod/prod.auto.tfvars \
  infra/lambda/environments/prod/.terraform.lock.hcl
do
  git cat-file -e "$pr_head_sha:$path"
done
gh pr checks "$pr_url" --watch --interval 10
remote_head_sha=$(gh pr view "$pr_url" --json headRefOid --jq .headRefOid)
test "$remote_head_sha" = "$pr_head_sha"
gh pr view "$pr_url" --json number,headRefOid,mergeable,mergeStateStatus,statusCheckRollup,url
```

**Approval gate:** Stop and present the PR number, immutable release SHA, exact
PR head SHA, check
results including production offline validation, merge method, and resulting
branch policy. Only after current-session
approval run:

```bash
: "${APPROVED_PR_URL:?set the exact reviewed replacement PR URL}"
: "${APPROVED_PR_HEAD_SHA:?set the exact reviewed replacement PR head SHA}"
test "$(gh pr view "$APPROVED_PR_URL" --json headRefOid --jq .headRefOid)" = \
  "$APPROVED_PR_HEAD_SHA"
gh pr ready "$APPROVED_PR_URL"
test "$(gh pr view "$APPROVED_PR_URL" --json headRefOid --jq .headRefOid)" = \
  "$APPROVED_PR_HEAD_SHA"
gh pr merge "$APPROVED_PR_URL" --merge \
  --match-head-commit "$APPROVED_PR_HEAD_SHA"
```

Verify afterward that both SHAs are ancestors of the merge commit, that no
application/image-input path changed after the release SHA, and that
`origin/main` has the same application and infrastructure tree. The release SHA
need not be a direct merge parent because later commits change only reviewed
infrastructure configuration.

- [ ] **Step 9: Switch only the development traffic record**

Prepare a one-record mutation from the saved App Runner value to the exact
OpenTofu `api_gateway_domain_targets` CNAME target, TTL Auto, proxy disabled.

**Approval gate:** Present the record ID and complete before/after values. Apply
only after current-session approval. Do not alter the ACM validation records,
apex, `www`, App Runner, Amplify, SSM, or DynamoDB.

- [ ] **Step 10: Verify externally and handle stale cache explicitly**

Verify TLS certificate, `/healthz` revision, every primary route, CSS, binary
assets, Soccer import, full Google OAuth, add, sync, Lambda/API logs, and alarm
state from the public hostname.

If an old edge object appears, generate and review an exact URL list containing
only affected `https://dev.craigdevjohnson.com/...` URLs. **Approval gate:** purge
only that list after current-session approval; never use a zone-wide purge.

- [ ] **Step 11: Create the machine-validated immutable development release record**

Re-derive every value instead of using variables from earlier tasks:

```bash
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task lambda-artifacts-init
task lambda-dev-init

function_name=$(tofu -chdir=infra/lambda/environments/dev output -raw lambda_function_name)
api_url=$(tofu -chdir=infra/lambda/environments/dev output -raw api_default_url)
repository_url=$(tofu -chdir=infra/lambda/artifacts output -raw ecr_repository_url)
live_version=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" lambda get-alias \
  --function-name "$function_name" --name live --query FunctionVersion --output text)
image_uri=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" lambda get-function \
  --function-name "$function_name" --qualifier "$live_version" --query Code.ImageUri --output text)
release_digest=${image_uri##*@}
release_tag=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ecr describe-images \
  --repository-name portfolio-lambda-releases --image-ids imageDigest="$release_digest" \
  --query 'imageDetails[0].imageTags[?starts_with(@, `git-`)] | [0]' --output text)
release_sha=${release_tag#git-}
health_revision=$(curl --fail --show-error --silent \
  https://dev.craigdevjohnson.com/healthz | jq -r .revision)
test "$health_revision" = "$release_sha"
printf 'function=%s api=%s repository=%s version=%s image=%s source=%s\n' \
  "$function_name" "$api_url" "$repository_url" "$live_version" "$image_uri" "$release_sha"
```

Create a clean evidence branch from the merged main and use `apply_patch` to
write `docs/deployment/evidence/releases/${release_sha}.json`. The JSON schema is:

```bash
git fetch origin
git switch -c codex/development-observation-evidence origin/main
test -z "$(git status --porcelain)"
mkdir -p docs/deployment/evidence/releases
release_sha=$(curl --fail --show-error --silent \
  https://dev.craigdevjohnson.com/healthz | jq -er .revision)
test "$(printf '%s' "$release_sha" | grep -Ec '^[0-9a-f]{40}$')" = "1"
release_record="docs/deployment/evidence/releases/${release_sha}.json"
printf '%s\n' "$release_record"
```

- `schema_version: 1` and the 40-character `source_sha`;
- `image`: repository name/URL, tag formed from `git-` plus `source_sha`, digest-qualified URI,
  push time, and `COMPLETE` scan status;
- `development`: function name, published version, identical `live` alias
  target, default endpoint, custom-domain array, health revision, DNS-cutover
  timestamp, `rollback_evidence` pointing to the committed development
  precutover JSON, `observation_evidence` equal to
  `docs/deployment/evidence/development-observation.jsonl`, and a null
  observation-completion timestamp; and
- `production: null`.

Do not include secrets, cookies, raw state, OAuth client values, Cloudflare API
tokens, or subscriber endpoints. Validate it mechanically:

```bash
release_sha=$(curl --fail --show-error --silent \
  https://dev.craigdevjohnson.com/healthz | jq -er .revision)
release_record="docs/deployment/evidence/releases/${release_sha}.json"
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task lambda-artifacts-init
task lambda-dev-init
function_name=$(tofu -chdir=infra/lambda/environments/dev output -raw lambda_function_name)
live_version=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" lambda get-alias \
  --function-name "$function_name" --name live --query FunctionVersion --output text)
live_image_uri=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" lambda get-function \
  --function-name "$function_name" --qualifier "$live_version" --query Code.ImageUri --output text)
release_digest=${live_image_uri##*@}
test "$(printf '%s' "$release_digest" | grep -Ec '^sha256:[0-9a-f]{64}$')" = "1"
jq -e --arg sha "$release_sha" --arg digest "$release_digest" --arg image "$live_image_uri" \
  '.schema_version == 1 and
   .source_sha == $sha and
   .image.repository_name == "portfolio-lambda-releases" and
   .image.tag == ("git-" + $sha) and
   .image.digest == $digest and
   .image.uri == $image and
   .image.uri == (.image.repository_url + "@" + .image.digest) and
   .image.scan_status == "COMPLETE" and
   .development.healthz_revision == $sha and
   .development.published_version == .development.live_alias_target and
   (.development.api_endpoint | length > 0) and
   (.development.custom_domains | index("dev.craigdevjohnson.com") != null) and
   .development.rollback_evidence == "docs/deployment/evidence/development-precutover.json" and
   .development.observation_evidence == "docs/deployment/evidence/development-observation.jsonl" and
   .development.observation_completed_at == null and
   .production == null' "$release_record"

: "${CONNECT_REQUEST_ID:?set the sanitized connect request ID from Step 10}"
: "${ADD_REQUEST_ID:?set the sanitized add request ID from Step 10}"
: "${SYNC_REQUEST_ID:?set the sanitized sync request ID from Step 10}"
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
evidence_file=docs/deployment/evidence/development-observation.jsonl
task lambda-dev-observation-workflow \
  RELEASE_RECORD="$release_record" \
  EVIDENCE_FILE="$evidence_file" \
  PUBLIC_HOST=dev.craigdevjohnson.com \
  CONNECT_REQUEST_ID="$CONNECT_REQUEST_ID" \
  ADD_REQUEST_ID="$ADD_REQUEST_ID" \
  SYNC_REQUEST_ID="$SYNC_REQUEST_ID" \
  OAUTH_OK=true SECURE_COOKIES_OK=true ADD_OK=true SYNC_OK=true

git add -- "$release_record" "$evidence_file"
git commit -m "docs: record development Lambda release"
```

The workflow command records request IDs only, never cookies, calendar names,
event bodies, JWTs, OAuth codes, or tokens.

Keep the evidence branch local until the observation gate passes; its JSONL file
is the durable state for the recurring monitor.

---

### Task 13: Observe development for seven days and close the rollback window

**Files:**

- Create: `docs/deployment/evidence/development-observation.jsonl`
- Modify: the source-SHA-named JSON release record created in Task 12
- No runtime, DNS, data, or legacy-platform deletion

**Interfaces:**

- Consumes: the complete development release record and committed rollback evidence
- Produces: a seven-full-day go/no-go record required before production starts

- [ ] **Step 1: Start recurring development checks**

Set explicit paths from the reviewed source SHA and run the authoritative sample:

```bash
: "${RELEASE_RECORD:?set the reviewed release JSON path}"
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
EVIDENCE_FILE=docs/deployment/evidence/development-observation.jsonl
task lambda-dev-observation-sample \
  RELEASE_RECORD="$RELEASE_RECORD" \
  EVIDENCE_FILE="$EVIDENCE_FILE"
```

Use the product's recurring monitoring mechanism to run the same sample at
least daily. Record one manual full OAuth, Secure-cookie, add, and result-sync
proof at cutover and again after seven full days in the schema consumed by the
gate.

- [ ] **Step 2: Apply explicit release criteria**

The sample or gate fails and requires a rollback proposal when any of these occurs:

- TLS, cookie security, health revision, digest, or alias proof is wrong once;
- a primary route or required asset fails two probes five minutes apart;
- Google OAuth, calendar add, or result sync fails twice after one safe retry;
- Lambda error, throttle, API 5xx, duration, or latency alarm remains `ALARM`
  for ten minutes; or
- Lambda duration p95 exceeds 24 seconds or API latency p95 exceeds 25 seconds
  in two consecutive five-minute periods with at least 20 requests total.

The final gate also requires at least eight passing samples, no gap over 26
hours, stable SHA/digest/version/alias coordinates, a healthy App Runner origin,
and at least 604800 seconds since the recorded DNS cutover. Record low-volume
cases as such instead of treating missing metrics as success.

- [ ] **Step 3: Prepare and, when required, approve rollback**

Read `.development.rollback_evidence` from the release JSON, require that file,
and mechanically validate its hostname, App Runner origin, and complete DNS
record fields before comparing them to live Cloudflare state. Build the exact
affected-URL purge list. Keep the Lambda environment, tables, image, and logs
intact. **Approval gate:** present the exact DNS restoration and cache URLs and
obtain current-session approval before changing traffic. After an approved
rollback, verify the recorded App Runner origin publicly and record the incident.

- [ ] **Step 4: Record the day-seven full workflow proof**

After at least 604800 seconds, complete OAuth/connect, Secure-cookie inspection,
one approved calendar add, and one approved result sync in a clean browser
session. Confirm the corresponding sanitized request IDs in Lambda/API logs and
append the second workflow record:

```bash
: "${RELEASE_RECORD:?set the reviewed release JSON path}"
: "${CONNECT_REQUEST_ID:?set the sanitized day-seven connect request ID}"
: "${ADD_REQUEST_ID:?set the sanitized day-seven add request ID}"
: "${SYNC_REQUEST_ID:?set the sanitized day-seven sync request ID}"
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
EVIDENCE_FILE=docs/deployment/evidence/development-observation.jsonl
task lambda-dev-observation-workflow \
  RELEASE_RECORD="$RELEASE_RECORD" \
  EVIDENCE_FILE="$EVIDENCE_FILE" \
  PUBLIC_HOST=dev.craigdevjohnson.com \
  CONNECT_REQUEST_ID="$CONNECT_REQUEST_ID" \
  ADD_REQUEST_ID="$ADD_REQUEST_ID" \
  SYNC_REQUEST_ID="$SYNC_REQUEST_ID" \
  OAUTH_OK=true SECURE_COOKIES_OK=true ADD_OK=true SYNC_OK=true
```

- [ ] **Step 5: Close the development window only with the executable gate**

```bash
: "${RELEASE_RECORD:?set the reviewed release JSON path}"
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
EVIDENCE_FILE=docs/deployment/evidence/development-observation.jsonl
task lambda-dev-observation-gate \
  RELEASE_RECORD="$RELEASE_RECORD" \
  EVIDENCE_FILE="$EVIDENCE_FILE"
```

Only after exit zero, use `apply_patch` to set the release record's
`development.observation_completed_at` to the final sample's RFC3339 timestamp.
Re-run the JSON consistency checks and the gate, then commit the JSONL and JSON
as `docs: record development Lambda observation`.

**Approval gate:** Present the sanitized evidence diff, exact branch and commit,
PR title/body, and merge method. Push, open the evidence PR, and merge it only
after separate current-session approvals and successful hosted checks. The
production plan must not start until both files are merged on `origin/main` and
App Runner remains unchanged.

For the merge approval, record `evidence_head_sha=$(git rev-parse HEAD)`, require
the PR `headRefOid` to equal it before and after marking ready, and use
`gh pr merge "$evidence_pr_url" --merge --match-head-commit "$evidence_head_sha"`.
