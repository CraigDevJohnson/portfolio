#!/bin/sh
set -eu

fail() {
	printf 'Lambda observation recorder failed: %s\n' "$1" >&2
	exit 1
}

: "${RELEASE_RECORD:?set RELEASE_RECORD explicitly}"
: "${EVIDENCE_FILE:?set EVIDENCE_FILE explicitly}"
: "${ENVIRONMENT:?set ENVIRONMENT to development or production}"
: "${AWS_PROFILE:?set AWS_PROFILE explicitly}"
: "${AWS_REGION:?set AWS_REGION explicitly}"

test "$AWS_PROFILE" = portfolio-deployer || fail "AWS_PROFILE must be portfolio-deployer"
test "$AWS_REGION" = us-west-2 || fail "AWS_REGION must be us-west-2"
[ -z "${AWS_ACCESS_KEY_ID+x}" ] || fail "AWS_ACCESS_KEY_ID must be unset"
[ -z "${AWS_SECRET_ACCESS_KEY+x}" ] || fail "AWS_SECRET_ACCESS_KEY must be unset"
[ -z "${AWS_SESSION_TOKEN+x}" ] || fail "AWS_SESSION_TOKEN must be unset"
test -f "$RELEASE_RECORD" || fail "release record does not exist"
case "$ENVIRONMENT" in
	development) root=infra/lambda/environments/dev ;;
	production) root=infra/lambda/environments/prod ;;
	*) fail "unsupported environment" ;;
esac

environment_record=$(jq -cer --arg environment "$ENVIRONMENT" '.[$environment]' "$RELEASE_RECORD") ||
	fail "release record has no environment coordinates"
