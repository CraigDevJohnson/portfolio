#!/bin/sh
set -eu

# Remove only in the separately authorized activation change after contract review.
echo "Production apply is disabled pending readiness and activation review" >&2
exit 1

: "${SOURCE_SHA:?set SOURCE_SHA to the promotion commit}"
: "${EVIDENCE_DIR:?set EVIDENCE_DIR to the reviewed plan artifact directory}"
: "${ECR_URL:?set ECR_URL}"

umask 077
snapshot_dir=$(mktemp -d)
trap 'rm -rf "$snapshot_dir"' EXIT
trap 'exit 1' HUP INT TERM
validated_plan_file="$snapshot_dir/prod.tfplan"
VALIDATED_PLAN_FILE="$validated_plan_file" \
  sh scripts/validate-ci-lambda-production-apply.sh

identity_file="$EVIDENCE_DIR/release-identity.json"
identity=$(jq -cer . "$identity_file")
image_digest=$(printf '%s\n' "$identity" | jq -r .image_digest)
production_deployment_id=$(printf '%s\n' "$identity" | jq -r .production_deployment_id)
prior_alias_version=$(printf '%s\n' "$identity" | jq -r .prior_verified_version)

check_alias_version() {
  current_alias_version=$(aws lambda get-alias \
    --function-name portfolio-lambda-prod \
    --name live \
    --query FunctionVersion \
    --output text)
  test "$current_alias_version" = "$prior_alias_version" || {
    echo 'Production live alias changed after the reviewed plan was created' >&2
    return 1
  }
}
check_alias_version
current_coordinate=$(sh scripts/resolve-production-rollback-coordinate.sh)
[ "$(printf '%s\n' "$current_coordinate" | cut -f1)" = "$production_deployment_id" ] &&
  [ "$(printf '%s\n' "$current_coordinate" | cut -f4)" = "$prior_alias_version" ] || {
  echo 'Latest verified production coordinate changed after planning' >&2
  exit 1
}
sh scripts/check-current-main.sh "$SOURCE_SHA"
check_alias_version

if ! tofu -chdir=infra/lambda/environments/prod apply \
  -lock-timeout=5m \
  -input=false \
  "$validated_plan_file"; then
  RELEASE_ENVIRONMENT=production \
    PRIOR_VERSION="$prior_alias_version" \
    IMAGE_DIGEST="$image_digest" \
    EVIDENCE_DIR="$EVIDENCE_DIR" \
    ECR_URL="$ECR_URL" \
    sh scripts/create-ci-lambda-rollback-plan.sh || true
  echo 'Production apply failed; any accepted rollback plan still requires separate authorization' >&2
  exit 1
fi

printf '%s\n' 'Production saved plan applied; public service is not yet verified.' \
  > "$EVIDENCE_DIR/APPLIED_NOT_VERIFIED"
