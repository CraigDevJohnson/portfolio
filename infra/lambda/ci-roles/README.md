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

This root uses the isolated state configuration in `backend.hcl`. Its plan task
creates a private OpenTofu data directory, reinitializes the exact backend,
requires the `default` workspace, and emits both a saved plan and a
checksum-bound `.provenance.json` sidecar. Review the rendered plan, the
sidecar, and both checksums, then apply that exact pair only through a separately
reviewed administrator session. The sidecar is an attestation from this
reviewed creation task; OpenTofu's public saved-plan JSON does not expose the
embedded backend, so a hand-built plan and sidecar are not valid substitutes.
The
`portfolio-deployer` profile is deliberately insufficient: its bootstrap policy
does not grant access to this state or authority over CI roles. Use a distinct
administrator identity that has been explicitly reviewed to access only:

- `s3:ListBucket` on `arn:aws:s3:::portfolio-tofu-state-180294223248`, with a
  `StringEquals` condition limiting `s3:prefix` to the exact
  `portfolio-lambda-http-api/ci-roles/terraform.tfstate` and
  `portfolio-lambda-http-api/ci-roles/terraform.tfstate.tflock` keys;
- `s3:GetBucketLocation` and `s3:GetBucketVersioning` on that bucket for the
  repository's region and versioning preflight, plus `s3:GetObject`,
  `s3:PutObject`, and `s3:DeleteObject` on only those two state objects; and
- the `portfolio-release-builder-ci`, `portfolio-development-deployer-ci`, and
  `portfolio-production-planner-ci` IAM roles and their inline policies,
  including read access to the existing GitHub OIDC provider.

The reviewed IAM Identity Center permission-set name is
`PortfolioCIRolesAdministrator`. The AWS CLI profile must be exactly
`portfolio-ci-roles-administrator`, and its effective STS ARN must match
`arn:aws:sts::180294223248:assumed-role/AWSReservedSSO_PortfolioCIRolesAdministrator_<suffix>/<session>`.
Every CI-role Task also requires `AWS_REGION=us-west-2`, rejects root and every
other account or role, and refuses ambient `AWS_ACCESS_KEY_ID`,
`AWS_SECRET_ACCESS_KEY`, or `AWS_SESSION_TOKEN` values so OpenTofu uses the
same reviewed SSO session that the guard checks.

Each Task invocation requires the exact root acknowledgement below. It prevents
accidentally using the normal deployment identity; it does not grant any AWS
permissions:

```bash
export AWS_PROFILE=portfolio-ci-roles-administrator
export AWS_REGION=us-west-2
ci_roles_plan_dir=$(mktemp -d)
ci_roles_plan="$ci_roles_plan_dir/ci-roles.tfplan"
APPROVED_CI_ROLES_ADMIN=portfolio-lambda-http-api/ci-roles \
  task lambda-ci-roles-plan \
    PLAN_FILE="$ci_roles_plan" \
    APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/ci-roles/terraform.tfstate.tflock
ci_roles_plan_sha256=$(shasum -a 256 "$ci_roles_plan" | awk '{print $1}')
ci_roles_provenance="$ci_roles_plan.provenance.json"
ci_roles_provenance_sha256=$(shasum -a 256 "$ci_roles_provenance" | awk '{print $1}')
printf 'ci_roles_plan_sha256=%s\n' "$ci_roles_plan_sha256"
printf 'ci_roles_provenance_sha256=%s\n' "$ci_roles_provenance_sha256"
jq . "$ci_roles_provenance"
```

After separate plan review, obtain a fresh current-session apply and lock-write
approval. Copy both reviewed checksums exactly, then run:

```bash
: "${APPROVED_PLAN_SHA256:?set the exact reviewed plan SHA-256 checksum}"
: "${APPROVED_PROVENANCE_SHA256:?set the exact reviewed provenance SHA-256 checksum}"
APPROVED_CI_ROLES_ADMIN=portfolio-lambda-http-api/ci-roles \
  task lambda-ci-roles-apply \
    PLAN_FILE="$ci_roles_plan" \
    PROVENANCE_FILE="$ci_roles_provenance" \
    APPROVED_PLAN_SHA256="$APPROVED_PLAN_SHA256" \
    APPROVED_PROVENANCE_SHA256="$APPROVED_PROVENANCE_SHA256" \
    APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/ci-roles/terraform.tfstate.tflock
```

The sidecar contains only the plan checksum, `default` workspace, and the
reviewed S3 backend type, bucket, key, region, encryption, and native-locking
fields. Plan and apply reject ambient OpenTofu workspace, data-directory,
provider-reattachment, CLI-configuration, CLI-argument, credential-redirection,
role-assumption, and AWS endpoint overrides. Each command uses a private empty
OpenTofu CLI configuration. Apply snapshots both approved files into a private
directory, verifies the copied bytes and provenance, renders the copied plan to
JSON, reruns the complete CI-role policy contract, and only then applies that
private plan copy.

After the apply, use the authoritative read-only verification task to require
each deterministic ARN before using it in GitHub configuration:

```bash
APPROVED_CI_ROLES_ADMIN=portfolio-lambda-http-api/ci-roles \
  task lambda-ci-roles-verify
```

No release workflow may provision or modify these roles. The root must not be
added to any automated deployer's permissions.
