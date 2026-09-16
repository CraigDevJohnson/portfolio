#!/bin/sh
set -eu

: "${BASE_URL:?set BASE_URL}"
: "${SOURCE_SHA:?set SOURCE_SHA}"
: "${IMAGE_DIGEST:?set IMAGE_DIGEST}"
: "${FUNCTION_NAME:?set FUNCTION_NAME}"
: "${ALIAS_NAME:=live}"
: "${EVIDENCE_DIR:?set EVIDENCE_DIR}"
ORIGIN_HOST=${ORIGIN_HOST:-}
SMOKE_WINDOW_SECONDS=${SMOKE_WINDOW_SECONDS:-300}
SMOKE_INTERVAL_SECONDS=${SMOKE_INTERVAL_SECONDS:-30}

is_positive_decimal() {
  case "$1" in
    ''|*[!0-9]*|0*) return 1 ;;
    *) return 0 ;;
  esac
}

is_hostname() {
  printf '%s\n' "$1" |
    grep -Eq '^([A-Za-z0-9][A-Za-z0-9-]*\.)*[A-Za-z0-9][A-Za-z0-9-]*$'
}

case "$BASE_URL" in
  https://*) base_host=${BASE_URL#https://} ;;
  *)
    echo 'BASE_URL must be an HTTPS origin without a path' >&2
    exit 1
    ;;
esac
is_hostname "$base_host" || {
  echo 'BASE_URL must be an HTTPS origin without a path' >&2
  exit 1
}
if [ -n "$ORIGIN_HOST" ]; then
  is_hostname "$ORIGIN_HOST" || {
    echo 'ORIGIN_HOST must be a hostname' >&2
    exit 1
  }
fi

is_positive_decimal "$SMOKE_WINDOW_SECONDS" || {
  echo 'SMOKE_WINDOW_SECONDS must be a positive decimal integer' >&2
  exit 1
}
is_positive_decimal "$SMOKE_INTERVAL_SECONDS" || {
  echo 'SMOKE_INTERVAL_SECONDS must be a positive decimal integer' >&2
  exit 1
}
[ "$SMOKE_WINDOW_SECONDS" -ge 300 ] && [ "$SMOKE_WINDOW_SECONDS" -le 900 ] || {
  echo 'SMOKE_WINDOW_SECONDS must be between 300 and 900 seconds' >&2
  exit 1
}
[ "$SMOKE_INTERVAL_SECONDS" -le 30 ] || {
  echo 'SMOKE_INTERVAL_SECONDS must not exceed 30 seconds' >&2
  exit 1
}
[ $((SMOKE_WINDOW_SECONDS % SMOKE_INTERVAL_SECONDS)) -eq 0 ] || {
  echo 'SMOKE_WINDOW_SECONDS must be divisible by SMOKE_INTERVAL_SECONDS' >&2
  exit 1
}
smoke_observations=$((SMOKE_WINDOW_SECONDS / SMOKE_INTERVAL_SECONDS + 1))
mkdir -p "$EVIDENCE_DIR"
jq -n \
  --arg base_url "$BASE_URL" \
  --arg origin_host "$ORIGIN_HOST" \
  '{base_url:$base_url,origin_host:(if $origin_host == "" then null else $origin_host end)}' \
  > "$EVIDENCE_DIR/route-probe-target.json"
jq -n \
  --argjson window_seconds "$SMOKE_WINDOW_SECONDS" \
  --argjson interval_seconds "$SMOKE_INTERVAL_SECONDS" \
  --argjson observations "$smoke_observations" \
  '{window_seconds:$window_seconds,interval_seconds:$interval_seconds,observations:$observations}' \
  > "$EVIDENCE_DIR/alarm-smoke-window.json"

while IFS='|' read -r route name expected_content_type body_contract; do
  body_file="$EVIDENCE_DIR/$name.body"
  [ "$body_contract" != health ] || body_file="$EVIDENCE_DIR/healthz.json"
  if [ -n "$ORIGIN_HOST" ]; then
    probe=$(curl -sS \
      --connect-timeout 10 \
      --max-time 30 \
      --max-redirs 0 \
      --connect-to "$base_host:443:$ORIGIN_HOST:443" \
      -D "$EVIDENCE_DIR/$name.headers" \
      -o "$body_file" \
      --write-out '%{http_code}\n%{content_type}\n' \
      "$BASE_URL$route")
  else
    probe=$(curl -sS \
      --connect-timeout 10 \
      --max-time 30 \
      --max-redirs 0 \
      -D "$EVIDENCE_DIR/$name.headers" \
      -o "$body_file" \
      --write-out '%{http_code}\n%{content_type}\n' \
      "$BASE_URL$route")
  fi
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
alarm_observation=1
while [ "$alarm_observation" -le "$smoke_observations" ]; do
  alarm_file=$(printf '%s/alarms-%03d.json' "$EVIDENCE_DIR" "$alarm_observation")
  aws cloudwatch describe-alarms \
    --alarm-names \
    "$FUNCTION_NAME-api-5xx" \
    "$FUNCTION_NAME-api-latency" \
    "$FUNCTION_NAME-lambda-duration" \
    "$FUNCTION_NAME-lambda-errors" \
    "$FUNCTION_NAME-lambda-throttles" \
    --no-paginate \
    --output json > "$alarm_file"
  cp "$alarm_file" "$EVIDENCE_DIR/alarms.json"
  jq -e --arg function_name "$FUNCTION_NAME" '
    (["api-5xx", "api-latency", "lambda-duration", "lambda-errors", "lambda-throttles"] |
      map($function_name + "-" + .) | sort) as $expected_names |
    (.MetricAlarms | type == "array") and
    ([.MetricAlarms[].AlarmName] | sort) == $expected_names and
    all(.MetricAlarms[]; .StateValue == "OK")
  ' "$alarm_file" > /dev/null
  if [ "$alarm_observation" -lt "$smoke_observations" ]; then
    sleep "$SMOKE_INTERVAL_SECONDS"
  fi
  alarm_observation=$((alarm_observation + 1))
done
jq -n \
  --arg source_sha "$SOURCE_SHA" \
  --arg image_digest "$IMAGE_DIGEST" \
  --arg version "$version" \
  '{source_sha:$source_sha,image_digest:$image_digest,lambda_version:$version,verified_at:(now|todate)}' \
  > "$EVIDENCE_DIR/verification.json"
