#!/bin/sh
set -eu
root=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir "$tmp/bin"
for command in aws tofu gh; do
  printf '#!/bin/sh\nprintf "external command invoked\\n" >> "$CALL_LOG"\nexit 99\n' > "$tmp/bin/$command"
  chmod +x "$tmp/bin/$command"
done
for entrypoint in apply-ci-lambda-production.sh deploy-ci-lambda-production.sh; do
  if PATH="$tmp/bin:$PATH" CALL_LOG="$tmp/calls" \
    PRODUCTION_APPLY_ENABLED=true PRODUCTION_ACTIVATION_PREPARATION=true \
    sh "$root/scripts/$entrypoint" > "$tmp/output" 2>&1; then
    echo "Production entrypoint unexpectedly enabled: $entrypoint" >&2
    exit 1
  fi
  grep -Fq 'Production apply is disabled pending readiness and activation review' "$tmp/output"
done
test ! -e "$tmp/calls"
apply_contract=$(cat "$root/scripts/apply-ci-lambda-production.sh" \
  "$root/scripts/validate-ci-lambda-production-apply.sh")
for guard in APPROVED_IDENTITY_SHA256 APPROVED_BACKEND_SHA256 APPROVED_APPROVAL_SHA256 \
  approval.json reviewer_login approval_id apply_authorized \
  manifest_sha256 scan_sha256 plan_json_sha256 plan_text_sha256 policy_sha256 \
  prior_verified_version production_deployment_id \
  validate-production-release.sh resolve-production-rollback-coordinate.sh; do
  printf '%s\n' "$apply_contract" | grep -Fq "$guard" || {
    echo "Production apply contract omits $guard" >&2; exit 1;
  }
done

# The active planner reuses the digest-bound scan produced by the successful
# release build, avoiding any new AWS permission on the deployed planner role.
scan_sha=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
scan_digest=sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
scan_dir="$tmp/scan-artifact"
mkdir -p "$scan_dir"
jq -n --arg digest "$scan_digest" '{
  repositoryName:"portfolio-lambda-releases", imageId:{imageDigest:$digest},
  imageScanStatus:{status:"COMPLETE"},
  imageScanFindings:{findingSeverityCounts:{CRITICAL:0}}
}' > "$scan_dir/scan.json"
(cd "$scan_dir" && zip -q "$tmp/scan.zip" scan.json)
cat > "$tmp/bin/gh" <<'GH'
#!/bin/sh
set -eu
case "$*" in
  *actions/workflows/release.yml/runs*)
    jq -nc --arg sha "$DEVELOPMENT_SOURCE_SHA" '{total_count:1,workflow_runs:[{
      id:71,run_attempt:2,head_sha:$sha,event:"workflow_run",status:"completed",
      conclusion:"success",created_at:"2026-09-20T01:00:00Z"}]}' ;;
  *actions/runs/71/artifacts*)
    jq -nc --arg name "release-$DEVELOPMENT_SOURCE_SHA" \
      '{total_count:1,artifacts:[{id:72,name:$name,expired:false}]}' ;;
  *actions/artifacts/72/zip*) cat "$SCAN_ARCHIVE" ;;
  *repos/CraigDevJohnson/portfolio/commits/main*) printf '%s\n' "$SOURCE_SHA" ;;
  *) exit 97 ;;
esac
GH
chmod +x "$tmp/bin/gh"
PATH="$tmp/bin:$PATH" SCAN_ARCHIVE="$tmp/scan.zip" \
  GITHUB_REPOSITORY=CraigDevJohnson/portfolio \
  DEVELOPMENT_SOURCE_SHA="$scan_sha" IMAGE_DIGEST="$scan_digest" \
  SCAN_FILE="$tmp/fetched-scan.json" \
  sh "$root/scripts/fetch-ci-lambda-release-scan.sh"
cmp "$scan_dir/scan.json" "$tmp/fetched-scan.json"

