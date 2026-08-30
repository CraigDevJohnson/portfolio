#!/bin/sh
set -eu

: "${BASE_URL:?set BASE_URL}"
: "${SOURCE_SHA:?set SOURCE_SHA}"
: "${IMAGE_DIGEST:?set IMAGE_DIGEST}"
: "${FUNCTION_NAME:?set FUNCTION_NAME}"
: "${ALIAS_NAME:=live}"
: "${EVIDENCE_DIR:?set EVIDENCE_DIR}"
mkdir -p "$EVIDENCE_DIR"

while IFS='|' read -r route name expected_content_type body_contract; do
  body_file="$EVIDENCE_DIR/$name.body"
  [ "$body_contract" != health ] || body_file="$EVIDENCE_DIR/healthz.json"
  probe=$(curl -sS \
    --connect-timeout 10 \
    --max-time 30 \
    --max-redirs 0 \
    -D "$EVIDENCE_DIR/$name.headers" \
    -o "$body_file" \
    --write-out '%{http_code}\n%{content_type}\n' \
    "$BASE_URL$route")
  status=$(printf '%s\n' "$probe" | sed -n '1p')
  content_type=$(printf '%s\n' "$probe" | sed -n '2p' |
    tr '[:upper:]' '[:lower:]' | sed 's/[[:space:]]*$//; s/;.*$//')
  [ "$status" = 200 ] || {
    printf 'Route %s returned HTTP %s instead of 200\n' "$route" "$status" >&2
    exit 1
  }
  [ "$content_type" = "$expected_content_type" ] || {
    printf 'Route %s returned %s instead of %s\n' \
      "$route" "$content_type" "$expected_content_type" >&2
    exit 1
  }
  test -s "$body_file" || {
    printf 'Route %s returned an empty body\n' "$route" >&2
    exit 1
  }
  case "$body_contract" in
    health)
      jq -e --arg sha "$SOURCE_SHA" '
        (.status // .Status) == "ok" and
        (.revision // .Revision) == $sha
      ' "$body_file" > /dev/null || {
        printf 'Route %s did not return the expected healthy revision\n' "$route" >&2
        exit 1
      }
      ;;
    html)
      grep -Eiq '<(!doctype[[:space:]]+html|html)([[:space:]>])' "$body_file" || {
        printf 'Route %s did not return an HTML document\n' "$route" >&2
        exit 1
      }
      ;;
    css)
      grep -Eq '[{}]' "$body_file" || {
        printf 'Route %s did not return a CSS stylesheet\n' "$route" >&2
        exit 1
      }
      ;;
    jpeg)
      magic=$(od -An -tx1 -N 3 "$body_file" | tr -d '[:space:]')
      [ "$magic" = ffd8ff ] || {
        printf 'Route %s did not return a JPEG body\n' "$route" >&2
        exit 1
      }
      ;;
  esac
  printf 'status=%s\ncontent_type=%s\n' "$status" "$content_type" > "$EVIDENCE_DIR/$name.probe"
done << 'EOF'
/healthz|healthz|application/json|health
/|home|text/html|html
/soccer|soccer|text/html|html
/static/css/tailwind.css|static-css-tailwind.css|text/css|css
/static/images/backgrounds/home-hero.jpg|static-images-backgrounds-home-hero.jpg|image/jpeg|jpeg
EOF
aws lambda get-alias --function-name "$FUNCTION_NAME" --name "$ALIAS_NAME" --output json > "$EVIDENCE_DIR/alias.json"
version=$(jq -er '.FunctionVersion | select(test("^[0-9]+$"))' "$EVIDENCE_DIR/alias.json")
aws lambda get-function \
  --function-name "$FUNCTION_NAME" \
  --qualifier "$version" \
  --output json > "$EVIDENCE_DIR/version.json"
jq -er --arg image_digest "$IMAGE_DIGEST" \
  '.Code.ImageUri | select(type == "string" and endswith("@" + $image_digest))' \
  "$EVIDENCE_DIR/version.json" > "$EVIDENCE_DIR/image-uri.txt"
aws cloudwatch describe-alarms --alarm-name-prefix "$FUNCTION_NAME" --output json > "$EVIDENCE_DIR/alarms.json"
jq -e --arg function_name "$FUNCTION_NAME" '
  (["api-5xx", "api-latency", "lambda-duration", "lambda-errors", "lambda-throttles"] |
    map($function_name + "-" + .) | sort) as $expected_names |
  (.MetricAlarms | type == "array") and
  ([.MetricAlarms[].AlarmName] | sort) == $expected_names and
  all(.MetricAlarms[]; .StateValue == "OK" or .StateValue == "INSUFFICIENT_DATA")
' "$EVIDENCE_DIR/alarms.json" > /dev/null
jq -n \
  --arg source_sha "$SOURCE_SHA" \
  --arg image_digest "$IMAGE_DIGEST" \
  --arg version "$version" \
  '{source_sha:$source_sha,image_digest:$image_digest,lambda_version:$version,verified_at:(now|todate)}' \
  > "$EVIDENCE_DIR/verification.json"
