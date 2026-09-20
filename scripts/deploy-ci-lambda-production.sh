#!/bin/sh
set -eu

# Remove only in the separately reviewed activation change after the protected
# production Environment and role have been independently verified.
echo "Production apply is disabled pending readiness and activation review" >&2
exit 1

: "${EVIDENCE_DIR:?set EVIDENCE_DIR}"
: "${SOURCE_SHA:?set SOURCE_SHA to the promotion commit}"
: "${GITHUB_REPOSITORY:?set GITHUB_REPOSITORY}"

identity="$EVIDENCE_DIR/release-identity.json"
development_source_sha=$(jq -er .development_source_sha "$identity")
image_digest=$(jq -er .image_digest "$identity")
development_deployment_id=$(jq -er .development_deployment_id "$identity")
prior_version=$(jq -er '.prior_verified_version | tostring' "$identity")
plan_sha256=$(jq -er .plan_sha256 "$identity")

finalize() {
  result=$?
  trap - EXIT HUP INT TERM
  if [ "$result" -ne 0 ] && [ -f "$EVIDENCE_DIR/github-production-deployment.json" ]; then
    DEPLOYMENT_STATE=failure \
      DEVELOPMENT_SOURCE_SHA="$development_source_sha" \
      IMAGE_DIGEST="$image_digest" \
      DEVELOPMENT_DEPLOYMENT_ID="$development_deployment_id" \
      PLAN_SHA256="$plan_sha256" \
      sh scripts/record-ci-lambda-production.sh ||
      printf '%s\n' 'Terminal production deployment recording failed; evidence retained.' \
        > "$EVIDENCE_DIR/RECORDING_FAILED"
  fi
  exit "$result"
}
trap finalize EXIT
trap 'exit 1' HUP INT TERM

DEPLOYMENT_STATE=in_progress \
  DEVELOPMENT_SOURCE_SHA="$development_source_sha" \
  IMAGE_DIGEST="$image_digest" \
  DEVELOPMENT_DEPLOYMENT_ID="$development_deployment_id" \
  PLAN_SHA256="$plan_sha256" \
  PRIOR_VERSION="$prior_version" \
  sh scripts/record-ci-lambda-production.sh

sh scripts/apply-ci-lambda-production.sh
tofu -chdir=infra/lambda/environments/prod output -json > "$EVIDENCE_DIR/outputs.json"
origin_host=$(jq -er '.api_gateway_domain_targets.value["craigdevjohnson.com"]' \
  "$EVIDENCE_DIR/outputs.json")
ORIGIN_HOST="$origin_host" IMAGE_DIGEST="$image_digest" \
  sh scripts/verify-ci-lambda-production.sh
lambda_version=$(jq -er .lambda_version "$EVIDENCE_DIR/production-verification.json")
DEPLOYMENT_STATE=success \
  DEVELOPMENT_SOURCE_SHA="$development_source_sha" \
  IMAGE_DIGEST="$image_digest" \
  DEVELOPMENT_DEPLOYMENT_ID="$development_deployment_id" \
  PLAN_SHA256="$plan_sha256" \
  LAMBDA_VERSION="$lambda_version" \
  sh scripts/record-ci-lambda-production.sh
