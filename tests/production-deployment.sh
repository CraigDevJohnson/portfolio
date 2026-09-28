#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/scripts" "$tmp/bin"

cp "$root/scripts/deploy-ci-lambda-production.sh" "$tmp/scripts/deploy-ci-lambda-production.sh"

cat > "$tmp/scripts/record-ci-lambda-production.sh" <<'FAKE'
#!/bin/sh
set -eu
: "${RELEASE_IDENTITY_SHA256:?}"
: "${SCAN_SHA256:?}"
: "${PLANNING_RUN_ID:?}"
: "${PLANNING_RUN_ATTEMPT:?}"
: "${APPROVAL_ID:?}"
: "${REVIEWER_LOGIN:?}"
printf 'record:%s\n' "$DEPLOYMENT_STATE" >> "$CALL_LOG"
if [ "$DEPLOYMENT_STATE" = in_progress ]; then
  if [ "${FAIL_STAGE:-}" = in-progress-response ]; then
    printf '{}\n' > "$EVIDENCE_DIR/github-production-deployment-response.json"
    exit 1
  fi
  printf '{"production_deployment_id":91}\n' > "$EVIDENCE_DIR/github-production-deployment.json"
fi
FAKE
cat > "$tmp/scripts/apply-ci-lambda-production.sh" <<'FAKE'
#!/bin/sh
set -eu
printf 'apply\n' >> "$CALL_LOG"
[ "${FAIL_STAGE:-}" != apply ]
FAKE
cat > "$tmp/scripts/verify-ci-lambda-production.sh" <<'FAKE'
#!/bin/sh
set -eu
: "${EXPECTED_VERIFICATION_SHA:?}"
[ "$SOURCE_SHA" = "$EXPECTED_VERIFICATION_SHA" ]
[ "$PROMOTION_SHA" != "$SOURCE_SHA" ]
[ "$PRODUCTION_DEPLOYMENT_ID" = 91 ]
[ "$API_ID" = api123 ]
printf 'origin:%s\n' "$APEX_ORIGIN_HOST" >> "$CALL_LOG"
printf 'verify\n' >> "$CALL_LOG"
[ "${FAIL_STAGE:-}" != verify ]
printf '{"lambda_version":"8"}\n' > "$EVIDENCE_DIR/automated-window.json"
FAKE
cat > "$tmp/bin/tofu" <<'FAKE'
#!/bin/sh
set -eu
printf 'output\n' >> "$CALL_LOG"
[ "${FAIL_STAGE:-}" != output ]
printf '{"api_id":{"value":"api123"},"api_gateway_domain_targets":{"value":{"craigdevjohnson.com":"apex-origin.example"}}}\n'
FAKE
chmod +x "$tmp/scripts/"*.sh "$tmp/bin/tofu"

source_sha=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
development_sha=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
digest=sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc
plan=dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd

run_deployment() {
  evidence=$1
  shift
  mkdir -p "$evidence"
  jq -n --arg development_source_sha "$development_sha" --arg image_digest "$digest" \
    --arg plan_sha256 "$plan" '{
      development_source_sha: $development_source_sha,
      image_digest: $image_digest,
      development_deployment_id: 41,
      prior_verified_version: 7,
      plan_sha256: $plan_sha256,
      scan_sha256: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
      planning_run_id: "123456",
      planning_run_attempt: "2"
    }' > "$evidence/release-identity.json"
  printf '{"approval_id":"approval-789","reviewer_login":"CraigDevJohnson"}\n' \
    > "$evidence/approval.json"
  (cd "$tmp" && env "$@" PATH="$tmp/bin:$PATH" CALL_LOG="$evidence/calls" \
    EVIDENCE_DIR="$evidence" SOURCE_SHA="$source_sha" \
    EXPECTED_VERIFICATION_SHA="$development_sha" \
    GITHUB_REPOSITORY=CraigDevJohnson/portfolio \
    sh scripts/deploy-ci-lambda-production.sh)
}

success="$tmp/success"
run_deployment "$success"
cat > "$tmp/expected-success" <<'EOF'
record:in_progress
apply
output
origin:apex-origin.example
verify
EOF
cmp "$tmp/expected-success" "$success/calls"
test -s "$success/APPLIED_NOT_VERIFIED"
test ! -e "$success/production-verification.json"

for stage in apply output verify; do
  evidence="$tmp/failure-$stage"
  if run_deployment "$evidence" FAIL_STAGE="$stage" >"$tmp/$stage.out" 2>&1; then
    echo "Production deployment ignored $stage failure" >&2
    exit 1
  fi
  grep -Fqx 'record:in_progress' "$evidence/calls"
  grep -Fqx 'record:failure' "$evidence/calls"
  if grep -Fqx 'record:success' "$evidence/calls"; then
    echo "Production deployment recorded success after $stage failure" >&2
    exit 1
  fi
done

# A create response must trigger failure finalization even if normalized evidence
# was not written before the in-progress recorder stopped.
response_only="$tmp/failure-in-progress-response"
if run_deployment "$response_only" FAIL_STAGE=in-progress-response \
  >"$tmp/in-progress-response.out" 2>&1; then
  echo 'Production deployment ignored interrupted in-progress recording' >&2
  exit 1
fi
grep -Fqx 'record:in_progress' "$response_only/calls"
grep -Fqx 'record:failure' "$response_only/calls"
test -f "$response_only/github-production-deployment-response.json"

# An incomplete invocation must fail before touching deployment state.
if (unset EVIDENCE_DIR; PATH="$tmp/bin:$PATH" CALL_LOG="$tmp/missing-input-calls" \
  sh "$root/scripts/deploy-ci-lambda-production.sh") >"$tmp/missing-input.out" 2>&1; then
  echo 'Production deployment accepted missing evidence' >&2
  exit 1
fi
grep -Fq 'set EVIDENCE_DIR' "$tmp/missing-input.out"
test ! -e "$tmp/missing-input-calls"

echo 'Production deployment orchestration contracts passed'