mkdir "$tmp/extra" && cp "$scan_dir/scan.json" "$tmp/extra/scan.json"
printf 'unexpected\n' > "$tmp/extra/other.txt"
(cd "$tmp/extra" && zip -q "$tmp/extra.zip" scan.json other.txt)
if PATH="$tmp/bin:$PATH" SCAN_ARCHIVE="$tmp/extra.zip" \
  GITHUB_REPOSITORY=CraigDevJohnson/portfolio \
  DEVELOPMENT_SOURCE_SHA="$scan_sha" IMAGE_DIGEST="$scan_digest" \
  SCAN_FILE="$tmp/rejected-scan.json" \
  sh "$root/scripts/fetch-ci-lambda-release-scan.sh" >/dev/null 2>&1; then
  echo 'Release scan fetch accepted an archive with extra entries' >&2
  exit 1
fi

# Execute early validation failures rather than merely grepping for guards.
evidence="$tmp/evidence"
mkdir "$evidence"
for file in prod.tfplan plan.json plan.txt policy.txt; do printf 'fixture\n' > "$evidence/$file"; done
printf '%s  prod.tfplan\n' "$(sha256sum "$evidence/prod.tfplan" | awk '{print $1}')" \
  > "$evidence/plan.sha256"
cp "$scan_dir/scan.json" "$evidence/scan.json"
printf '{}\n' > "$evidence/release-identity.json"
printf '{}\n' > "$evidence/approval.json"
sha64=cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc
run_validator() {
  PATH="$tmp/bin:$PATH" \
    SOURCE_SHA=dddddddddddddddddddddddddddddddddddddddd \
    GITHUB_RUN_ID=10 GITHUB_RUN_ATTEMPT=1 GITHUB_REPOSITORY=CraigDevJohnson/portfolio \
    EVIDENCE_DIR="$evidence" ECR_REPOSITORY=portfolio-lambda-releases \
    ECR_URL=example.invalid/portfolio-lambda-releases \
    APPROVED_PLAN_SHA256="$sha64" APPROVED_IDENTITY_SHA256="$sha64" \
    APPROVED_BACKEND_SHA256="$sha64" APPROVED_APPROVAL_SHA256="$sha64" \
    VALIDATED_PLAN_FILE="$tmp/validated/prod.tfplan" \
    sh "$root/scripts/validate-ci-lambda-production-apply.sh"
}
if run_validator > "$tmp/validator-output" 2>&1; then
  echo 'Production validator accepted a substituted approval checksum' >&2
  exit 1
fi
grep -Fq 'Protected production approval checksum does not match' "$tmp/validator-output"
test ! -e "$tmp/validated/prod.tfplan"

