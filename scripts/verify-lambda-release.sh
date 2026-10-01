#!/bin/sh
# Verify a released environment: the public routes answer from the expected
# revision, the live alias runs the released image, and none of the
# environment's alarms is missing or firing.
set -eu

: "${ENVIRONMENT:?set ENVIRONMENT to dev or prod}"
: "${SOURCE_SHA:?set SOURCE_SHA}"
: "${IMAGE_DIGEST:?set IMAGE_DIGEST}"
: "${EVIDENCE_DIR:?set EVIDENCE_DIR}"

case "$ENVIRONMENT" in
  dev) hostname=dev.craigdevjohnson.com ;;
  prod) hostname=craigdevjohnson.com ;;
  *)
    echo 'ENVIRONMENT must be dev or prod' >&2
    exit 1
    ;;
esac
root=infra/lambda/environments/$ENVIRONMENT
function_name=portfolio-lambda-$ENVIRONMENT
mkdir -p "$EVIDENCE_DIR/verify-$ENVIRONMENT"
evidence="$EVIDENCE_DIR/verify-$ENVIRONMENT"

# Probe the API Gateway custom domain directly, with the public hostname and
# certificate, so Cloudflare's browser challenge does not affect the check.
# Without an active custom domain, probe the execute-api endpoint.
tofu -chdir="$root" output -json > "$evidence/outputs.json"
origin_host=$(jq -r --arg host "$hostname" \
  '.api_gateway_domain_targets.value[$host] // empty' "$evidence/outputs.json")
if [ -n "$origin_host" ]; then
  base_url="https://$hostname"
  connect_to="$hostname:443:$origin_host:443"
else
  base_url=$(jq -er '.api_default_url.value' "$evidence/outputs.json")
  connect_to=
fi

while IFS='|' read -r route name expected_content_type body_contract; do
  body_file="$evidence/$name.body"
  if [ -n "$connect_to" ]; then
    set -- --connect-to "$connect_to"
  else
    set --
  fi
  probe=$(curl -sS --connect-timeout 10 --max-time 30 --max-redirs 0 "$@" \
    -o "$body_file" --write-out '%{http_code}\n%{content_type}\n' "$base_url$route")
  status=$(printf '%s\n' "$probe" | sed -n '1p')
  content_type=$(printf '%s\n' "$probe" | sed -n '2p' |
    tr '[:upper:]' '[:lower:]' | sed 's/[[:space:]]*$//; s/;.*$//')
  [ "$status" = 200 ] || {
    printf 'Route %s returned HTTP %s instead of 200\n' "$route" "$status" >&2
    exit 1
  }
  [ "$content_type" = "$expected_content_type" ] || {
    printf 'Route %s returned %s instead of %s\n' "$route" "$content_type" "$expected_content_type" >&2
    exit 1
  }
  case "$body_contract" in
    health)
      jq -e --arg sha "$SOURCE_SHA" '.status == "ok" and .revision == $sha' "$body_file" > /dev/null || {
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
      # Binary assets must survive the API Gateway and Lambda encoding.
      [ "$(od -An -tx1 -N 3 "$body_file" | tr -d '[:space:]')" = ffd8ff ] || {
        printf 'Route %s did not return a JPEG body\n' "$route" >&2
        exit 1
      }
      ;;
  esac
done << 'EOF_ROUTES'
/healthz|healthz|application/json|health
/|home|text/html|html
/soccer|soccer|text/html|html
/static/css/tailwind.css|tailwind-css|text/css|css
/static/images/backgrounds/home-hero.jpg|home-hero-jpg|image/jpeg|jpeg
EOF_ROUTES

aws lambda get-alias --function-name "$function_name" --name live --output json > "$evidence/alias.json"
version=$(jq -er '.FunctionVersion | select(test("^[0-9]+$"))' "$evidence/alias.json")
aws lambda get-function --function-name "$function_name" --qualifier "$version" \
  --output json > "$evidence/version.json"
jq -e --arg digest "$IMAGE_DIGEST" '.Code.ImageUri | endswith("@" + $digest)' \
  "$evidence/version.json" > /dev/null || {
  echo 'The live alias does not run the released image' >&2
  exit 1
}

# The environment's outputs name its alarms: the five service alarms, plus the
# LPS history alarms only once a history stage is applied. History alarms that
# the environment does not have are neither expected nor requested, so a
# release with history off asks only for the alarms the CI roles have always
# been able to read.
jq -e --arg prefix "$function_name-" '
  .alarm_names.value as $names |
  ($names | type) == "array" and
  all($names[]; type == "string" and startswith($prefix) and test("^[a-z0-9-]+$")) and
  all("api-5xx", "api-latency", "lambda-duration", "lambda-errors", "lambda-throttles";
    ($prefix + .) as $service | any($names[]; . == $service))
' "$evidence/outputs.json" > /dev/null || {
  echo 'The environment outputs do not name its service alarms' >&2
  exit 1
}
alarm_names=$(jq -r '.alarm_names.value[]' "$evidence/outputs.json")
alarm_count=$(printf '%s\n' "$alarm_names" | wc -l | tr -d '[:space:]')
# The names were checked above to hold only [a-z0-9-], so splitting is safe.
# shellcheck disable=SC2086
aws cloudwatch describe-alarms --no-paginate --output json --alarm-names $alarm_names \
  > "$evidence/alarms.json"
jq -e --argjson count "$alarm_count" '
  (.MetricAlarms | length) == $count and all(.MetricAlarms[]; .StateValue != "ALARM")
' "$evidence/alarms.json" > /dev/null || {
  echo 'An environment alarm is missing or in ALARM' >&2
  exit 1
}

printf 'Verified %s: revision %s, image %s, live alias version %s\n' \
  "$ENVIRONMENT" "$SOURCE_SHA" "$IMAGE_DIGEST" "$version"
