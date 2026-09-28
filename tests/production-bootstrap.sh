#!/bin/sh
set -eu
root=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/scripts" "$tmp/bin" "$tmp/evidence"
cp "$root/scripts/apply-ci-lambda-production.sh" "$tmp/scripts/apply-ci-lambda-production.sh"
cat > "$tmp/scripts/validate-ci-lambda-production-apply.sh" <<'FAKE'
#!/bin/sh
set -eu
printf 'reviewed plan\n' > "$VALIDATED_PLAN_FILE"
export TF_CLI_CONFIG_FILE="$(dirname "$VALIDATED_PLAN_FILE")/empty.tfrc"
case "${MOCK_CONFIG_STATE:-empty}" in
  empty) : > "$TF_CLI_CONFIG_FILE" ;;
  nonempty) printf 'unexpected configuration\n' > "$TF_CLI_CONFIG_FILE" ;;
  missing) ;;
esac
FAKE
cat > "$tmp/scripts/collect-production-approval.py" <<'FAKE'
import os
from pathlib import Path
with Path(os.environ["CALL_LOG"]).open("a") as output:
    output.write("approval\n")
if os.environ.get("APPROVAL_REVOKED") == "true":
    raise SystemExit(1)
FAKE
cat > "$tmp/scripts/check-foundation-alarm-route.py" <<'FAKE'
import os
if os.environ.get("ROUTE_UNAVAILABLE") == "true":
    raise SystemExit(1)
FAKE
cat > "$tmp/scripts/resolve-production-rollback-coordinate.sh" <<'FAKE'
#!/bin/sh
set -eu
case "${HISTORY:-none}" in
  none) exit 2 ;;
  error) exit 1 ;;
  existing) printf '90\tsource\tdigest\t7\n' ;;
esac
FAKE
cat > "$tmp/scripts/check-current-main.sh" <<'FAKE'
#!/bin/sh
set -eu
printf 'main\n' >> "$CALL_LOG"
FAKE
cat > "$tmp/scripts/create-ci-lambda-rollback-plan.sh" <<'FAKE'
#!/bin/sh
set -eu
printf 'rollback\n' >> "$CALL_LOG"
FAKE
cat > "$tmp/bin/aws" <<'FAKE'
#!/bin/sh
set -eu
case "$*" in
  'lambda get-alias '*)
    printf 'alias\n' >> "$CALL_LOG"
    if [ "${SECOND_ALIAS_DRIFT:-false}" = true ] && grep -q '^main$' "$CALL_LOG"; then
      printf '{"FunctionVersion":"2","RevisionId":"changed"}\n'
    elif [ "${ALIAS_DRIFT:-false}" = true ]; then
      printf '{"FunctionVersion":"1","RevisionId":"changed"}\n'
    else
      printf '{"FunctionVersion":"1","RevisionId":"revision-1"}\n'
    fi ;;
  'lambda get-function '*) printf '%s\n' "${LIVE_IMAGE:-$BOOTSTRAP_IMAGE}" ;;
  *) exit 97 ;;
esac
FAKE
cat > "$tmp/bin/tofu" <<'FAKE'
#!/bin/sh
set -eu
for argument in "$@"; do plan_file=$argument; done
[ "${TF_CLI_CONFIG_FILE:-}" = "$(dirname "$plan_file")/empty.tfrc" ]
test -f "$TF_CLI_CONFIG_FILE"
test ! -s "$TF_CLI_CONFIG_FILE"
printf 'apply\n' >> "$CALL_LOG"
[ "${APPLY_FAIL:-false}" = false ]
FAKE
chmod +x "$tmp/bin/"* "$tmp/scripts/"*.sh
release_repository=180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases
export BOOTSTRAP_IMAGE=$release_repository@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
jq -n --arg image "$BOOTSTRAP_IMAGE" '{
  image_digest:"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
  production_deployment_id:null,prior_verified_version:null,
  bootstrap:{function_name:"portfolio-lambda-prod",alias_name:"live",alias_version:"1",
    alias_revision_id:"revision-1",image_uri:$image}
}' > "$tmp/evidence/release-identity.json"
run_apply() {
  : > "$tmp/calls"
  (cd "$tmp" && env "$@" PATH="$tmp/bin:$PATH" CALL_LOG="$tmp/calls" \
    SOURCE_SHA=cccccccccccccccccccccccccccccccccccccccc EVIDENCE_DIR="$tmp/evidence" \
    ECR_URL=180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases \
    sh scripts/apply-ci-lambda-production.sh)
}
run_apply >/dev/null
grep -q '^apply$' "$tmp/calls"
grep -q '^approval$' "$tmp/calls"
[ "$(grep -c '^alias$' "$tmp/calls")" -eq 2 ]
test -s "$tmp/evidence/APPLIED_NOT_VERIFIED"
for failure in HISTORY=error HISTORY=existing ALIAS_DRIFT=true SECOND_ALIAS_DRIFT=true \
  LIVE_IMAGE=wrong APPROVAL_REVOKED=true ROUTE_UNAVAILABLE=true \
  MOCK_CONFIG_STATE=missing MOCK_CONFIG_STATE=nonempty; do
  if run_apply "$failure" > "$tmp/output" 2>&1; then
    echo "First production apply accepted $failure" >&2; exit 1
  fi
  if grep -q '^apply$' "$tmp/calls"; then
    echo "First production apply mutated after $failure" >&2; exit 1
  fi
