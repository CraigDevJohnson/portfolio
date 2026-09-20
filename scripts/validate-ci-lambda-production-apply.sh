#!/bin/sh
set -eu

: "${SOURCE_SHA:?set SOURCE_SHA to the promotion commit}"
: "${GITHUB_RUN_ID:?set GITHUB_RUN_ID}"
: "${GITHUB_RUN_ATTEMPT:?set GITHUB_RUN_ATTEMPT}"
: "${GITHUB_REPOSITORY:?set GITHUB_REPOSITORY}"
: "${EVIDENCE_DIR:?set EVIDENCE_DIR to the reviewed plan artifact directory}"
: "${ECR_REPOSITORY:?set ECR_REPOSITORY}"
: "${ECR_URL:?set ECR_URL}"
: "${APPROVED_PLAN_SHA256:?set APPROVED_PLAN_SHA256}"
: "${APPROVED_IDENTITY_SHA256:?set APPROVED_IDENTITY_SHA256}"
: "${APPROVED_BACKEND_SHA256:?set APPROVED_BACKEND_SHA256}"
: "${APPROVED_APPROVAL_SHA256:?set APPROVED_APPROVAL_SHA256}"
: "${VALIDATED_PLAN_FILE:?set VALIDATED_PLAN_FILE to an absolute private destination}"

case "$EVIDENCE_DIR:$VALIDATED_PLAN_FILE" in
  /*:/*) ;;
  *) echo 'EVIDENCE_DIR and VALIDATED_PLAN_FILE must be absolute' >&2; exit 1 ;;
esac
printf '%s\n' "$SOURCE_SHA" | grep -Eq '^[0-9a-f]{40}$' || {
  echo 'SOURCE_SHA must be a full lowercase commit SHA' >&2
  exit 1
}
for checksum in "$APPROVED_PLAN_SHA256" "$APPROVED_IDENTITY_SHA256" \
  "$APPROVED_BACKEND_SHA256" "$APPROVED_APPROVAL_SHA256"; do
  printf '%s\n' "$checksum" | grep -Eq '^[0-9a-f]{64}$' || {
    echo 'Approved checksums must be lowercase SHA-256 checksums' >&2
    exit 1
  }
done

identity_file="$EVIDENCE_DIR/release-identity.json"
source_plan_file="$EVIDENCE_DIR/prod.tfplan"
source_plan_json="$EVIDENCE_DIR/plan.json"
plan_checksum="$EVIDENCE_DIR/plan.sha256"
scan_file="$EVIDENCE_DIR/scan.json"
approval_file="$EVIDENCE_DIR/approval.json"
manifest_file=deploy/production-release.json
for required_file in "$identity_file" "$source_plan_file" "$source_plan_json" \
  "$plan_checksum" "$scan_file" "$approval_file" "$EVIDENCE_DIR/plan.txt" \
  "$EVIDENCE_DIR/policy.txt"; do
  test -f "$required_file" || {
    echo "Missing reviewed production plan artifact: $required_file" >&2
    exit 1
  }
done

[ "$(sha256sum "$approval_file" | awk '{print $1}')" = "$APPROVED_APPROVAL_SHA256" ] || {
  echo 'Protected production approval checksum does not match' >&2
  exit 1
}
jq -e \
  --arg promotion_sha "$SOURCE_SHA" \
  --arg plan_sha256 "$APPROVED_PLAN_SHA256" \
  --arg planning_run_id "$GITHUB_RUN_ID" \
  --arg planning_run_attempt "$GITHUB_RUN_ATTEMPT" '
  type == "object" and
  (keys | sort) == (["approval_id", "environment", "plan_sha256", "planning_run_attempt",
    "planning_run_id", "promotion_sha", "reviewer_login", "schema_version"] | sort) and
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

identity=$(jq -cer \
  --arg promotion_sha "$SOURCE_SHA" \
  --arg planning_run_id "$GITHUB_RUN_ID" \
  --arg planning_run_attempt "$GITHUB_RUN_ATTEMPT" \
  --arg approved_plan_sha256 "$APPROVED_PLAN_SHA256" '
  select(
    type == "object" and .schema_version == 1 and
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
    (.backend.sha256 | type == "string" and test("^[0-9a-f]{64}$")) and
    (.development_source_sha | type == "string" and test("^[0-9a-f]{40}$")) and
    (.image_digest | type == "string" and test("^sha256:[0-9a-f]{64}$")) and
    (.development_deployment_id | type == "number" and . > 0 and floor == .) and
    (.production_deployment_id | type == "number" and . > 0 and floor == .) and
    (.prior_verified_version | type == "number" and . > 0 and floor == .) and
    (.manifest_sha256 | type == "string" and test("^[0-9a-f]{64}$")) and
    (.scan_sha256 | type == "string" and test("^[0-9a-f]{64}$")) and
    (.plan_json_sha256 | type == "string" and test("^[0-9a-f]{64}$")) and
    (.plan_text_sha256 | type == "string" and test("^[0-9a-f]{64}$")) and
    (.policy_sha256 | type == "string" and test("^[0-9a-f]{64}$"))
  )
' "$identity_file") || {
  echo 'Production release identity does not match this reviewed apply run' >&2
  exit 1
}

development_source_sha=$(printf '%s\n' "$identity" | jq -r .development_source_sha)
image_digest=$(printf '%s\n' "$identity" | jq -r .image_digest)
development_deployment_id=$(printf '%s\n' "$identity" | jq -r .development_deployment_id)
jq -e --arg digest "$image_digest" '
  .repositoryName == "portfolio-lambda-releases" and
  .imageId.imageDigest == $digest and
  .imageScanStatus.status == "COMPLETE" and
  (.imageScanFindings.findingSeverityCounts | type == "object") and
  ((.imageScanFindings.findingSeverityCounts.CRITICAL // 0) == 0)
' "$scan_file" > /dev/null || {
  echo 'Reviewed production evidence has no acceptable digest-bound ECR scan' >&2
  exit 1
}
jq -e \
  --arg source_sha "$development_source_sha" \
  --arg image_digest "$image_digest" \
  --argjson deployment_id "$development_deployment_id" '
  .schema_version == 1 and .source_sha == $source_sha and
  .image_digest == $image_digest and .development_deployment_id == $deployment_id
' "$manifest_file" > /dev/null || {
  echo 'Current production manifest differs from the reviewed release identity' >&2
  exit 1
}

sh scripts/check-current-main.sh "$SOURCE_SHA"
[ "$(sha256sum "$identity_file" | awk '{print $1}')" = "$APPROVED_IDENTITY_SHA256" ] || {
  echo 'Reviewed production release identity checksum does not match approval' >&2
  exit 1
}
actual_backend_sha256=$(sha256sum infra/lambda/environments/prod/backend.hcl | awk '{print $1}')
[ "$actual_backend_sha256" = "$APPROVED_BACKEND_SHA256" ] &&
  [ "$(printf '%s\n' "$identity" | jq -r .backend.sha256)" = "$actual_backend_sha256" ] || {
  echo 'Production backend checksum or identity does not match approval' >&2
  exit 1
}

for name in $(env | sed -n 's/^\(TF_[A-Za-z0-9_]*\)=.*/\1/p'); do
  case "$name" in
    TF_IN_AUTOMATION) ;;
    *) echo "Refusing ambient OpenTofu override: $name" >&2; exit 1 ;;
  esac
