#!/bin/sh
set -eu

: "${SOURCE_SHA:?set SOURCE_SHA}"
: "${IMAGE_DIGEST:?set IMAGE_DIGEST}"
: "${APEX_ORIGIN_HOST:?set APEX_ORIGIN_HOST}"
: "${WWW_ORIGIN_HOST:?set WWW_ORIGIN_HOST}"
: "${EVIDENCE_DIR:?set EVIDENCE_DIR}"

case "$EVIDENCE_DIR" in /*) ;; *) echo 'EVIDENCE_DIR must be absolute' >&2; exit 1 ;; esac
mkdir -p "$EVIDENCE_DIR"

fail() {
  printf 'Production OAuth verification failed: %s\n' "$1" >&2
  exit 1
}

header_value() {
  awk -v name="$2" '
    index(tolower($0), tolower(name) ":") == 1 {
      sub(/^[^:]*:[[:space:]]*/, ""); sub(/\r$/, ""); print; exit
    }
  ' "$1"
}

cookie_line() {
  awk -v name="$2" '
    index(tolower($0), "set-cookie: " tolower(name) "=") == 1 {
      sub(/\r$/, ""); print; exit
    }
  ' "$1"
}

cookie_has_attribute() {
  printf '%s\n' "$1" | tr ';' '\n' | sed 's/^[[:space:]]*//' |
    grep -Fxiq "$2"
}

check_cookie() {
  cookie=$(cookie_line "$1" "$2")
  [ -n "$cookie" ] || fail "$2 cookie is missing"
  cookie_has_attribute "$cookie" Secure || fail "$2 cookie is not Secure"
  cookie_has_attribute "$cookie" HttpOnly || fail "$2 cookie is not HttpOnly"
  cookie_has_attribute "$cookie" 'Path=/' || fail "$2 cookie path is not /"
  cookie_has_attribute "$cookie" "SameSite=$3" || fail "$2 cookie SameSite policy is not $3"
  if [ "$4" = true ]; then
    printf '%s\n' "$cookie" | grep -Eiq "^Set-Cookie:[[:space:]]*$2=;" ||
      fail "$2 cookie was not cleared"
    cookie_has_attribute "$cookie" 'Max-Age=0' || fail "$2 cookie was not expired"
  else
    printf '%s\n' "$cookie" | grep -Eiq "^Set-Cookie:[[:space:]]*$2=[^;]+;" ||
      fail "$2 cookie has no OAuth state"
  fi
}

