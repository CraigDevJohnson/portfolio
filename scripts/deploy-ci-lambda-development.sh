#!/bin/sh
# Plan, apply and verify one development release with the CI deployer role.
set -eu

: "${SOURCE_SHA:?set SOURCE_SHA}"
: "${IMAGE_DIGEST:?set IMAGE_DIGEST}"
: "${GITHUB_WORKSPACE:?set GITHUB_WORKSPACE}"
evidence_dir="$GITHUB_WORKSPACE/evidence"

sh scripts/check-ci-state-bucket.sh
RELEASE_ENVIRONMENT=development EVIDENCE_DIR="$evidence_dir" \
  sh scripts/create-ci-lambda-release-plan.sh

sh scripts/check-current-main.sh "$SOURCE_SHA"
tofu -chdir=infra/lambda/environments/dev apply \
  -lock-timeout=5m -input=false "$evidence_dir/dev.tfplan"

ENVIRONMENT=dev EVIDENCE_DIR="$evidence_dir" sh scripts/verify-lambda-release.sh
