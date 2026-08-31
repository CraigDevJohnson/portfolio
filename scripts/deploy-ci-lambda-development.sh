#!/bin/sh
set -eu

: "${SOURCE_SHA:?set SOURCE_SHA}"
: "${IMAGE_DIGEST:?set IMAGE_DIGEST}"
: "${GITHUB_WORKSPACE:?set GITHUB_WORKSPACE}"
: "${GITHUB_REPOSITORY:?set GITHUB_REPOSITORY}"
: "${ECR_URL:?set ECR_URL}"
: "${STATE_BUCKET:?set STATE_BUCKET}"

evidence_dir="$GITHUB_WORKSPACE/evidence"
mkdir -p "$evidence_dir"
deployment_evidence="$evidence_dir/github-deployment.json"
deployment_response="$evidence_dir/github-deployment-response.json"
finalize_deployment() {
  status=$?
  trap - EXIT
  trap '' HUP INT TERM
  if { [ -f "$deployment_evidence" ] || [ -f "$deployment_response" ]; } && ! jq -e '
    .status == "success" and .status_recorded == true
  ' "$deployment_evidence" > /dev/null 2>&1; then
    if ! DEPLOYMENT_STATE=failure \
      EVIDENCE_DIR="$evidence_dir" \
      sh scripts/record-ci-lambda-development.sh; then
      echo 'Could not record the failed development deployment status' >&2
    fi
  fi
  exit "$status"
}
trap finalize_deployment EXIT
trap 'exit 1' HUP INT TERM

sh scripts/check-ci-state-bucket.sh
aws lambda get-alias \
  --function-name portfolio-lambda-dev \
  --name live \
  --output json > "$evidence_dir/alias-before.json"
jq -e '
  .FunctionVersion | type == "string" and test("^[1-9][0-9]*$")
' "$evidence_dir/alias-before.json" > /dev/null
alias_before_version=$(jq -r '.FunctionVersion' "$evidence_dir/alias-before.json")

RELEASE_ENVIRONMENT=development \
  IMAGE_DIGEST="$IMAGE_DIGEST" \
  EVIDENCE_DIR="$evidence_dir" \
  sh scripts/create-ci-lambda-release-plan.sh

DEPLOYMENT_STATE=in_progress \
  ROLLBACK_VERSION="$alias_before_version" \
  EVIDENCE_DIR="$evidence_dir" \
  sh scripts/record-ci-lambda-development.sh
development_base=$(sh scripts/resolve-development-release-base.sh \
  "$SOURCE_SHA" coordinate)
development_base_version=$(printf '%s\n' "$development_base" | cut -f2)
rollback_version=${development_base_version:-$alias_before_version}
create_rollback_candidate() {
  printf '%s\n' "$rollback_version" | grep -Eq '^[1-9][0-9]*$' || {
    echo 'Cannot create rollback plan without the validated prior alias version' >&2
    return 1
  }
  PRIOR_VERSION="$rollback_version" \
    IMAGE_DIGEST="$IMAGE_DIGEST" \
    EVIDENCE_DIR="$evidence_dir" \
    sh scripts/create-ci-lambda-rollback-plan.sh
}
fail_with_rollback_candidate() {
  trap '' HUP INT TERM
  if ! create_rollback_candidate; then
    echo 'Could not retain an approved rollback candidate' >&2
  fi
  exit 1
}
trap fail_with_rollback_candidate HUP INT TERM

sh scripts/check-current-main.sh "$SOURCE_SHA"
if ! tofu -chdir=infra/lambda/environments/dev apply \
  -lock-timeout=5m \
  -input=false \
  "$evidence_dir/dev.tfplan"; then
  fail_with_rollback_candidate
fi
if ! tofu -chdir=infra/lambda/environments/dev output -json \
  > "$evidence_dir/outputs.json"; then
  fail_with_rollback_candidate
fi
if ! BASE_URL=https://dev.craigdevjohnson.com \
  SOURCE_SHA="$SOURCE_SHA" \
  IMAGE_DIGEST="$IMAGE_DIGEST" \
  FUNCTION_NAME=portfolio-lambda-dev \
  EVIDENCE_DIR="$evidence_dir" \
  sh scripts/verify-lambda-release.sh; then
  fail_with_rollback_candidate
fi

DEPLOYMENT_STATE=success \
  LAMBDA_VERSION="$(jq -er '
    .lambda_version | select(type == "string" and test("^[1-9][0-9]*$"))
  ' "$evidence_dir/verification.json")" \
  EVIDENCE_DIR="$evidence_dir" \
  sh scripts/record-ci-lambda-development.sh