done
for name in AWS_ENDPOINT_URL AWS_ENDPOINT_URL_S3 AWS_ENDPOINT_URL_DYNAMODB \
  AWS_PROFILE AWS_SHARED_CREDENTIALS_FILE; do
  eval "present=\${$name+x}"
  [ -z "$present" ] || { echo "Refusing ambient provider override: $name" >&2; exit 1; }
done

umask 077
mkdir -p "$(dirname "$VALIDATED_PLAN_FILE")"
test ! -e "$VALIDATED_PLAN_FILE" || {
  echo 'Validated plan destination already exists' >&2
  exit 1
}
cp "$source_plan_file" "$VALIDATED_PLAN_FILE"
[ "$(sha256sum "$VALIDATED_PLAN_FILE" | awk '{print $1}')" = "$APPROVED_PLAN_SHA256" ] || {
  echo 'Reviewed production plan checksum does not match the plan artifact' >&2
  exit 1
}
(cd "$EVIDENCE_DIR" && sha256sum -c "$(basename "$plan_checksum")" > /dev/null) || {
  echo 'Production plan checksum sidecar does not verify' >&2
  exit 1
}

validation_dir=$(dirname "$VALIDATED_PLAN_FILE")
export TF_CLI_CONFIG_FILE="$validation_dir/empty.tfrc"
: > "$TF_CLI_CONFIG_FILE"
rendered_plan_json="$validation_dir/plan.json"
tofu -chdir=infra/lambda/environments/prod show -json "$VALIDATED_PLAN_FILE" > "$rendered_plan_json"
for binding in \
  "$manifest_file:manifest_sha256" "$scan_file:scan_sha256" \
  "$rendered_plan_json:plan_json_sha256" "$EVIDENCE_DIR/plan.txt:plan_text_sha256" \
  "$EVIDENCE_DIR/policy.txt:policy_sha256"; do
  file=${binding%:*}; field=${binding#*:}
  [ "$(sha256sum "$file" | awk '{print $1}')" = "$(printf '%s\n' "$identity" | jq -r ".$field")" ] || {
    echo "Production release artifact checksum mismatch: $field" >&2
    exit 1
  }
done
AUTOMATED_RELEASE=production \
  PLAN_JSON="$rendered_plan_json" \
  ENVIRONMENT=prod \
  NAME_PREFIX=portfolio-lambda-prod \
  IMAGE_URI="$ECR_URL@$image_digest" \
  EXPECTED_ALARM_ACTIONS_JSON='["arn:aws:sns:us-west-2:180294223248:portfolio-lambda-prod-alerts"]' \
  sh scripts/check-lambda-plan.sh
sh scripts/validate-production-release.sh "$manifest_file"
