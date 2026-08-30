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
sh scripts/check-ci-state-bucket.sh
aws lambda get-alias \
  --function-name portfolio-lambda-dev \
  --name live \
  --output json > "$evidence_dir/alias-before.json" 2> /dev/null ||
  printf '{}\n' > "$evidence_dir/alias-before.json"

RELEASE_ENVIRONMENT=development \
  IMAGE_DIGEST="$IMAGE_DIGEST" \
  EVIDENCE_DIR="$evidence_dir" \
  sh scripts/create-ci-lambda-release-plan.sh

sh scripts/check-current-main.sh "$SOURCE_SHA"
tofu -chdir=infra/lambda/environments/dev apply \
  -lock-timeout=5m \
  -input=false \
  "$evidence_dir/dev.tfplan"
tofu -chdir=infra/lambda/environments/dev output -json > "$evidence_dir/outputs.json"

if ! BASE_URL=https://dev.craigdevjohnson.com \
  SOURCE_SHA="$SOURCE_SHA" \
  IMAGE_DIGEST="$IMAGE_DIGEST" \
  FUNCTION_NAME=portfolio-lambda-dev \
  EVIDENCE_DIR="$evidence_dir" \
  sh scripts/verify-lambda-release.sh; then
  prior=$(jq -r '.FunctionVersion // empty' "$evidence_dir/alias-before.json")
  if printf '%s\n' "$prior" | grep -Eq '^[0-9]+$'; then
    PRIOR_VERSION="$prior" \
      IMAGE_DIGEST="$IMAGE_DIGEST" \
      EVIDENCE_DIR="$evidence_dir" \
      sh scripts/create-ci-lambda-rollback-plan.sh
  fi
  exit 1
fi

EVIDENCE_DIR="$evidence_dir" sh scripts/record-ci-lambda-development.sh
