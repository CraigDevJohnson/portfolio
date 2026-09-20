#!/bin/sh
set -eu

# Remove only in the separately authorized activation change after contract review.
echo "Production apply is disabled pending readiness and activation review" >&2
exit 1

: "${SOURCE_SHA:?set SOURCE_SHA to the promotion commit}"
: "${GITHUB_RUN_ID:?set GITHUB_RUN_ID}"
: "${GITHUB_RUN_ATTEMPT:?set GITHUB_RUN_ATTEMPT}"
: "${GITHUB_REPOSITORY:?set GITHUB_REPOSITORY}"
: "${EVIDENCE_DIR:?set EVIDENCE_DIR to the reviewed plan artifact directory}"
: "${ECR_URL:?set ECR_URL}"
: "${APPROVED_PLAN_SHA256:?set APPROVED_PLAN_SHA256}"
: "${APPROVED_IDENTITY_SHA256:?set APPROVED_IDENTITY_SHA256}"
: "${APPROVED_BACKEND_SHA256:?set APPROVED_BACKEND_SHA256}"
: "${APPROVED_APPROVAL_SHA256:?set APPROVED_APPROVAL_SHA256}"

sanitize_opentofu_environment() {
  for name in $(env | sed -n 's/^\(TF_[A-Za-z0-9_]*\)=.*/\1/p'); do
    case "$name" in TF_IN_AUTOMATION) ;; *) echo "Refusing ambient OpenTofu override: $name" >&2; return 1 ;; esac
  done
  for name in AWS_ENDPOINT_URL AWS_ENDPOINT_URL_S3 AWS_ENDPOINT_URL_DYNAMODB AWS_PROFILE AWS_SHARED_CREDENTIALS_FILE; do
    eval "present=\${$name+x}"
    [ -z "$present" ] || { echo "Refusing ambient provider override: $name" >&2; return 1; }
  done
  export TF_CLI_CONFIG_FILE="$snapshot_dir/empty.tfrc"
  : > "$TF_CLI_CONFIG_FILE"
}

