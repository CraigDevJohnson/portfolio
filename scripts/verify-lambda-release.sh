#!/bin/sh
set -eu

: "${BASE_URL:?set BASE_URL}"
: "${SOURCE_SHA:?set SOURCE_SHA}"
: "${IMAGE_DIGEST:?set IMAGE_DIGEST}"
: "${FUNCTION_NAME:?set FUNCTION_NAME}"
: "${ALIAS_NAME:=live}"
: "${EVIDENCE_DIR:?set EVIDENCE_DIR}"
mkdir -p "$EVIDENCE_DIR"

curl -fsS "$BASE_URL/healthz" >"$EVIDENCE_DIR/healthz.json"
jq -e --arg sha "$SOURCE_SHA" '(.revision // .Revision) == $sha' "$EVIDENCE_DIR/healthz.json" >/dev/null
for route in / /soccer /static/css/tailwind.css; do
	name=$(printf '%s' "$route" | sed 's#^/$#home#; s#^/##; s#[/]#-#g')
	curl -fsS -D "$EVIDENCE_DIR/$name.headers" -o "$EVIDENCE_DIR/$name.body" "$BASE_URL$route"
done
aws lambda get-alias --function-name "$FUNCTION_NAME" --name "$ALIAS_NAME" --output json >"$EVIDENCE_DIR/alias.json"
version=$(jq -er '.FunctionVersion | select(test("^[0-9]+$"))' "$EVIDENCE_DIR/alias.json")
aws lambda get-function --function-name "$FUNCTION_NAME" --qualifier "$version" --output json >"$EVIDENCE_DIR/version.json"
jq -er --arg image_digest "$IMAGE_DIGEST" \
	'.Code.ImageUri | select(type == "string" and endswith("@" + $image_digest))' \
	"$EVIDENCE_DIR/version.json" >"$EVIDENCE_DIR/image-uri.txt"
aws cloudwatch describe-alarms --alarm-name-prefix "$FUNCTION_NAME" --output json >"$EVIDENCE_DIR/alarms.json"
jq -e '[.MetricAlarms[] | select(.StateValue != "OK" and .StateValue != "INSUFFICIENT_DATA")] | length == 0' "$EVIDENCE_DIR/alarms.json" >/dev/null
jq -n --arg source_sha "$SOURCE_SHA" --arg image_digest "$IMAGE_DIGEST" --arg version "$version" \
	'{source_sha:$source_sha,image_digest:$image_digest,lambda_version:$version,verified_at:(now|todate)}' >"$EVIDENCE_DIR/verification.json"
