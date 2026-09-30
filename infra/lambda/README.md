# Lambda HTTP API infrastructure

The OpenTofu roots for the portfolio in the workloads account. Each root has
its own state key in `portfolio-tofu-state-<account>` and an `aws_account_id`
input that its provider enforces through `allowed_account_ids`.

- `ci-roles` is the account root: the GitHub OIDC CI roles, the
  `PortfolioLambdaExecutionBoundary` permissions boundary and the state bucket.
  Apply it first. See its [README](ci-roles/README.md).
- `artifacts` owns only the immutable release repository.
- `environments/dev` and `environments/prod` call `modules/service` with
  distinct backend keys and settings.
- `modules/service` contains no backend or provider configuration.
- `auth/dev` is the planned development Cognito pool; it is not provisioned.

Saved plans can contain sensitive configuration, so keep them outside the
checkout. Plan and apply are separate commands; see
[DEPLOY-INSTRUCTIONS.md](../../DEPLOY-INSTRUCTIONS.md).