cutover_epoch=$(printf '%s\n' "$environment_record" | jq -er '.dns_cutover_at | fromdateiso8601') || fail "invalid DNS cutover timestamp"
source_sha=$(jq -er '.source_sha | select(test("^[0-9a-f]{40}$"))' "$RELEASE_RECORD") || fail "invalid source SHA"
expected_digest=$(jq -er '.image.digest | select(test("^sha256:[0-9a-f]{64}$"))' "$RELEASE_RECORD") || fail "invalid image digest"
expected_version=$(printf '%s\n' "$environment_record" | jq -er '.published_version | tostring | select(length > 0)') || fail "invalid published version"
expected_alias=$(printf '%s\n' "$environment_record" | jq -er '.live_alias_target | tostring | select(length > 0)') || fail "invalid alias target"
public_host=$(printf '%s\n' "$environment_record" | jq -er '.custom_domains[0] | select(type == "string" and test("^[A-Za-z0-9.-]+$"))') || fail "invalid public hostname"
rollback_evidence=$(printf '%s\n' "$environment_record" | jq -er '.rollback_evidence | select(type == "string" and length > 0)') || fail "missing rollback evidence path"
test -f "$rollback_evidence" || fail "rollback evidence does not exist"
rollback_origin=$(jq -er --arg environment "$ENVIRONMENT" '
	def strict_https_origin:
		type == "string" and
		test("^https://[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)*(?::[0-9]{1,5})?$") and
		(if test(":[0-9]+$") then
			(capture(":(?<port>[0-9]+)$").port | tonumber) as $port |
			$port >= 1 and $port <= 65535
		else true end);
	select(.schema_version == 1 and .environment == $environment) |
	.rollback_origin_url | select(strict_https_origin)
' "$rollback_evidence") || fail "invalid rollback evidence"

repository_url=$(tofu -chdir=infra/lambda/artifacts output -raw ecr_repository_url)
function_name=$(tofu -chdir="$root" output -raw lambda_function_name)
api_id=$(tofu -chdir="$root" output -raw api_id)
alarm_names=$(tofu -chdir="$root" output -json alarm_names)
printf '%s\n' "$alarm_names" | jq -e 'type == "array" and length == 5 and all(.[]; type == "string")' >/dev/null || fail "OpenTofu did not return exactly five alarm names"

live_version=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" lambda get-alias \
	--function-name "$function_name" --name live --query FunctionVersion --output text)
live_image_uri=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" lambda get-function \
	--function-name "$function_name" --qualifier "$live_version" --query Code.ImageUri --output text)
live_digest=${live_image_uri##*@}
test "$live_image_uri" = "$repository_url@$expected_digest" || fail "live image URI does not match the release record"
test "$live_digest" = "$expected_digest" || fail "live digest does not match the release record"
test "$live_version" = "$expected_version" || fail "published version does not match the release record"
test "$live_version" = "$expected_alias" || fail "live alias does not match the release record"

health_json=$(curl --fail --show-error --silent "https://$public_host/healthz") || fail "public health probe failed"
health_revision=$(printf '%s\n' "$health_json" | jq -er '.revision | select(test("^[0-9a-f]{40}$"))') || fail "public health revision is invalid"
test "$health_revision" = "$source_sha" || fail "public health revision does not match the release record"

probe() {
	url=$1
	curl --show-error --silent --output /dev/null --write-out '%{http_code}|%{content_type}' "$url"
}

home_probe=$(probe "https://$public_host/")
soccer_probe=$(probe "https://$public_host/soccer")
css_probe=$(probe "https://$public_host/static/css/tailwind.css")
asset_probe=$(probe "https://$public_host/static/images/backgrounds/home-hero.jpg")
health_probe=$(probe "https://$public_host/healthz")
rollback_home_probe=$(probe "$rollback_origin/")
rollback_soccer_probe=$(probe "$rollback_origin/soccer")
rollback_asset_probe=$(probe "$rollback_origin/static/css/tailwind.css")

set --
while IFS= read -r alarm_name; do
	set -- "$@" "$alarm_name"
done <<EOF
$(printf '%s\n' "$alarm_names" | jq -r '.[]')
EOF
alarms_json=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" cloudwatch describe-alarms \
	--alarm-names "$@" --output json)
alarms=$(printf '%s\n' "$alarms_json" | jq -ce --argjson names "$alarm_names" '
	[.MetricAlarms[] | select(.AlarmName as $name | $names | index($name)) | {name: .AlarmName, state: .StateValue}] | sort_by(.name) |
	select(length == 5 and ([.[].name] | sort) == ($names | sort))
') || fail "AWS did not return the exact five live alarms"

if [ "$ENVIRONMENT" = production ]; then
	alarm_delivery=$(printf '%s\n' "$environment_record" | jq -er '.alarm_delivery_evidence | select(type == "string" and length > 0)') || fail "production release has no alarm-delivery evidence"
	test -f "$alarm_delivery" || fail "production alarm-delivery evidence does not exist"
	topic_arn=$(jq -er --argjson cutover "$cutover_epoch" '
		select(
			(keys | sort) == ([
				"account_id",
				"confirmed_subscription_count",
				"environment",
				"message_id",
				"receipt_confirmed_at",
				"receipt_token_sha256",
				"region",
				"schema_version",
				"sent_at",
				"topic_arn"
			] | sort) and
			.schema_version == 1 and
			.environment == "production" and
			.account_id == "180294223248" and
			.region == "us-west-2" and
			.topic_arn == "arn:aws:sns:us-west-2:180294223248:portfolio-lambda-prod-alerts" and
			(.confirmed_subscription_count | type) == "number" and
			.confirmed_subscription_count >= 1 and
			.confirmed_subscription_count == (.confirmed_subscription_count | floor) and
			(.message_id | type == "string" and test("^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}$")) and
			(.receipt_token_sha256 | type == "string" and test("^[0-9a-f]{64}$")) and
			(.sent_at | fromdateiso8601) as $sent |
			(.receipt_confirmed_at | fromdateiso8601) as $confirmed |
			.sent_at == ($sent | todateiso8601) and
			.receipt_confirmed_at == ($confirmed | todateiso8601) and
			$confirmed >= $sent and
			($confirmed - $sent) <= 300 and
			$confirmed <= $cutover and
			$confirmed <= now
		) |
		.topic_arn
	' "$alarm_delivery") || fail "production alarm-delivery evidence is invalid"
	printf '%s\n' "$alarms_json" | jq -e --arg topic "$topic_arn" '
		(.MetricAlarms | length) == 5 and all(.MetricAlarms[]; .AlarmActions == [$topic])
	' >/dev/null || fail "the production topic is not the sole action on all five alarms"
fi

observed_epoch=$(jq -nr 'now | floor')
observed_at=$(jq -nr --argjson epoch "$observed_epoch" '$epoch | todateiso8601')
start_epoch=$((observed_epoch - 300))
start_time=$(jq -nr --argjson epoch "$start_epoch" '$epoch | todateiso8601')

metric_sum() {
	namespace=$1
	metric_name=$2
	dimension=$3
	aws --profile "$AWS_PROFILE" --region "$AWS_REGION" cloudwatch get-metric-statistics \
		--namespace "$namespace" --metric-name "$metric_name" --dimensions "$dimension" \
		--start-time "$start_time" --end-time "$observed_at" --period 300 --statistics Sum --output json |
		jq -c '[.Datapoints[]?] | sort_by(.Timestamp) | last | if . == null then null else .Sum end'
}

metric_p95() {
	namespace=$1
	metric_name=$2
	dimension=$3
	aws --profile "$AWS_PROFILE" --region "$AWS_REGION" cloudwatch get-metric-statistics \
		--namespace "$namespace" --metric-name "$metric_name" --dimensions "$dimension" \
		--start-time "$start_time" --end-time "$observed_at" --period 300 --extended-statistics p95 --output json |
		jq -c '[.Datapoints[]?] | sort_by(.Timestamp) | last | if . == null then null else .ExtendedStatistics.p95 end'
}

lambda_errors=$(metric_sum AWS/Lambda Errors "Name=FunctionName,Value=$function_name")
lambda_throttles=$(metric_sum AWS/Lambda Throttles "Name=FunctionName,Value=$function_name")
lambda_duration=$(metric_p95 AWS/Lambda Duration "Name=FunctionName,Value=$function_name")
api_5xx=$(metric_sum AWS/ApiGateway 5xx "Name=ApiId,Value=$api_id")
api_latency=$(metric_p95 AWS/ApiGateway Latency "Name=ApiId,Value=$api_id")

sample=$(jq -nc \
	--arg environment "$ENVIRONMENT" \
	--arg observed_at "$observed_at" \
	--argjson observed_epoch "$observed_epoch" \
	--arg host "$public_host" \
	--arg sha "$source_sha" \
	--arg digest "$live_digest" \
	--arg version "$live_version" \
	--arg alias "$live_version" \
	--arg health_probe "$health_probe" \
	--arg health_revision "$health_revision" \
	--arg home_probe "$home_probe" \
	--arg soccer_probe "$soccer_probe" \
	--arg css_probe "$css_probe" \
	--arg asset_probe "$asset_probe" \
	--arg rollback_origin "$rollback_origin" \
	--arg rollback_home_probe "$rollback_home_probe" \
	--arg rollback_soccer_probe "$rollback_soccer_probe" \
	--arg rollback_asset_probe "$rollback_asset_probe" \
	--argjson alarms "$alarms" \
	--argjson lambda_errors "$lambda_errors" \
	--argjson lambda_throttles "$lambda_throttles" \
	--argjson lambda_duration "$lambda_duration" \
	--argjson api_5xx "$api_5xx" \
	--argjson api_latency "$api_latency" '
	def parsed($probe): ($probe | split("|")) as $parts | {status: ($parts[0] | tonumber), content_type: $parts[1]};
	def route($path; $probe): parsed($probe) + {path: $path};
	def good($probe; $content_type): parsed($probe) | .status == 200 and (.content_type | startswith($content_type));
	[
		(if good($health_probe; "application/json") then empty else "health probe failed" end),
		(if good($home_probe; "text/html") then empty else "home route failed" end),
		(if good($soccer_probe; "text/html") then empty else "soccer route failed" end),
		(if good($css_probe; "text/css") then empty else "CSS asset failed" end),
		(if good($asset_probe; "image/") then empty else "binary asset failed" end),
		(if good($rollback_home_probe; "text/html") and good($rollback_soccer_probe; "text/html") and good($rollback_asset_probe; "text/css") then empty else "rollback origin failed" end),
		(if all($alarms[]; .state != "ALARM") then empty else "alarm in ALARM" end),
		(if $lambda_errors == null or $lambda_throttles == null or $lambda_duration == null or $api_5xx == null or $api_latency == null then "missing metric datapoint" else empty end),
		(if $lambda_errors != null and $lambda_errors > 0 then "Lambda errors observed" else empty end),
		(if $lambda_throttles != null and $lambda_throttles > 0 then "Lambda throttles observed" else empty end),
		(if $lambda_duration != null and $lambda_duration >= 24000 then "Lambda duration threshold reached" else empty end),
		(if $api_5xx != null and $api_5xx > 0 then "API 5xx observed" else empty end),
		(if $api_latency != null and $api_latency >= 25000 then "API latency threshold reached" else empty end)
	] as $blockers |
	{
		schema_version: 1,
		kind: "sample",
		environment: $environment,
		observed_at: $observed_at,
		observed_at_epoch: $observed_epoch,
		public_hostname: $host,
		passed: ($blockers | length == 0),
		source_sha: $sha,
		image_digest: $digest,
		published_version: $version,
		alias_target: $alias,
		health: (parsed($health_probe) + {revision: $health_revision}),
		routes: [route("/"; $home_probe), route("/soccer"; $soccer_probe), route("/static/css/tailwind.css"; $css_probe), route("/static/images/backgrounds/home-hero.jpg"; $asset_probe)],
		alarms: $alarms,
		metrics: {lambda_errors: $lambda_errors, lambda_throttles: $lambda_throttles, lambda_duration_p95_ms: $lambda_duration, api_5xx: $api_5xx, api_latency_p95_ms: $api_latency},
		rollback_origin: {url: $rollback_origin, passed: (good($rollback_home_probe; "text/html") and good($rollback_soccer_probe; "text/html") and good($rollback_asset_probe; "text/css")), probes: [route("/"; $rollback_home_probe), route("/soccer"; $rollback_soccer_probe), route("/static/css/tailwind.css"; $rollback_asset_probe)]},
		unresolved_blockers: $blockers
	}
')

printf '%s\n' "$sample" >>"$EVIDENCE_FILE"
printf '%s\n' "$sample" | jq -e '.passed == true' >/dev/null || fail "sample recorded unresolved blockers"
printf 'Lambda %s observation sample recorded\n' "$ENVIRONMENT"
