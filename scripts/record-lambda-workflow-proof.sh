#!/bin/sh
set -eu

fail() {
	printf 'Lambda workflow recorder failed: %s\n' "$1" >&2
	exit 1
}

: "${RELEASE_RECORD:?set RELEASE_RECORD explicitly}"
: "${EVIDENCE_FILE:?set EVIDENCE_FILE explicitly}"
: "${ENVIRONMENT:?set ENVIRONMENT to development or production}"
: "${PUBLIC_HOST:?set PUBLIC_HOST explicitly}"
: "${CONNECT_REQUEST_ID:?set CONNECT_REQUEST_ID to the sanitized request ID}"
: "${ADD_REQUEST_ID:?set ADD_REQUEST_ID to the sanitized request ID}"
: "${SYNC_REQUEST_ID:?set SYNC_REQUEST_ID to the sanitized request ID}"
: "${OAUTH_OK:?set OAUTH_OK explicitly}"
: "${SECURE_COOKIES_OK:?set SECURE_COOKIES_OK explicitly}"
: "${ADD_OK:?set ADD_OK explicitly}"
: "${SYNC_OK:?set SYNC_OK explicitly}"
: "${AWS_PROFILE:?set AWS_PROFILE explicitly}"
: "${AWS_REGION:?set AWS_REGION explicitly}"

test "$AWS_PROFILE" = portfolio-deployer || fail "AWS_PROFILE must be portfolio-deployer"
test "$AWS_REGION" = us-west-2 || fail "AWS_REGION must be us-west-2"
[ -z "${AWS_ACCESS_KEY_ID+x}" ] || fail "AWS_ACCESS_KEY_ID must be unset"
[ -z "${AWS_SECRET_ACCESS_KEY+x}" ] || fail "AWS_SECRET_ACCESS_KEY must be unset"
[ -z "${AWS_SESSION_TOKEN+x}" ] || fail "AWS_SESSION_TOKEN must be unset"
case "$ENVIRONMENT" in
	development) root=infra/lambda/environments/dev ;;
	production) root=infra/lambda/environments/prod ;;
	*) fail "unsupported environment" ;;
esac
test -f "$RELEASE_RECORD" || fail "release record does not exist"
printf '%s\n' "$PUBLIC_HOST" | grep -Eq '^[A-Za-z0-9.-]+$' || fail "PUBLIC_HOST is not a hostname"
for request_id in "$CONNECT_REQUEST_ID" "$ADD_REQUEST_ID" "$SYNC_REQUEST_ID"; do
	printf '%s\n' "$request_id" | grep -Eq '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$' || fail "request IDs must be present and sanitized"
done
for flag in "$OAUTH_OK" "$SECURE_COOKIES_OK" "$ADD_OK" "$SYNC_OK"; do
	test "$flag" = true || fail "all workflow pass flags must equal true"
done

environment_record=$(jq -cer --arg environment "$ENVIRONMENT" '.[$environment]' "$RELEASE_RECORD") || fail "release record has no environment coordinates"
printf '%s\n' "$environment_record" | jq -e --arg host "$PUBLIC_HOST" '.custom_domains | index($host) != null' >/dev/null || fail "PUBLIC_HOST is not recorded for this release"
source_sha=$(jq -er '.source_sha | select(test("^[0-9a-f]{40}$"))' "$RELEASE_RECORD") || fail "invalid source SHA"
expected_digest=$(jq -er '.image.digest | select(test("^sha256:[0-9a-f]{64}$"))' "$RELEASE_RECORD") || fail "invalid image digest"
expected_version=$(printf '%s\n' "$environment_record" | jq -er '.published_version | tostring | select(length > 0)') || fail "invalid published version"
expected_alias=$(printf '%s\n' "$environment_record" | jq -er '.live_alias_target | tostring | select(length > 0)') || fail "invalid alias target"

repository_url=$(tofu -chdir=infra/lambda/artifacts output -raw ecr_repository_url)
function_name=$(tofu -chdir="$root" output -raw lambda_function_name)
live_version=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" lambda get-alias \
	--function-name "$function_name" --name live --query FunctionVersion --output text)
live_image_uri=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" lambda get-function \
	--function-name "$function_name" --qualifier "$live_version" --query Code.ImageUri --output text)
live_digest=${live_image_uri##*@}
test "$live_image_uri" = "$repository_url@$expected_digest" || fail "live image URI does not match the release record"
test "$live_digest" = "$expected_digest" || fail "live digest does not match the release record"
test "$live_version" = "$expected_version" || fail "published version does not match the release record"
test "$live_version" = "$expected_alias" || fail "live alias does not match the release record"

health_json=$(curl --fail --show-error --silent "https://$PUBLIC_HOST/healthz") || fail "public health probe failed"
health_revision=$(printf '%s\n' "$health_json" | jq -er '.revision | select(test("^[0-9a-f]{40}$"))') || fail "public health revision is invalid"
test "$health_revision" = "$source_sha" || fail "public health revision does not match the release record"

observed_epoch=$(jq -nr 'now | floor')
observed_at=$(jq -nr --argjson epoch "$observed_epoch" '$epoch | todateiso8601')
jq -nc \
	--arg environment "$ENVIRONMENT" \
	--arg observed_at "$observed_at" \
	--argjson observed_epoch "$observed_epoch" \
	--arg host "$PUBLIC_HOST" \
	--arg sha "$source_sha" \
	--arg digest "$live_digest" \
	--arg version "$live_version" \
	--arg connect_request_id "$CONNECT_REQUEST_ID" \
	--arg add_request_id "$ADD_REQUEST_ID" \
	--arg sync_request_id "$SYNC_REQUEST_ID" '
	{
		schema_version: 1,
		kind: "workflow",
		environment: $environment,
		observed_at: $observed_at,
		observed_at_epoch: $observed_epoch,
		public_hostname: $host,
		source_sha: $sha,
		image_digest: $digest,
		published_version: $version,
		alias_target: $version,
		connect_request_id: $connect_request_id,
		add_request_id: $add_request_id,
		sync_request_id: $sync_request_id,
		oauth_ok: true,
		secure_cookies_ok: true,
		add_ok: true,
		sync_ok: true,
		passed: true
	}
' >>"$EVIDENCE_FILE"

printf 'Lambda %s workflow proof recorded\n' "$ENVIRONMENT"
