#!/bin/sh
set -eu
root=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/scripts" "$tmp/evidence"
cp "$root/scripts/verify-ci-lambda-production.sh" "$tmp/scripts/" 2>/dev/null || true
cat > "$tmp/scripts/verify-lambda-release.sh" <<'FAKE'
#!/bin/sh
set -eu
printf '%s|%s|%s\n' "$BASE_URL" "${ORIGIN_HOST:-public}" "$EVIDENCE_DIR" >> "$CALL_LOG"
mkdir -p "$EVIDENCE_DIR"
printf '{"lambda_version":"8"}\n' > "$EVIDENCE_DIR/verification.json"
FAKE
chmod +x "$tmp/scripts/verify-lambda-release.sh"
set +e
(cd "$tmp" && CALL_LOG="$tmp/calls" SOURCE_SHA=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
  IMAGE_DIGEST=sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb \
  ORIGIN_HOST=origin.execute-api.us-west-2.amazonaws.com EVIDENCE_DIR="$tmp/evidence" \
  sh scripts/verify-ci-lambda-production.sh) >"$tmp/out" 2>&1
rc=$?
set -e
[ "$rc" -eq 0 ] || { cat "$tmp/out"; exit 1; }
[ "$(wc -l < "$tmp/calls" | tr -d ' ')" -eq 3 ]
grep -Fqx "https://craigdevjohnson.com|origin.execute-api.us-west-2.amazonaws.com|$tmp/evidence/origin-apex" "$tmp/calls"
grep -Fqx "https://craigdevjohnson.com|public|$tmp/evidence/public-apex" "$tmp/calls"
grep -Fqx "https://www.craigdevjohnson.com|public|$tmp/evidence/public-www" "$tmp/calls"
jq -e '.origin == "verified" and .public_apex == "verified" and .public_www == "verified" and .lambda_version == "8"' "$tmp/evidence/production-verification.json" >/dev/null
echo 'Production verification routing contracts passed'
