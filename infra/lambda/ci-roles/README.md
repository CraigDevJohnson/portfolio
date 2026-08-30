# GitHub Actions deployment roles

This isolated root defines three GitHub OIDC roles. It is **not** called by a
release workflow. Provision it through a separately reviewed administrator plan,
then read back the exact role ARNs and configure them as
repository/environment variables:

- `AWS_RELEASE_BUILDER_ROLE_ARN` (repository variable);
- `AWS_DEVELOPMENT_DEPLOYER_ROLE_ARN` (`development` environment); and
- `AWS_PRODUCTION_PLANNER_ROLE_ARN` (`production-plan` environment).

The release trust is restricted to `main`; environment trust is restricted to
the exact GitHub Environment. There are no IAM-user keys. The production role
has read-only infrastructure permissions plus access to its state lock object;
it cannot update production services or state. The `production-plan`
environment must have required reviewers.

The development role is post-bootstrap and runtime-only. It can refresh the
existing stack, write only the development state and lock objects, and update
only the digest-qualified image, published version, and `live` alias of
`portfolio-lambda-dev`. It cannot create or reconfigure IAM, API Gateway,
DynamoDB, ACM, log, or alarm resources. Provision missing infrastructure through
a separately reviewed `portfolio-deployer` SSO plan. If replacement state is
lost, restore a reviewed version from the versioned state bucket; do not grant
the workflow bootstrap or import permissions.

This root uses the isolated state configuration in `backend.hcl`. Initialize it,
create a saved plan, review its rendered output and checksum, and apply that
exact plan only through a separately reviewed administrator session:

```bash
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task lambda-ci-roles-init
ci_roles_plan_dir=$(mktemp -d)
ci_roles_plan="$ci_roles_plan_dir/ci-roles.tfplan"
task lambda-ci-roles-plan \
  PLAN_FILE="$ci_roles_plan" \
  APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/ci-roles/terraform.tfstate.tflock
ci_roles_plan_sha256=$(shasum -a 256 "$ci_roles_plan" | awk '{print $1}')
printf 'ci_roles_plan_sha256=%s\n' "$ci_roles_plan_sha256"
```

After separate plan review, obtain a fresh current-session apply and lock-write
approval. Copy the reviewed checksum exactly, then run:

```bash
: "${APPROVED_PLAN_SHA256:?set the exact reviewed plan SHA-256 checksum}"
task lambda-ci-roles-apply \
  PLAN_FILE="$ci_roles_plan" \
  APPROVED_PLAN_SHA256="$APPROVED_PLAN_SHA256" \
  APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/ci-roles/terraform.tfstate.tflock
```

After the apply, read each role from IAM and require its deterministic ARN before
using it in GitHub configuration:

```bash
release_role_arn=$(
  aws --profile "$AWS_PROFILE" --region "$AWS_REGION" iam get-role \
    --role-name portfolio-release-builder-ci --query 'Role.Arn' --output text
)
development_role_arn=$(
  aws --profile "$AWS_PROFILE" --region "$AWS_REGION" iam get-role \
    --role-name portfolio-development-deployer-ci --query 'Role.Arn' --output text
)
production_role_arn=$(
  aws --profile "$AWS_PROFILE" --region "$AWS_REGION" iam get-role \
    --role-name portfolio-production-planner-ci --query 'Role.Arn' --output text
)
test "$release_role_arn" = "arn:aws:iam::180294223248:role/portfolio-release-builder-ci"
test "$development_role_arn" = "arn:aws:iam::180294223248:role/portfolio-development-deployer-ci"
test "$production_role_arn" = "arn:aws:iam::180294223248:role/portfolio-production-planner-ci"
printf '%s\n%s\n%s\n' "$release_role_arn" "$development_role_arn" "$production_role_arn"
```

No release workflow may provision or modify these roles. The root must not be
added to any automated deployer's permissions.
