# Development App Runner Retirement Design

**Approved:** 2026-08-29

**Scope:** Retire the development App Runner service and its dedicated IAM
resources after the Lambda/API Gateway cutover, without completing the former
seven-day development observation window.

## Decision

The repository owner accepts the loss of App Runner as a development rollback
origin. The development observation gate remains truthfully incomplete, with
`development.observation_completed_at` left `null`. Retirement uses one fresh
pre-delete smoke check and one fresh post-delete smoke check instead of a
multi-day window.

This decision supersedes only the development App Runner retention and
observation prerequisites in the 2026-08-21 Lambda platform migration design
and implementation plans. It does not waive the production observation gate,
authorize Amplify retirement, or approve any live mutation by itself.

The historical failed development sample remains unchanged. It records a
healthy replacement Lambda, routes, assets, alarms, and metrics, with the sole
blocker that the legacy App Runner release returned `404` for the replacement
stylesheet path. That rollback-only mismatch is accepted; the obsolete probe
will not be repaired or represented as passing.

Root inventory later on 2026-08-29 corrected one design assumption: neither the
legacy state nor the live account contains the formerly declared
`portfolio-lambda` function, `portfolio-lambda-http` API, execution role, or
runtime policy. Their unused declarations, outputs, and targeted deployment
helpers must be removed before planning so a full refresh cannot propose
creating them.

## Retirement boundary

The legacy shared OpenTofu root may delete exactly these managed resources:

1. `aws_apprunner_service.app`
2. `aws_iam_role.apprunner_ecr_access`
3. `aws_iam_role_policy_attachment.apprunner_ecr_access`
4. `aws_iam_role.apprunner_instance`
5. `aws_iam_role_policy_attachment.google_connections_dynamodb`
6. `aws_iam_role_policy_attachment.soccer_sessions_dynamodb`
7. `aws_iam_policy.apprunner_runtime_secrets`
8. `aws_iam_role_policy_attachment.apprunner_runtime_secrets`

The plan may also delete the four App Runner-only root outputs:

- `app_runner_service_url`
- `app_runner_service_arn`
- `app_runner_service_id`
- `instance_role_arn`

No create, update, replacement, or other delete action is allowed. Data-source
reads and no-op actions do not mutate infrastructure and may remain in the plan.

## Resources retained

This retirement retains:

- the legacy `portfolio` ECR repository and lifecycle policy;
- the immutable `portfolio-lambda-releases` ECR repository;
- the legacy Google connection and Soccer session DynamoDB tables;
- the shared DynamoDB IAM policies;
- all `/portfolio/*` and `/portfolio/lambda/*` SSM parameters;
- every replacement root under `infra/lambda/`;
- Cloudflare records and rules;
- Google OAuth configuration; and
- App Runner CloudWatch log groups pending a separate retention or deletion
  decision.

The out-of-band App Runner custom-domain association is not managed by this
OpenTofu state. It must be inventoried, separately approved, disassociated, and
verified absent before the saved retirement plan is applied.
The App Runner instance role's untracked `portoflio-ssm-params` inline policy is
also an out-of-band prerequisite: its document is a strict subset of the
attached managed runtime-secrets policy, but its exact deletion still requires
separate approval and verification before the preflight can pass.

## Execution contract

Retirement plan creation and apply must:

- use `AWS_PROFILE=portfolio-deployer` and `AWS_REGION=us-west-2`;
- reject root, ambient static credentials, and the wrong account or SSO role;
- require explicit acknowledgement of
  `s3://portfolio-tofu-state-180294223248/portfolio/terraform.tfstate.tflock`;
- initialize the legacy root using its checked-in backend block with
  `tofu -chdir=infra init -reconfigure -lockfile=readonly -input=false`, its
  reviewed AWS provider 5.100.0 lock, and never a nonexistent `backend.hcl`
  override;
- reject `TF_CLI_ARGS`, `TF_CLI_ARGS_init`, `TF_CLI_ARGS_workspace`,
  `TF_CLI_ARGS_plan`, `TF_CLI_ARGS_show`, `TF_CLI_ARGS_apply`, `TF_WORKSPACE`,
  and `TF_DATA_DIR`, and prove the workspace is `default` at initialization,
  plan creation, and apply time;
- use a new absolute saved-plan path in a current-user-owned mode-700
  directory, rejecting symlinks and multi-link plan files;
- validate the saved-plan JSON against the exact action allowlist above;
- create plans with the normal full refresh and lock acquisition, without
  `-target`, `-destroy`, or automatic approval;
- reject `-target`, `-destroy`, and automatic approval;
- before apply, verify the supplied SHA-256 checksum, inspect and checker-verify
  fresh JSON from that exact saved plan, verify the checksum again, and apply
  only the unchanged reviewed file; and
- keep Git publication, App Runner domain disassociation, plan lock writes,
  plan apply, and later cleanup as separate approval boundaries.

The legacy `task deploy`, App Runner `task redeploy`, and App Runner `task logs`
interfaces are retired so they cannot recreate or operate the deleted service.
The unused legacy Lambda-specific deployment helpers are also removed because
there is no corresponding state or live runtime and they would create new,
unreviewed infrastructure.

## Point-in-time verification

Immediately before live deletion, capture an authenticated read-only
Cloudflare export proving `dev.craigdevjohnson.com` is proxied to the API Gateway
regional hostname and that no origin rule or Worker references App Runner.
Also verify the Lambda function state, `live` alias target, image digest, API
mapping, five alarm states, public `/healthz`, `/`, `/soccer`, Tailwind asset,
and OAuth initiation.

After deletion completes, repeat the replacement checks and confirm the App
Runner service and dedicated IAM resources are absent. The existing plain-HTTP
`521` response is a separate Cloudflare redirect defect and is not part of this
retirement.