case "$EVIDENCE_DIR" in
  /*) ;;
  *) echo 'EVIDENCE_DIR must be absolute' >&2; exit 1 ;;
esac
printf '%s\n' "$SOURCE_SHA" | grep -Eq '^[0-9a-f]{40}$' || {
  echo 'SOURCE_SHA must be a full lowercase commit SHA' >&2
  exit 1
}
printf '%s\n' "$APPROVED_PLAN_SHA256" | grep -Eq '^[0-9a-f]{64}$' || {
  echo 'APPROVED_PLAN_SHA256 must be a lowercase SHA-256 checksum' >&2
  exit 1
}

identity_file="$EVIDENCE_DIR/release-identity.json"
plan_file="$EVIDENCE_DIR/prod.tfplan"
plan_json="$EVIDENCE_DIR/plan.json"
plan_checksum="$EVIDENCE_DIR/plan.sha256"
scan_file="$EVIDENCE_DIR/scan.json"
approval_file="$EVIDENCE_DIR/approval.json"
manifest_file=deploy/production-release.json
for required_file in "$identity_file" "$plan_file" "$plan_json" "$plan_checksum" "$scan_file" "$approval_file"; do
  test -f "$required_file" || {
    echo "Missing reviewed production plan artifact: $required_file" >&2
    exit 1
  }
done
actual_approval_sha256=$(sha256sum "$approval_file" | awk '{print $1}')
[ "$actual_approval_sha256" = "$APPROVED_APPROVAL_SHA256" ] || {
  echo 'Protected production approval checksum does not match' >&2
  exit 1
}
jq -e \
  --arg promotion_sha "$SOURCE_SHA" \
  --arg plan_sha256 "$APPROVED_PLAN_SHA256" \
  --arg planning_run_id "$GITHUB_RUN_ID" \
  --arg planning_run_attempt "$GITHUB_RUN_ATTEMPT" '
  .schema_version == 1 and .environment == "production" and
  .promotion_sha == $promotion_sha and .plan_sha256 == $plan_sha256 and
  .planning_run_id == $planning_run_id and
  .planning_run_attempt == $planning_run_attempt and
  (.reviewer_login | type == "string" and length > 0) and
  (.approval_id | type == "string" and length > 0)
' "$approval_file" > /dev/null || {
  echo 'Protected production approval identity is inconsistent or reusable' >&2
  exit 1
}
jq -e '
  .imageScanStatus.status == "COMPLETE" and
  ((.imageScanFindings.findingSeverityCounts.CRITICAL // 0) == 0)
' "$scan_file" > /dev/null || {
  echo 'Reviewed production evidence has no acceptable completed ECR scan' >&2
  exit 1
}

identity=$(jq -cer \
  --arg promotion_sha "$SOURCE_SHA" \
  --arg planning_run_id "$GITHUB_RUN_ID" \
  --arg planning_run_attempt "$GITHUB_RUN_ATTEMPT" \
  --arg approved_plan_sha256 "$APPROVED_PLAN_SHA256" '
  select(
    .schema_version == 1 and
    .promotion_sha == $promotion_sha and
    .planning_run_id == $planning_run_id and
    .planning_run_attempt == $planning_run_attempt and
    .plan_sha256 == $approved_plan_sha256 and
    .apply_authorized == false and
    .planning_environment == "production-plan" and
    .production_root == "infra/lambda/environments/prod" and
    .backend.bucket == "portfolio-tofu-state-180294223248" and
    .backend.key == "portfolio-lambda-http-api/prod/terraform.tfstate" and
    .backend.region == "us-west-2" and .backend.workspace == "default" and
    (.development_source_sha | type == "string" and test("^[0-9a-f]{40}$")) and
    (.image_digest | type == "string" and test("^sha256:[0-9a-f]{64}$")) and
    (.development_deployment_id | type == "number" and . > 0 and floor == .) and
    (.production_deployment_id | type == "number" and . > 0 and floor == .) and
    (.prior_verified_version | type == "number" and . > 0 and floor == .) and
    (.manifest_sha256 | test("^[0-9a-f]{64}$")) and
    (.scan_sha256 | test("^[0-9a-f]{64}$")) and
    (.plan_json_sha256 | test("^[0-9a-f]{64}$")) and
    (.plan_text_sha256 | test("^[0-9a-f]{64}$")) and
    (.policy_sha256 | test("^[0-9a-f]{64}$"))
  )
' "$identity_file") || {
  echo 'Production release identity does not match this reviewed apply run' >&2
  exit 1
}

development_source_sha=$(printf '%s\n' "$identity" | jq -r .development_source_sha)
image_digest=$(printf '%s\n' "$identity" | jq -r .image_digest)
development_deployment_id=$(printf '%s\n' "$identity" | jq -r .development_deployment_id)
production_deployment_id=$(printf '%s\n' "$identity" | jq -r .production_deployment_id)
prior_alias_version=$(printf '%s\n' "$identity" | jq -r .prior_verified_version)
jq -e \
  --arg source_sha "$development_source_sha" \
  --arg image_digest "$image_digest" \
  --argjson deployment_id "$development_deployment_id" '
  .schema_version == 1 and
  .source_sha == $source_sha and
  .image_digest == $image_digest and
  .development_deployment_id == $deployment_id
' deploy/production-release.json > /dev/null || {
  echo 'Current production manifest differs from the reviewed release identity' >&2
  exit 1
}

sh scripts/check-current-main.sh "$SOURCE_SHA"
umask 077
snapshot_dir=$(mktemp -d)
trap 'rm -rf "$snapshot_dir"' EXIT
trap 'exit 1' HUP INT TERM
cp "$plan_file" "$snapshot_dir/prod.tfplan"
actual_identity_sha256=$(sha256sum "$identity_file" | awk '{print $1}')
[ "$actual_identity_sha256" = "$APPROVED_IDENTITY_SHA256" ] || {
  echo 'Reviewed production release identity checksum does not match approval' >&2; exit 1;
}
actual_backend_sha256=$(sha256sum infra/lambda/environments/prod/backend.hcl | awk '{print $1}')
[ "$actual_backend_sha256" = "$APPROVED_BACKEND_SHA256" ] || {
  echo 'Production backend checksum does not match approval' >&2; exit 1;
}
[ "$(printf '%s\n' "$identity" | jq -r .backend.sha256)" = "$actual_backend_sha256" ] || {
  echo 'Production backend identity is inconsistent' >&2; exit 1;
}
sanitize_opentofu_environment
plan_file="$snapshot_dir/prod.tfplan"
plan_json="$snapshot_dir/plan.json"
actual_plan_sha256=$(sha256sum "$plan_file" | awk '{print $1}')
test "$actual_plan_sha256" = "$APPROVED_PLAN_SHA256" || {
  echo 'Reviewed production plan checksum does not match the plan artifact' >&2
  exit 1
}
(cd "$EVIDENCE_DIR" && sha256sum -c "$(basename "$plan_checksum")" > /dev/null) || {
  echo 'Production plan checksum sidecar does not verify' >&2
  exit 1
}
tofu -chdir=infra/lambda/environments/prod show -json "$plan_file" > "$plan_json"
for binding in \
  "$manifest_file:manifest_sha256" "$scan_file:scan_sha256" \
  "$plan_json:plan_json_sha256" "$EVIDENCE_DIR/plan.txt:plan_text_sha256" \
  "$EVIDENCE_DIR/policy.txt:policy_sha256"; do
  file=${binding%:*}; field=${binding#*:}
  [ "$(sha256sum "$file" | awk '{print $1}')" = "$(printf '%s\n' "$identity" | jq -r ".$field")" ] || {
    echo "Production release artifact checksum mismatch: $field" >&2; exit 1;
  }
done
AUTOMATED_RELEASE=production \
  PLAN_JSON="$plan_json" \
  ENVIRONMENT=prod \
  NAME_PREFIX=portfolio-lambda-prod \
  IMAGE_URI="$ECR_URL@$image_digest" \
  EXPECTED_ALARM_ACTIONS_JSON='["arn:aws:sns:us-west-2:180294223248:portfolio-lambda-prod-alerts"]' \
  sh scripts/check-lambda-plan.sh

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
sh scripts/validate-production-release.sh deploy/production-release.json
current_coordinate=$(sh scripts/resolve-production-rollback-coordinate.sh)
[ "$(printf '%s\n' "$current_coordinate" | cut -f1)" = "$production_deployment_id" ] &&
  [ "$(printf '%s\n' "$current_coordinate" | cut -f4)" = "$prior_alias_version" ] || {
  echo 'Latest verified production coordinate changed after planning' >&2; exit 1;
}
sh scripts/check-current-main.sh "$SOURCE_SHA"
check_alias_version

if ! tofu -chdir=infra/lambda/environments/prod apply \
  -lock-timeout=5m \
  -input=false \
  "$plan_file"; then
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
