# Portfolio account root

This root owns the portfolio's account-level resources in the workloads
account. The directory keeps its historical `ci-roles` name and state key.

| File | Resources |
| --- | --- |
| `main.tf` | The four GitHub OIDC CI roles and their inline policies |
| `boundary.tf` | `PortfolioLambdaExecutionBoundary` at `/portfolio/boundaries/` |
| `state_bucket.tf` | The `portfolio-tofu-state-<account>` state bucket and its settings |

Apply this root before any other portfolio root: the dev and prod execution
roles attach the boundary by ARN, and every root stores its state in the bucket.

## Identity

The provider and backend are pinned to the `workloads-admin` profile, and
`allowed_account_ids` is `[var.aws_account_id]`. Release workflows never plan or
apply this root. Only Craig's WorkloadsAdmin session changes it, through a saved
plan he has reviewed.

## CI roles

| Role | GitHub variable | Trusted `sub` |
| --- | --- | --- |
| `portfolio-release-builder-ci` | `AWS_RELEASE_BUILDER_ROLE_ARN` (repository) | `repo:CraigDevJohnson/portfolio:ref:refs/heads/main` |
| `portfolio-development-deployer-ci` | `AWS_DEVELOPMENT_DEPLOYER_ROLE_ARN` (`development`) | `repo:CraigDevJohnson/portfolio:environment:development` |
| `portfolio-production-planner-ci` | `AWS_PRODUCTION_PLANNER_ROLE_ARN` (`production-plan`) | `repo:CraigDevJohnson/portfolio:environment:production-plan` |
| `portfolio-production-deployer-ci` | `AWS_PRODUCTION_DEPLOYER_ROLE_ARN` (`production`) | `repo:CraigDevJohnson/portfolio:environment:production` |

Every role trusts the account's GitHub OIDC provider, which aws-setup owns. This
root looks it up by URL with a data source, requires `aud` to be
`sts.amazonaws.com`, and pins `sub` exactly. The repository uses GitHub's
default subject format.

- The release builder can push to `portfolio-lambda-releases` and nothing else.
- The development deployer can read the dev stack, write only the dev state
  object, and publish a new image version and move the `live` alias of
  `portfolio-lambda-dev`.
- The production planner can read the prod stack and write only the prod state
  lock object.
- The production deployer adds prod state writes and the same image, version and
  alias writes for `portfolio-lambda-prod`.

No CI role can create or reconfigure IAM, API Gateway, DynamoDB, ACM, logs or
alarms. Those changes are applied by Craig with `workloads-admin`.

After an apply, `tofu output role_arns` prints the four ARNs for the GitHub
variables.

## Execution boundary

The boundary is generated from the account ID. Each statement allows only its
own environment's execution role (`aws:PrincipalArn`):

- the environment's two DynamoDB tables;
- `ssm:GetParameters` and SSM-mediated `kms:Decrypt` for the environment's
  three SecureStrings under `/portfolio/lambda/<env>/`: `CLIENT_ID_KEY`,
  `CLIENT_SECRET_KEY` and `LPS_SESSION_KEY`. The retired `MGMT_SESSION_KEY`
  is not readable; `SITE_SESSION_KEY` is added when that environment's site
  sign-in is activated;
- the environment's Lambda log group;
- for dev only, the portal's read-only `ec2:DescribeInstances` and
  `cloudwatch:GetMetricStatistics` in us-west-2.

Per D22 the boundary grants no EC2 start/stop and no `/ec2/i-*` log reads.

## State bucket

The bucket is created once with the AWS CLI, because this root keeps its own
state in it. The first plan then adopts it through the `import` blocks in
`state_bucket.tf`: versioning, full public access block, SSE-S3 and
`BucketOwnerEnforced`. `prevent_destroy` protects the bucket, its versioning
and its public access block. Once the bucket is in
state, the import blocks are no-ops. The tests set `import_state_bucket = false`
because mock providers cannot import.

## Commands

```sh
task lambda-ci-roles-init
task lambda-ci-roles-plan PLAN_FILE=/absolute/path/ci-roles.tfplan
task lambda-ci-roles-apply PLAN_FILE=/absolute/path/ci-roles.tfplan
```

Offline checks run in `task lambda-infrastructure-ci`.
