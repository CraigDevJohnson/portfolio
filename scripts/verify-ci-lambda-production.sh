#!/bin/sh
set -eu

: "${SOURCE_SHA:?set SOURCE_SHA}"
: "${IMAGE_DIGEST:?set IMAGE_DIGEST}"
: "${ORIGIN_HOST:?set ORIGIN_HOST}"
: "${EVIDENCE_DIR:?set EVIDENCE_DIR}"

case "$EVIDENCE_DIR" in /*) ;; *) echo 'EVIDENCE_DIR must be absolute' >&2; exit 1 ;; esac
mkdir -p "$EVIDENCE_DIR"

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
verify_route origin-apex https://craigdevjohnson.com "$ORIGIN_HOST"
verify_route public-apex https://craigdevjohnson.com ''
verify_route public-www https://www.craigdevjohnson.com ''

version=$(jq -er '.lambda_version | select(test("^[1-9][0-9]*$"))' \
  "$EVIDENCE_DIR/public-apex/verification.json")
[ "$(jq -r .lambda_version "$EVIDENCE_DIR/origin-apex/verification.json")" = "$version" ] &&
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
  lambda_version:$version
}' > "$EVIDENCE_DIR/production-verification.json"