done
if run_apply APPLY_FAIL=true > "$tmp/output" 2>&1; then
  echo 'First production apply ignored failure' >&2; exit 1
fi
grep -q '^apply$' "$tmp/calls"
if grep -q '^rollback$' "$tmp/calls"; then
  echo 'First deployment fabricated rollback history' >&2; exit 1
fi
echo 'First production apply rechecks alias/image/history and never fabricates rollback'

# Build a real release identity from a saved-plan fixture and explicit bootstrap metadata.
mkdir -p "$tmp/deploy" "$tmp/infra/lambda/environments/prod"
cp "$root/scripts/plan-ci-lambda-production.sh" "$tmp/scripts/"
cp "$root/scripts/validate-ci-lambda-production-apply.sh" "$tmp/scripts/"
printf 'backend fixture\n' > "$tmp/infra/lambda/environments/prod/backend.hcl"
export SOURCE_SHA=cccccccccccccccccccccccccccccccccccccccc
export GITHUB_RUN_ID=10 GITHUB_RUN_ATTEMPT=1 GITHUB_REPOSITORY=CraigDevJohnson/portfolio
export ECR_URL=180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases
export ECR_REPOSITORY=portfolio-lambda-releases STATE_BUCKET=portfolio-tofu-state-180294223248
export BASE_SHA=dddddddddddddddddddddddddddddddddddddddd GITHUB_WORKSPACE="$tmp"
export CALL_LOG="$tmp/calls" PATH="$tmp/bin:$PATH" EVIDENCE_DIR="$tmp/evidence"
jq -n '{schema_version:1,source_sha:"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  image_digest:"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
  development_deployment_id:41}' > "$tmp/deploy/production-release.json"
jq '.bootstrap' "$tmp/evidence/release-identity.json" > "$tmp/bootstrap.json"
cat > "$tmp/bin/git" <<'FAKE'
#!/bin/sh
printf 'deploy/production-release.json\n'
FAKE
cat > "$tmp/scripts/fetch-ci-lambda-release-scan.sh" <<'FAKE'
#!/bin/sh
jq -n --arg digest "$IMAGE_DIGEST" '{repositoryName:"portfolio-lambda-releases",
  imageId:{imageDigest:$digest},imageScanStatus:{status:"COMPLETE"},
  imageScanFindings:{findingSeverityCounts:{CRITICAL:0}}}' > "$SCAN_FILE"
FAKE
for script in check-ci-state-bucket.sh validate-production-release.sh check-lambda-plan.sh; do
  printf '#!/bin/sh\nexit 0\n' > "$tmp/scripts/$script"
done
cat > "$tmp/scripts/create-ci-lambda-release-plan.sh" <<'FAKE'
#!/bin/sh
set -eu
jq -n --arg image "$BOOTSTRAP_IMAGE" --arg version "${PLAN_ALIAS_VERSION:-1}" '{resource_changes:[
  {address:"module.service.aws_lambda_alias.live",change:{before:
    {function_name:"portfolio-lambda-prod",name:"live",function_version:$version}}},
  {address:"module.service.aws_lambda_function.app",change:{before:
    {function_name:"portfolio-lambda-prod",version:"1",image_uri:$image}}}
]}' > "$EVIDENCE_DIR/plan.json"
cp "$EVIDENCE_DIR/plan.json" "$EVIDENCE_DIR/prod.tfplan"
printf 'rendered plan\n' > "$EVIDENCE_DIR/plan.txt"
printf 'policy passed\n' > "$EVIDENCE_DIR/policy.txt"
(cd "$EVIDENCE_DIR" && sha256sum prod.tfplan > plan.sha256)
FAKE
cat > "$tmp/bin/tofu" <<'FAKE'
#!/bin/sh
set -eu
[ "${TF_CLI_CONFIG_FILE:-}" = "$(dirname "$VALIDATED_PLAN_FILE")/empty.tfrc" ]
test -f "$TF_CLI_CONFIG_FILE"
test ! -s "$TF_CLI_CONFIG_FILE"
case "$*" in
  '-chdir=infra/lambda/environments/prod init -backend-config=backend.hcl -reconfigure -lockfile=readonly -input=false')
    printf 'init\n' >> "$MOCK_TOFU_LOG"
    [ "${MOCK_INIT_FAIL:-false}" = false ] || exit 55
    : > "$MOCK_TOFU_INITIALIZED" ;;
  '-chdir=infra/lambda/environments/prod workspace show')
    test -f "$MOCK_TOFU_INITIALIZED"
    printf 'workspace\n' >> "$MOCK_TOFU_LOG"
    printf '%s\n' "${MOCK_WORKSPACE:-default}" ;;
  *' show -json '*)
    test -f "$MOCK_TOFU_INITIALIZED"
    printf 'show\n' >> "$MOCK_TOFU_LOG"
    cat "$4" ;;
  *) exit 97 ;;
