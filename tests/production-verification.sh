#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/scripts"
cp "$root/scripts/verify-ci-lambda-production.sh" "$tmp/scripts/"

cat > "$tmp/scripts/verify-lambda-release.sh" <<'FAKE'
#!/bin/sh
set -eu
label=${EVIDENCE_DIR##*/}
printf '%s|%s|%s\n' "$BASE_URL" "${ORIGIN_HOST:-public}" "$EVIDENCE_DIR" >> "$CALL_LOG"
[ "${FAIL_ROUTE:-}" != "$label" ] || exit 23
version=8
[ "${MISMATCH_ROUTE:-}" != "$label" ] || version=9
mkdir -p "$EVIDENCE_DIR"
printf '{"lambda_version":"%s"}\n' "$version" > "$EVIDENCE_DIR/verification.json"
FAKE
chmod +x "$tmp/scripts/verify-lambda-release.sh"

run_verification() {
  evidence=$1
  shift
  (cd "$tmp" && env "$@" CALL_LOG="$tmp/calls" \
    SOURCE_SHA=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
    IMAGE_DIGEST=sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb \
    ORIGIN_HOST=origin.execute-api.us-west-2.amazonaws.com EVIDENCE_DIR="$evidence" \
    sh scripts/verify-ci-lambda-production.sh)
}

evidence="$tmp/evidence-success"
run_verification "$evidence"
[ "$(wc -l < "$tmp/calls" | tr -d ' ')" -eq 3 ]
grep -Fqx "https://craigdevjohnson.com|origin.execute-api.us-west-2.amazonaws.com|$evidence/origin-apex" "$tmp/calls"
grep -Fqx "https://craigdevjohnson.com|public|$evidence/public-apex" "$tmp/calls"
grep -Fqx "https://www.craigdevjohnson.com|public|$evidence/public-www" "$tmp/calls"
jq -e '.origin == "verified" and .public_apex == "verified" and
  .public_www == "verified" and .lambda_version == "8"' \
  "$evidence/production-verification.json" >/dev/null

failed_evidence="$tmp/evidence-child-failure"
if run_verification "$failed_evidence" FAIL_ROUTE=public-apex >"$tmp/failure.out" 2>&1; then
  echo 'Production verification ignored a failed child verifier' >&2
  exit 1
fi
test ! -e "$failed_evidence/production-verification.json"
test ! -e "$failed_evidence/public-www/verification.json"

mismatch_evidence="$tmp/evidence-version-mismatch"
if run_verification "$mismatch_evidence" MISMATCH_ROUTE=public-www >"$tmp/mismatch.out" 2>&1; then
  echo 'Production verification accepted inconsistent Lambda versions' >&2
  exit 1
fi
grep -Fq 'Production route verifications resolved different Lambda versions' "$tmp/mismatch.out"
test ! -e "$mismatch_evidence/production-verification.json"

echo 'Production verification routing and failure contracts passed'
