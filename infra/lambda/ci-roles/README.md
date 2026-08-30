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

This root uses the isolated state configuration in `backend.hcl`. Initialize it
only through a separately reviewed administrator session. The
`portfolio-deployer` profile is deliberately insufficient: its bootstrap policy
does not grant access to this state or authority over CI roles. Use a distinct
administrator identity that has been explicitly reviewed to access only:

- the
  `portfolio-lambda-http-api/ci-roles/terraform.tfstate{,.tflock}` objects in
  `portfolio-tofu-state-180294223248`; and
- the `portfolio-release-builder-ci`, `portfolio-development-deployer-ci`, and
  `portfolio-production-planner-ci` IAM roles and their inline policies,
  including read access to the existing GitHub OIDC provider.

The acknowledgement prevents accidentally using the normal deployment
identity; it does not grant any AWS permissions:

```bash
export AWS_PROFILE=portfolio-ci-roles-administrator
export AWS_REGION=us-west-2
APPROVED_CI_ROLES_ADMIN=portfolio-lambda-http-api/ci-roles \
  task lambda-ci-roles-init
```

No release workflow may provision or modify these roles. The root must not be
added to any automated deployer's permissions.
