#!/bin/sh
# Apply the production plan saved earlier in the same Release run, after Craig
# approved the production environment, then verify the release.
set -eu

: "${SOURCE_SHA:?set SOURCE_SHA}"
: "${IMAGE_DIGEST:?set IMAGE_DIGEST}"
: "${GITHUB_WORKSPACE:?set GITHUB_WORKSPACE}"
: "${PLANNED_PLAN_SHA256:?set PLANNED_PLAN_SHA256}"
evidence_dir="$GITHUB_WORKSPACE/evidence"
root=infra/lambda/environments/prod
plan_file="$evidence_dir/prod.tfplan"

actual_sha256=$(shasum -a 256 "$plan_file" | awk '{print $1}')
[ "$actual_sha256" = "$PLANNED_PLAN_SHA256" ] || {
  echo 'Saved production plan does not match the plan shown for approval' >&2
  exit 1
}

sh scripts/check-current-main.sh "$SOURCE_SHA"
tofu -chdir="$root" init -backend-config=backend.hcl -reconfigure -input=false
workspace=$(tofu -chdir="$root" workspace show)
[ "$workspace" = default ] || {
  printf 'Refusing non-default OpenTofu workspace: %s\n' "$workspace" >&2
  exit 1
}
tofu -chdir="$root" apply -lock-timeout=5m -input=false "$plan_file"

ENVIRONMENT=prod EVIDENCE_DIR="$evidence_dir" sh scripts/verify-lambda-release.sh
