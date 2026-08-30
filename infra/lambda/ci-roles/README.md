# GitHub Actions deployment roles

This isolated root defines three GitHub OIDC roles. It is **not** called by a
release workflow. Provision it through a separately reviewed administrator plan,
then configure its outputs as repository/environment variables:

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
export APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/ci-roles/terraform.tfstate.tflock
ci_roles_plan="$(pwd)/ci-roles.tfplan"
task lambda-ci-roles-init
task lambda-ci-roles-plan PLAN_FILE="$ci_roles_plan"
shasum -a 256 "$ci_roles_plan"
# After separate review, copy the printed checksum exactly.
task lambda-ci-roles-apply PLAN_FILE="$ci_roles_plan" APPROVED_PLAN_SHA256=<reviewed-sha256>
```

No release workflow may provision or modify these roles. The root must not be
added to any automated deployer's permissions.
