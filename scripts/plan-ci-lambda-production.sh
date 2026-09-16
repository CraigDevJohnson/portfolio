#!/bin/sh
set -eu

: "${BASE_SHA:?set BASE_SHA}"
: "${SOURCE_SHA:?set SOURCE_SHA}"
: "${GITHUB_WORKSPACE:?set GITHUB_WORKSPACE}"
: "${ECR_URL:?set ECR_URL}"
: "${STATE_BUCKET:?set STATE_BUCKET}"

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

sh scripts/check-current-main.sh "$SOURCE_SHA"
sh scripts/check-ci-state-bucket.sh
RELEASE_ENVIRONMENT=production \
  IMAGE_DIGEST="$digest" \
  EVIDENCE_DIR="$evidence_dir" \
  sh scripts/create-ci-lambda-release-plan.sh
echo 'Production is plan-only until public Lambda cutover prerequisites are independently verified.' \
  > "$evidence_dir/PLAN_ONLY"