esac
FAKE
chmod +x "$tmp/bin/"* "$tmp/scripts/"*.sh
(cd "$tmp" && sh scripts/plan-ci-lambda-production.sh)
test -f "$EVIDENCE_DIR/BOOTSTRAP_REQUIRED"
jq -e '.bootstrap == null and .prior_verified_version == null and .production_deployment_id == null' \
  "$EVIDENCE_DIR/release-identity.json" >/dev/null
(cd "$tmp" && BOOTSTRAP_EVIDENCE_FILE="$tmp/bootstrap.json" sh scripts/plan-ci-lambda-production.sh)
test ! -e "$EVIDENCE_DIR/BOOTSTRAP_REQUIRED"
jq -e '.bootstrap.alias_version == "1" and .prior_verified_version == null and .production_deployment_id == null' \
  "$EVIDENCE_DIR/release-identity.json" >/dev/null
export APPROVED_PLAN_SHA256="$(awk 'NR == 1 {print $1}' "$EVIDENCE_DIR/plan.sha256")"
export APPROVED_BACKEND_SHA256="$(sha256sum "$tmp/infra/lambda/environments/prod/backend.hcl" | awk '{print $1}')"
jq -n --arg source "$SOURCE_SHA" --arg plan "$APPROVED_PLAN_SHA256" '{schema_version:1,
  environment:"production",promotion_sha:$source,plan_sha256:$plan,planning_run_id:"10",
  planning_run_attempt:"1",reviewer_login:"CraigDevJohnson",approval_id:"review-10"}' > "$EVIDENCE_DIR/approval.json"
export APPROVED_APPROVAL_SHA256="$(sha256sum "$EVIDENCE_DIR/approval.json" | awk '{print $1}')"
export APPROVED_IDENTITY_SHA256="$(sha256sum "$EVIDENCE_DIR/release-identity.json" | awk '{print $1}')"
export MOCK_TOFU_LOG="$tmp/tofu-calls" MOCK_TOFU_INITIALIZED="$tmp/tofu-initialized"
(cd "$tmp" && VALIDATED_PLAN_FILE="$tmp/accepted/prod.tfplan" sh scripts/validate-ci-lambda-production-apply.sh)
printf 'init\nworkspace\nshow\n' > "$tmp/expected-tofu-calls"
cmp "$tmp/expected-tofu-calls" "$MOCK_TOFU_LOG"
: > "$MOCK_TOFU_LOG"
rm "$MOCK_TOFU_INITIALIZED"
if (cd "$tmp" && MOCK_INIT_FAIL=true VALIDATED_PLAN_FILE="$tmp/init-failed/prod.tfplan" \
  sh scripts/validate-ci-lambda-production-apply.sh) > "$tmp/init-failed-output" 2>&1; then
  echo 'Validator ignored initialization failure' >&2; exit 1
fi
printf 'init\n' > "$tmp/expected-tofu-calls"
cmp "$tmp/expected-tofu-calls" "$MOCK_TOFU_LOG"
: > "$MOCK_TOFU_LOG"
if (cd "$tmp" && MOCK_WORKSPACE=other VALIDATED_PLAN_FILE="$tmp/wrong-workspace/prod.tfplan" \
  sh scripts/validate-ci-lambda-production-apply.sh) > "$tmp/wrong-workspace-output" 2>&1; then
  echo 'Validator accepted a non-default production workspace' >&2; exit 1
fi
grep -Fq 'Refusing non-default production OpenTofu workspace' "$tmp/wrong-workspace-output"
printf 'init\nworkspace\n' > "$tmp/expected-tofu-calls"
cmp "$tmp/expected-tofu-calls" "$MOCK_TOFU_LOG"
# Even a consistently checksummed identity cannot substitute a different observed bootstrap.
jq '.bootstrap.alias_version = "2"' "$EVIDENCE_DIR/release-identity.json" > "$tmp/changed-identity"
cp "$tmp/changed-identity" "$EVIDENCE_DIR/release-identity.json"
export APPROVED_IDENTITY_SHA256="$(sha256sum "$EVIDENCE_DIR/release-identity.json" | awk '{print $1}')"
if (cd "$tmp" && VALIDATED_PLAN_FILE="$tmp/rejected/prod.tfplan" \
  sh scripts/validate-ci-lambda-production-apply.sh) > "$tmp/output" 2>&1; then
  echo 'Validator accepted bootstrap metadata inconsistent with its saved plan' >&2; exit 1
fi
grep -Fq 'differs from the bound bootstrap resources' "$tmp/output"
if (cd "$tmp" && PLAN_ALIAS_VERSION=2 BOOTSTRAP_EVIDENCE_FILE="$tmp/bootstrap.json" \
  sh scripts/plan-ci-lambda-production.sh) > "$tmp/output" 2>&1; then
  echo 'Planner accepted bootstrap metadata inconsistent with live plan state' >&2; exit 1
fi
grep -Fq 'does not match the observed bootstrap resources' "$tmp/output"
echo 'First production planner and validator bind actual bootstrap resources without verified history'
