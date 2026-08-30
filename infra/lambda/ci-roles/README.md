# GitHub Actions deployment roles

This isolated root defines three GitHub OIDC roles. It is **not** called by a
release workflow. Provision it through a separately reviewed administrator plan,
then configure its outputs as repository/environment variables:

- `AWS_RELEASE_BUILDER_ROLE_ARN` (repository variable);
- `AWS_DEVELOPMENT_DEPLOYER_ROLE_ARN` (`development` environment); and
- `AWS_PRODUCTION_DEPLOYER_ROLE_ARN` (`production` environment).

The release trust is restricted to `main`; deployer trust is restricted to its
exact GitHub Environment. There are no IAM-user keys. Production must have
required reviewers, and remains plan-only until the public cutover checklist is
closed. This root must use its own remote state before provisioning; it must not
be added to any automated deployer's permissions.
