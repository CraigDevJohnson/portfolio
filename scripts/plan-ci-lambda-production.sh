#!/bin/sh
set -eu

: "${BASE_SHA:?set BASE_SHA}"
: "${SOURCE_SHA:?set SOURCE_SHA}"
: "${GITHUB_WORKSPACE:?set GITHUB_WORKSPACE}"
: "${ECR_URL:?set ECR_URL}"
: "${STATE_BUCKET:?set STATE_BUCKET}"
: "${GITHUB_RUN_ID:?set GITHUB_RUN_ID}"
: "${GITHUB_RUN_ATTEMPT:?set GITHUB_RUN_ATTEMPT}"

changed_files=$(git diff --no-renames --name-only "$BASE_SHA" "$SOURCE_SHA")
[ "$changed_files" = deploy/production-release.json ] || {
  echo 'Production promotion must change only deploy/production-release.json' >&2
  exit 1
}
sh scripts/validate-production-release.sh deploy/production-release.json
promoted_source_sha=$(jq -er \
  '.source_sha | select(test("^[0-9a-f]{40}$"))' \
  deploy/production-release.json)
digest=$(jq -er \
  '.image_digest | select(test("^sha256:[0-9a-f]{64}$"))' \
  deploy/production-release.json)
deployment_id=$(jq -er \
  '.development_deployment_id | select(type == "number")' \
  deploy/production-release.json)

evidence_dir="$GITHUB_WORKSPACE/evidence"
mkdir -p "$evidence_dir"
printf '{"source_sha":"%s","image_digest":"%s","development_deployment_id":%s}\n' \
  "$promoted_source_sha" "$digest" "$deployment_id" > "$evidence_dir/promotion.json"
aws ecr describe-images \
  --repository-name "$ECR_REPOSITORY" \
  --image-ids "imageDigest=$digest" \
  --query 'imageDetails[0]' \
  --output json > "$evidence_dir/scan.json"
jq -e --arg digest "$digest" '
  .imageDigest == $digest and
  .imageScanStatus.status == "COMPLETE" and
  ((.imageScanFindingsSummary.findingSeverityCounts.CRITICAL // 0) == 0)
' "$evidence_dir/scan.json" > /dev/null || {
  echo 'Promoted image does not have acceptable scan evidence' >&2
  exit 1
}

sh scripts/check-current-main.sh "$SOURCE_SHA"
sh scripts/check-ci-state-bucket.sh
production_deployment_id=null
prior_verified_version=null
if coordinate=$(sh scripts/resolve-production-rollback-coordinate.sh \
  2> "$evidence_dir/rollback-coordinate-error.txt"); then
  production_deployment_id=$(printf '%s\n' "$coordinate" | cut -f1)
  prior_verified_version=$(printf '%s\n' "$coordinate" | cut -f4)
  printf '%s\n' "$coordinate" > "$evidence_dir/rollback-coordinate.tsv"
  rm -f "$evidence_dir/rollback-coordinate-error.txt"
else
  coordinate_status=$?
  [ "$coordinate_status" -eq 2 ] || {
    echo 'Could not resolve trusted production rollback evidence' >&2
    exit "$coordinate_status"
  }
  printf '%s\n' \
    'No durable verified production rollback target exists.' \
    'Activation requires independently reviewed bootstrap evidence.' \
    > "$evidence_dir/BOOTSTRAP_REQUIRED"
fi
RELEASE_ENVIRONMENT=production \
  IMAGE_DIGEST="$digest" \
  EVIDENCE_DIR="$evidence_dir" \
  sh scripts/create-ci-lambda-release-plan.sh
plan_sha256=$(awk 'NR == 1 {print $1}' "$evidence_dir/plan.sha256")
manifest_sha256=$(sha256sum deploy/production-release.json | awk '{print $1}')
scan_sha256=$(sha256sum "$evidence_dir/scan.json" | awk '{print $1}')
plan_json_sha256=$(sha256sum "$evidence_dir/plan.json" | awk '{print $1}')
plan_text_sha256=$(sha256sum "$evidence_dir/plan.txt" | awk '{print $1}')
policy_sha256=$(sha256sum "$evidence_dir/policy.txt" | awk '{print $1}')
backend_sha256=$(sha256sum infra/lambda/environments/prod/backend.hcl | awk '{print $1}')
jq -n \
  --arg promotion_sha "$SOURCE_SHA" \
  --arg development_source_sha "$promoted_source_sha" \
  --arg image_digest "$digest" \
  --argjson development_deployment_id "$deployment_id" \
  --arg planning_run_id "$GITHUB_RUN_ID" \
  --arg planning_run_attempt "$GITHUB_RUN_ATTEMPT" \
  --argjson production_deployment_id "$production_deployment_id" \
  --argjson prior_verified_version "$prior_verified_version" \
  --arg plan_sha256 "$plan_sha256" \
  --arg manifest_sha256 "$manifest_sha256" \
  --arg scan_sha256 "$scan_sha256" \
  --arg plan_json_sha256 "$plan_json_sha256" \
  --arg plan_text_sha256 "$plan_text_sha256" \
  --arg policy_sha256 "$policy_sha256" \
  --arg backend_sha256 "$backend_sha256" '
  {
    schema_version: 1,
    promotion_sha: $promotion_sha,
    development_source_sha: $development_source_sha,
    image_digest: $image_digest,
    development_deployment_id: $development_deployment_id,
    planning_run_id: $planning_run_id,
    planning_run_attempt: $planning_run_attempt,
    planning_environment: "production-plan",
    production_root: "infra/lambda/environments/prod",
    backend: {
      bucket: "portfolio-tofu-state-180294223248",
      key: "portfolio-lambda-http-api/prod/terraform.tfstate",
      region: "us-west-2",
      workspace: "default",
      sha256: $backend_sha256
    },
    production_deployment_id: $production_deployment_id,
    prior_verified_version: $prior_verified_version,
    plan_sha256: $plan_sha256,
    manifest_sha256: $manifest_sha256,
    scan_sha256: $scan_sha256,
    plan_json_sha256: $plan_json_sha256,
    plan_text_sha256: $plan_text_sha256,
    policy_sha256: $policy_sha256,
    apply_authorized: false
  }
' > "$evidence_dir/release-identity.json"
echo 'Production is plan-only until public Lambda cutover prerequisites are independently verified.' \
  > "$evidence_dir/PLAN_ONLY"