promotion_sha=dddddddddddddddddddddddddddddddddddddddd
development_sha=9528b784088f71fa39d1d7fce8570278c0d3acaf
development_digest=sha256:affaf0a7e2ff3add63db709107785c49faf5b08a8a86959bd4719402372b5776
plan_sha=$(sha256sum "$evidence/prod.tfplan" | awk '{print $1}')
backend_sha=$(sha256sum "$root/infra/lambda/environments/prod/backend.hcl" | awk '{print $1}')
make_valid_bundle() {
  critical=$1
  jq -n --arg digest "$development_digest" --argjson critical "$critical" '{
    repositoryName:"portfolio-lambda-releases", imageId:{imageDigest:$digest},
    imageScanStatus:{status:"COMPLETE"},
    imageScanFindings:{findingSeverityCounts:{CRITICAL:$critical}}
  }' > "$evidence/scan.json"
  manifest_sha=$(sha256sum "$root/deploy/production-release.json" | awk '{print $1}')
  scan_evidence_sha=$(sha256sum "$evidence/scan.json" | awk '{print $1}')
  plan_json_sha=$(sha256sum "$evidence/plan.json" | awk '{print $1}')
  plan_text_sha=$(sha256sum "$evidence/plan.txt" | awk '{print $1}')
  policy_sha=$(sha256sum "$evidence/policy.txt" | awk '{print $1}')
  jq -n --arg promotion "$promotion_sha" --arg development "$development_sha" \
    --arg digest "$development_digest" --arg plan "$plan_sha" \
    --arg backend "$backend_sha" --arg manifest "$manifest_sha" \
    --arg scan "$scan_evidence_sha" --arg plan_json "$plan_json_sha" \
    --arg plan_text "$plan_text_sha" --arg policy "$policy_sha" '{
      schema_version:1,promotion_sha:$promotion,development_source_sha:$development,
      image_digest:$digest,development_deployment_id:6179270539,
      planning_run_id:"10",planning_run_attempt:"1",planning_environment:"production-plan",
      production_root:"infra/lambda/environments/prod",backend:{
        bucket:"portfolio-tofu-state-180294223248",
        key:"portfolio-lambda-http-api/prod/terraform.tfstate",region:"us-west-2",
        workspace:"default",sha256:$backend},production_deployment_id:80,
      prior_verified_version:7,plan_sha256:$plan,manifest_sha256:$manifest,
      scan_sha256:$scan,plan_json_sha256:$plan_json,plan_text_sha256:$plan_text,
      policy_sha256:$policy,apply_authorized:false
    }' > "$evidence/release-identity.json"
  jq -n --arg promotion "$promotion_sha" --arg plan "$plan_sha" '{
    schema_version:1,environment:"production",promotion_sha:$promotion,
    plan_sha256:$plan,planning_run_id:"10",planning_run_attempt:"1",
    reviewer_login:"maintainer",approval_id:"approval-1"
  }' > "$evidence/approval.json"
}
run_valid_bundle() {
  PATH="$tmp/bin:$PATH" SCAN_ARCHIVE="$tmp/scan.zip" \
    SOURCE_SHA="$promotion_sha" GITHUB_RUN_ID="${RUN_ID:-10}" GITHUB_RUN_ATTEMPT=1 \
    GITHUB_REPOSITORY=CraigDevJohnson/portfolio EVIDENCE_DIR="$evidence" \
    ECR_REPOSITORY=portfolio-lambda-releases ECR_URL=example.invalid/portfolio-lambda-releases \
    APPROVED_PLAN_SHA256="${PLAN_APPROVAL:-$plan_sha}" \
    APPROVED_IDENTITY_SHA256="$(sha256sum "$evidence/release-identity.json" | awk '{print $1}')" \
    APPROVED_BACKEND_SHA256="${BACKEND_APPROVAL:-$backend_sha}" \
    APPROVED_APPROVAL_SHA256="$(sha256sum "$evidence/approval.json" | awk '{print $1}')" \
    VALIDATED_PLAN_FILE="$tmp/validated-${CASE_NAME:-case}/prod.tfplan" \
    sh "$root/scripts/validate-ci-lambda-production-apply.sh"
}

make_valid_bundle 0
if RUN_ID=11 CASE_NAME=wrong-run run_valid_bundle > "$tmp/wrong-run" 2>&1; then
  echo 'Production validator accepted an approval from a different run' >&2; exit 1
fi
grep -Fq 'approval identity is inconsistent' "$tmp/wrong-run"
unset RUN_ID

make_valid_bundle 1
if CASE_NAME=critical run_valid_bundle > "$tmp/critical" 2>&1; then
  echo 'Production validator accepted a critical scan finding' >&2; exit 1
fi
grep -Fq 'no acceptable digest-bound ECR scan' "$tmp/critical"

make_valid_bundle 0
if BACKEND_APPROVAL="$sha64" CASE_NAME=backend run_valid_bundle > "$tmp/backend" 2>&1; then
  echo 'Production validator accepted a substituted backend checksum' >&2; exit 1
fi
grep -Fq 'backend checksum or identity does not match' "$tmp/backend"
unset BACKEND_APPROVAL

if TF_CLI_ARGS_plan=-refresh=false CASE_NAME=ambient run_valid_bundle > "$tmp/ambient" 2>&1; then
  echo 'Production validator accepted an ambient OpenTofu override' >&2; exit 1
fi
grep -Fq 'Refusing ambient OpenTofu override' "$tmp/ambient"

if PLAN_APPROVAL="$sha64" CASE_NAME=plan run_valid_bundle > "$tmp/plan" 2>&1; then
  echo 'Production validator accepted a substituted plan checksum' >&2; exit 1
fi
grep -Eq 'approval identity is inconsistent|release identity does not match' "$tmp/plan"

echo 'Production activation and dormant validation contracts passed'