probe_oauth() {
  oauth_dir="$EVIDENCE_DIR/oauth"
  mkdir -p "$oauth_dir"
  private_dir=$(mktemp -d)
  trap 'rm -rf "$private_dir"' EXIT HUP INT TERM

  login_status=$(curl -sS --connect-timeout 10 --max-time 30 --max-redirs 0 \
    -X POST -D "$private_dir/login.headers" -o /dev/null --write-out '%{http_code}' \
    https://craigdevjohnson.com/login)
  [ "$login_status" = 303 ] || fail "POST /login returned HTTP $login_status instead of 303"
  login_location=$(header_value "$private_dir/login.headers" Location)
  case "$login_location" in https://*/oauth2/authorize\?*) ;; *)
    fail 'POST /login did not redirect to an HTTPS Cognito authorization endpoint' ;; esac
  for contract in identity_provider=Google response_type=code \
    redirect_uri=https%3A%2F%2Fcraigdevjohnson.com%2Fcallback code_challenge_method=S256; do
    printf '%s\n' "$login_location" | grep -Fq "$contract" ||
      fail "POST /login redirect omitted $contract"
  done
  printf '%s\n' "$login_location" | grep -Eq '(^|[?&])code_challenge=[A-Za-z0-9_-]+(&|$)' ||
    fail 'POST /login redirect omitted a nonempty PKCE challenge'
  printf '%s\n' "$login_location" | grep -Eq '(^|[?&])state=[A-Za-z0-9_-]+(&|$)' ||
    fail 'POST /login redirect omitted a nonempty state'
  check_cookie "$private_dir/login.headers" mgmt_oauth_state Lax false

  callback_status=$(curl -sS --connect-timeout 10 --max-time 30 --max-redirs 0 \
    -D "$private_dir/callback.headers" -o /dev/null --write-out '%{http_code}' \
    https://craigdevjohnson.com/callback)
  [ "$callback_status" = 400 ] ||
    fail "invalid GET /callback returned HTTP $callback_status instead of 400"
  check_cookie "$private_dir/callback.headers" mgmt_oauth_state Lax true
  check_cookie "$private_dir/callback.headers" mgmt_session Strict true

  logout_status=$(curl -sS --connect-timeout 10 --max-time 30 --max-redirs 0 \
    -X POST -D "$private_dir/logout.headers" -o /dev/null --write-out '%{http_code}' \
    https://craigdevjohnson.com/logout)
  [ "$logout_status" = 302 ] || fail "POST /logout returned HTTP $logout_status instead of 302"
  logout_location=$(header_value "$private_dir/logout.headers" Location)
  case "$logout_location" in https://*/logout\?*) ;; *)
    fail 'POST /logout did not redirect to an HTTPS Cognito logout endpoint' ;; esac
  printf '%s\n' "$logout_location" |
    grep -Fq 'logout_uri=https%3A%2F%2Fcraigdevjohnson.com%2Flogin' ||
    fail 'POST /logout did not use the exact production logout URI'
  check_cookie "$private_dir/logout.headers" mgmt_oauth_state Lax true
  check_cookie "$private_dir/logout.headers" mgmt_session Strict true

  jq -n --argjson login_status "$login_status" --argjson callback_status "$callback_status" \
    --argjson logout_status "$logout_status" '{
      login:{status:$login_status,redirect:"verified",state_cookie:"verified"},
      callback:{status:$callback_status,rejection:"verified",cookies_cleared:"verified"},
      logout:{status:$logout_status,redirect:"verified",cookies_cleared:"verified"}
    }' > "$oauth_dir/verification.json"
  rm -rf "$private_dir"
  trap - EXIT HUP INT TERM
}

verify_route() {
  label=$1
  base_url=$2
  origin_host=$3
  target="$EVIDENCE_DIR/$label"
  BASE_URL="$base_url" \
    ORIGIN_HOST="$origin_host" \
    SOURCE_SHA="$SOURCE_SHA" \
    IMAGE_DIGEST="$IMAGE_DIGEST" \
    FUNCTION_NAME=portfolio-lambda-prod \
    EVIDENCE_DIR="$target" \
    sh scripts/verify-lambda-release.sh
}

# The origin probe pins the apex TLS name to API Gateway. Public probes use
# normal routing and therefore independently prove the apex and www paths.
verify_route origin-apex https://craigdevjohnson.com "$APEX_ORIGIN_HOST"
verify_route origin-www https://www.craigdevjohnson.com "$WWW_ORIGIN_HOST"
verify_route public-apex https://craigdevjohnson.com ''
verify_route public-www https://www.craigdevjohnson.com ''
probe_oauth

version=$(jq -er '.lambda_version | select(test("^[1-9][0-9]*$"))' \
  "$EVIDENCE_DIR/public-apex/verification.json")
[ "$(jq -r .lambda_version "$EVIDENCE_DIR/origin-apex/verification.json")" = "$version" ] &&
  [ "$(jq -r .lambda_version "$EVIDENCE_DIR/origin-www/verification.json")" = "$version" ] &&
  [ "$(jq -r .lambda_version "$EVIDENCE_DIR/public-www/verification.json")" = "$version" ] || {
  echo 'Production route verifications resolved different Lambda versions' >&2
  exit 1
}
jq -n --arg version "$version" --arg source_sha "$SOURCE_SHA" \
  --arg image_digest "$IMAGE_DIGEST" '{
  source_sha:$source_sha,
  image_digest:$image_digest,
  origin:"verified",
  public_apex:"verified",
  public_www:"verified",
  oauth_redirect:"verified",
  oauth_callback:"verified",
  oauth_logout:"verified",
  secure_cookies:"verified",
  lambda_version:$version
}' > "$EVIDENCE_DIR/production-verification.json"
