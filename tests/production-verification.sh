#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/scripts" "$tmp/bin"
cp "$root/scripts/verify-ci-lambda-production.sh" "$tmp/scripts/"

cat > "$tmp/bin/curl" <<'FAKE_CURL'
#!/bin/sh
set -eu
method=GET headers= url=
while [ "$#" -gt 0 ]; do
  case "$1" in
    -X) method=$2; shift 2 ;;
    -D) headers=$2; shift 2 ;;
    -o|--write-out|--connect-timeout|--max-time|--max-redirs) shift 2 ;;
    -*) shift ;;
    *) url=$1; shift ;;
  esac
done
: "${headers:?missing header output}"
case "$method:$url" in
  POST:https://craigdevjohnson.com/login)
    callback=https%3A%2F%2Fcraigdevjohnson.com%2Fcallback
    [ "${OAUTH_FAILURE:-}" != callback-uri ] || callback=https%3A%2F%2Fwrong.example%2Fcallback
    same_site=Lax; [ "${OAUTH_FAILURE:-}" != login-samesite ] || same_site=None
    secure='; Secure'; [ "${OAUTH_FAILURE:-}" != login-secure ] || secure=
    cat > "$headers" <<EOF
HTTP/2 303
Location: https://portfolio.auth.us-west-2.amazoncognito.com/oauth2/authorize?client_id=client&code_challenge=challenge&code_challenge_method=S256&identity_provider=Google&redirect_uri=$callback&response_type=code&state=state
Set-Cookie: mgmt_oauth_state=encrypted-state; Path=/; HttpOnly$secure; SameSite=$same_site

EOF
    printf 303 ;;
  GET:https://craigdevjohnson.com/callback)
    cat > "$headers" <<'EOF'
HTTP/2 400
Set-Cookie: mgmt_oauth_state=; Path=/; Max-Age=0; HttpOnly; Secure; SameSite=Lax
Set-Cookie: mgmt_session=; Path=/; Max-Age=0; HttpOnly; Secure; SameSite=Strict

EOF
    [ "${OAUTH_FAILURE:-}" != callback-status ] && printf 400 || printf 500 ;;
  POST:https://craigdevjohnson.com/logout)
    logout=https%3A%2F%2Fcraigdevjohnson.com%2Flogin
    [ "${OAUTH_FAILURE:-}" != logout-uri ] || logout=https%3A%2F%2Fwrong.example%2Flogin
    cat > "$headers" <<EOF
HTTP/2 302
Location: https://portfolio.auth.us-west-2.amazoncognito.com/logout?client_id=client&logout_uri=$logout
Set-Cookie: mgmt_oauth_state=; Path=/; Max-Age=0; HttpOnly; Secure; SameSite=Lax
Set-Cookie: mgmt_session=; Path=/; Max-Age=0; HttpOnly; Secure; SameSite=Strict

EOF
    printf 302 ;;
  *) echo "unexpected curl request: $method $url" >&2; exit 97 ;;
esac
FAKE_CURL
chmod +x "$tmp/bin/curl"

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
  (cd "$tmp" && env "$@" PATH="$tmp/bin:$PATH" CALL_LOG="$tmp/calls" \
    SOURCE_SHA=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
    IMAGE_DIGEST=sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb \
    APEX_ORIGIN_HOST=apex.execute-api.us-west-2.amazonaws.com \
    WWW_ORIGIN_HOST=www.execute-api.us-west-2.amazonaws.com EVIDENCE_DIR="$evidence" \
    sh scripts/verify-ci-lambda-production.sh)
}

evidence="$tmp/evidence-success"
run_verification "$evidence"
[ "$(wc -l < "$tmp/calls" | tr -d ' ')" -eq 4 ]
grep -Fqx "https://craigdevjohnson.com|apex.execute-api.us-west-2.amazonaws.com|$evidence/origin-apex" "$tmp/calls"
grep -Fqx "https://www.craigdevjohnson.com|www.execute-api.us-west-2.amazonaws.com|$evidence/origin-www" "$tmp/calls"
grep -Fqx "https://craigdevjohnson.com|public|$evidence/public-apex" "$tmp/calls"
grep -Fqx "https://www.craigdevjohnson.com|public|$evidence/public-www" "$tmp/calls"
jq -e '.origin == "verified" and .public_apex == "verified" and
  .public_www == "verified" and .oauth_redirect == "verified" and
  .oauth_callback == "verified" and .oauth_logout == "verified" and
  .secure_cookies == "verified" and .lambda_version == "8"' \
  "$evidence/production-verification.json" >/dev/null
jq -e '.login.status == 303 and .callback.status == 400 and .logout.status == 302' \
  "$evidence/oauth/verification.json" >/dev/null

failed_evidence="$tmp/evidence-child-failure"
if run_verification "$failed_evidence" FAIL_ROUTE=origin-www >"$tmp/failure.out" 2>&1; then
  echo 'Production verification ignored a failed child verifier' >&2
  exit 1
fi
test ! -e "$failed_evidence/production-verification.json"
test ! -e "$failed_evidence/public-apex/verification.json"

mismatch_evidence="$tmp/evidence-version-mismatch"
if run_verification "$mismatch_evidence" MISMATCH_ROUTE=origin-www >"$tmp/mismatch.out" 2>&1; then
  echo 'Production verification accepted inconsistent Lambda versions' >&2
  exit 1
fi
grep -Fq 'Production route verifications resolved different Lambda versions' "$tmp/mismatch.out"
test ! -e "$mismatch_evidence/production-verification.json"

for oauth_failure in callback-uri login-secure login-samesite callback-status logout-uri; do
  oauth_evidence="$tmp/evidence-oauth-$oauth_failure"
  if run_verification "$oauth_evidence" OAUTH_FAILURE="$oauth_failure" >"$tmp/oauth-$oauth_failure.out" 2>&1; then
    echo "Production verification accepted invalid OAuth contract: $oauth_failure" >&2
    exit 1
  fi
  grep -Fq 'Production OAuth verification failed:' "$tmp/oauth-$oauth_failure.out"
  test ! -e "$oauth_evidence/production-verification.json"
done

echo 'Production verification routing, OAuth, cookie, and failure contracts passed'
