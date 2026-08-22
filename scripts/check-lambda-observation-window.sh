#!/bin/sh
set -eu

fail() {
	printf 'Lambda observation gate failed: %s\n' "$1" >&2
	exit 1
}

: "${RELEASE_RECORD:?set RELEASE_RECORD explicitly}"
: "${EVIDENCE_FILE:?set EVIDENCE_FILE explicitly}"
: "${ENVIRONMENT:?set ENVIRONMENT to development or production}"

test -f "$RELEASE_RECORD" || fail "release record does not exist"
test -f "$EVIDENCE_FILE" || fail "evidence file does not exist"
case "$ENVIRONMENT" in
	development | production) ;;
	*) fail "unsupported environment" ;;
esac

environment_record=$(jq -cer --arg environment "$ENVIRONMENT" '.[$environment]' "$RELEASE_RECORD") ||
	fail "release record has no environment coordinates"
rollback_evidence=$(printf '%s\n' "$environment_record" | jq -er '.rollback_evidence | select(type == "string" and length > 0)') ||
	fail "release record has no rollback evidence path"
test -f "$rollback_evidence" || fail "rollback evidence does not exist"

jq -e --arg environment "$ENVIRONMENT" '
	.schema_version == 1 and
	.environment == $environment and
	(.public_hostname | type == "string" and length > 0) and
	(.rollback_origin_url | type == "string" and test("^https://")) and
	(.dns_record.id | type == "string" and length > 0) and
	(.dns_record.type | type == "string" and length > 0) and
	(.dns_record.name | type == "string" and length > 0) and
	(.dns_record.content | type == "string" and length > 0) and
	(.dns_record.ttl | type == "number") and
	(.dns_record.proxied | type == "boolean")
' "$rollback_evidence" >/dev/null || fail "rollback evidence is incomplete"

if [ "$ENVIRONMENT" = production ]; then
	alarm_delivery=$(printf '%s\n' "$environment_record" | jq -er '.alarm_delivery_evidence | select(type == "string" and length > 0)') ||
		fail "production release has no alarm-delivery evidence path"
	test -f "$alarm_delivery" || fail "production alarm-delivery evidence does not exist"
	jq -e '
		.schema_version == 1 and
		.environment == "production" and
		.account_id == "180294223248" and
		.region == "us-west-2" and
		.topic_arn == "arn:aws:sns:us-west-2:180294223248:portfolio-lambda-prod-alerts" and
		.delivery_verified == true
	' "$alarm_delivery" >/dev/null || fail "production alarm-delivery evidence is invalid"
fi

cutover_epoch=$(printf '%s\n' "$environment_record" | jq -er '.dns_cutover_at | fromdateiso8601') ||
	fail "DNS cutover timestamp is invalid"
release_sha=$(jq -er '.source_sha | select(test("^[0-9a-f]{40}$"))' "$RELEASE_RECORD") ||
	fail "source SHA is invalid"
release_digest=$(jq -er '.image.digest | select(test("^sha256:[0-9a-f]{64}$"))' "$RELEASE_RECORD") ||
	fail "image digest is invalid"
release_version=$(printf '%s\n' "$environment_record" | jq -er '.published_version | tostring | select(length > 0)') ||
	fail "published version is invalid"
release_alias=$(printf '%s\n' "$environment_record" | jq -er '.live_alias_target | tostring | select(length > 0)') ||
	fail "alias target is invalid"
test "$release_version" = "$release_alias" || fail "release version and alias target differ"
public_host=$(printf '%s\n' "$environment_record" | jq -er '.custom_domains[0] | select(type == "string" and length > 0)') ||
	fail "public hostname is invalid"
name_prefix=portfolio-lambda-dev
[ "$ENVIRONMENT" != production ] || name_prefix=portfolio-lambda-prod

jq -s -e \
	--arg environment "$ENVIRONMENT" \
	--arg host "$public_host" \
	--arg prefix "$name_prefix" \
	--arg sha "$release_sha" \
	--arg digest "$release_digest" \
	--arg version "$release_version" \
	--arg alias "$release_alias" \
	--argjson cutover "$cutover_epoch" '
	def samples: [.[] | select(.kind == "sample" and .environment == $environment)] | sort_by(.observed_at_epoch);
	def workflows: [.[] | select(.kind == "workflow" and .environment == $environment)] | sort_by(.observed_at_epoch);
	def gaps_ok($items):
		[range(1; $items | length) | ($items[.].observed_at_epoch - $items[. - 1].observed_at_epoch)] | all(. <= 93600);
	def route_ok($sample; $path; $content_type):
		[$sample.routes[] | select(.path == $path and .status == 200 and (.content_type | startswith($content_type)))] | length == 1;
	def alarm_names: [
		$prefix + "-api-5xx",
		$prefix + "-api-latency",
		$prefix + "-lambda-duration",
		$prefix + "-lambda-errors",
		$prefix + "-lambda-throttles"
	] | sort;
	samples as $samples |
	workflows as $workflows |
	($samples | length) >= 8 and
	($samples[-1].observed_at_epoch - $cutover) >= 604800 and
	gaps_ok($samples) and
	all($samples[];
		.schema_version == 1 and
		.passed == true and
		.observed_at_epoch >= $cutover and
		.public_hostname == $host and
		.source_sha == $sha and
		.image_digest == $digest and
		(.published_version | tostring) == $version and
		(.alias_target | tostring) == $alias and
		.health.status == 200 and
		(.health.content_type | startswith("application/json")) and
		.health.revision == $sha and
		(.routes | type == "array" and length == 4) and
		route_ok(.; "/"; "text/html") and
		route_ok(.; "/soccer"; "text/html") and
		route_ok(.; "/static/css/tailwind.css"; "text/css") and
		route_ok(.; "/static/images/backgrounds/home-hero.jpg"; "image/") and
		(.unresolved_blockers | type == "array" and length == 0) and
		(.alarms | type == "array" and length == 5) and
		([.alarms[].name] | sort) == alarm_names and
		all(.alarms[]; .state == "OK" or .state == "INSUFFICIENT_DATA") and
		(.metrics | [.lambda_errors, .lambda_throttles, .lambda_duration_p95_ms, .api_5xx, .api_latency_p95_ms] | all(.[]; type == "number")) and
		.rollback_origin.passed == true and
		(.rollback_origin.probes | type == "array" and length == 3) and
		all(.rollback_origin.probes[]; .status == 200)) and
	($workflows | length) >= 2 and
	($workflows[0].observed_at_epoch >= $cutover) and
	($workflows[0].observed_at_epoch - $cutover <= 93600) and
	($workflows[-1].observed_at_epoch - $cutover >= 604800) and
	all($workflows[];
		.schema_version == 1 and
		.passed == true and
		.public_hostname == $host and
		.source_sha == $sha and
		.image_digest == $digest and
		(.published_version | tostring) == $version and
		(.alias_target | tostring) == $alias and
		.oauth_ok == true and
		.secure_cookies_ok == true and
		.add_ok == true and
		.sync_ok == true and
		(.connect_request_id | type == "string" and length > 0) and
		(.add_request_id | type == "string" and length > 0) and
		(.sync_request_id | type == "string" and length > 0))
' "$EVIDENCE_FILE" >/dev/null || fail "sample or workflow window contract is not satisfied"

printf 'Lambda %s observation window passed\n' "$ENVIRONMENT"
