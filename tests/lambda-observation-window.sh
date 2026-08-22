#!/bin/sh
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
gate="$repo_root/scripts/check-lambda-observation-window.sh"
tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT HUP INT TERM

pass_count=0

pass() {
	pass_count=$((pass_count + 1))
	printf 'PASS: %s\n' "$1"
}

expect_pass() {
	name=$1
	shift
	if "$@" >"$tmp_dir/output" 2>&1; then
		pass "$name"
	else
		printf 'FAIL: %s\n' "$name" >&2
		cat "$tmp_dir/output" >&2
		exit 1
	fi
}

expect_fail() {
	name=$1
	shift
	if "$@" >"$tmp_dir/output" 2>&1; then
		printf 'FAIL: %s unexpectedly passed\n' "$name" >&2
		exit 1
	fi
	pass "$name"
}

sha=0123456789abcdef0123456789abcdef01234567
digest=sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
version=42
cutover=1000
final_epoch=$((cutover + 604800))

make_rollback() {
	environment=$1
	origin=$2
	jq -n \
		--arg environment "$environment" \
		--arg origin "$origin" \
		'{schema_version: 1, environment: $environment, captured_at: "1970-01-01T00:00:00Z", public_hostname: (if $environment == "development" then "dev.craigdevjohnson.com" else "craigdevjohnson.com" end), rollback_origin_url: $origin, dns_record: {id: "record-1", type: "CNAME", name: (if $environment == "development" then "dev.craigdevjohnson.com" else "craigdevjohnson.com" end), content: "legacy.example.com", ttl: 1, proxied: false}}' >"$3"
}

make_alarm_delivery() {
	jq -n '{schema_version: 1, environment: "production", account_id: "180294223248", region: "us-west-2", topic_arn: "arn:aws:sns:us-west-2:180294223248:portfolio-lambda-prod-alerts", confirmed_subscription_count: 1, message_id: "11111111-2222-4333-8444-555555555555", sent_at: "1970-01-01T00:10:00Z", receipt_confirmed_at: "1970-01-01T00:12:00Z", receipt_token_sha256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}' >"$1"
}

make_boolean_only_alarm_delivery() {
	jq -n '{schema_version: 1, environment: "production", account_id: "180294223248", region: "us-west-2", topic_arn: "arn:aws:sns:us-west-2:180294223248:portfolio-lambda-prod-alerts", delivery_verified: true}' >"$1"
}

make_release() {
	environment=$1
	rollback=$2
	alarm_delivery=$3
	output=$4
	jq -n \
		--arg environment "$environment" \
		--arg rollback "$rollback" \
		--arg alarm_delivery "$alarm_delivery" \
		--arg sha "$sha" \
		--arg digest "$digest" \
		--arg version "$version" \
		'{schema_version: 1, source_sha: $sha, image: {repository_name: "portfolio-lambda-releases", repository_url: "180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases", tag: ("git-" + $sha), digest: $digest, uri: ("180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases@" + $digest), scan_status: "COMPLETE"}, development: null, production: null}
		| .[$environment] = {
			function_name: ("portfolio-lambda-" + (if $environment == "development" then "dev" else "prod" end)),
			published_version: $version,
			live_alias_target: $version,
			api_endpoint: "https://api.example.com",
			custom_domains: [(if $environment == "development" then "dev.craigdevjohnson.com" else "craigdevjohnson.com" end)],
			healthz_revision: $sha,
			dns_cutover_at: "1970-01-01T00:16:40Z",
			rollback_evidence: $rollback,
			observation_evidence: "observation.jsonl",
			observation_completed_at: null
		}
		| if $environment == "production" then .production.alarm_delivery_evidence = $alarm_delivery else . end' >"$output"
}

make_evidence() {
	environment=$1
	output=$2
	prefix=portfolio-lambda-dev
	public_host=dev.craigdevjohnson.com
	rollback_origin=https://legacy-app-runner.example.com
	if [ "$environment" = production ]; then
		prefix=portfolio-lambda-prod
		public_host=craigdevjohnson.com
		rollback_origin=https://legacy-amplify.example.com
	fi
	: >"$output"
	i=0
	while [ "$i" -lt 8 ]; do
		epoch=$((cutover + (i * 86400)))
		[ "$i" -eq 7 ] && epoch=$final_epoch
		jq -nc \
			--arg environment "$environment" \
			--arg host "$public_host" \
			--arg prefix "$prefix" \
			--arg rollback_origin "$rollback_origin" \
			--arg sha "$sha" \
			--arg digest "$digest" \
			--arg version "$version" \
			--argjson epoch "$epoch" '
			{
				schema_version: 1,
				kind: "sample",
				environment: $environment,
				observed_at_epoch: $epoch,
				observed_at: ($epoch | todateiso8601),
				public_hostname: $host,
				passed: true,
				source_sha: $sha,
				image_digest: $digest,
				published_version: $version,
				alias_target: $version,
				health: {status: 200, content_type: "application/json", revision: $sha},
				routes: [
					{path: "/", status: 200, content_type: "text/html"},
					{path: "/soccer", status: 200, content_type: "text/html"},
					{path: "/static/css/tailwind.css", status: 200, content_type: "text/css"},
					{path: "/static/images/backgrounds/home-hero.jpg", status: 200, content_type: "image/jpeg"}
				],
				alarms: [
					{name: ($prefix + "-api-5xx"), state: "OK"},
					{name: ($prefix + "-api-latency"), state: "OK"},
					{name: ($prefix + "-lambda-duration"), state: "OK"},
					{name: ($prefix + "-lambda-errors"), state: "OK"},
					{name: ($prefix + "-lambda-throttles"), state: "OK"}
				],
				metrics: {lambda_errors: 0, lambda_throttles: 0, lambda_duration_p95_ms: 100, api_5xx: 0, api_latency_p95_ms: 100},
				rollback_origin: {url: $rollback_origin, passed: true, probes: [{path: "/", status: 200}, {path: "/soccer", status: 200}, {path: "/static/css/tailwind.css", status: 200}]},
				unresolved_blockers: []
			}' >>"$output"
		i=$((i + 1))
	done
	for epoch in "$cutover" "$final_epoch"; do
		jq -nc \
			--arg environment "$environment" \
			--arg host "$public_host" \
			--arg sha "$sha" \
			--arg digest "$digest" \
			--arg version "$version" \
			--argjson epoch "$epoch" '
			{schema_version: 1, kind: "workflow", environment: $environment, observed_at_epoch: $epoch, observed_at: ($epoch | todateiso8601), public_hostname: $host, source_sha: $sha, image_digest: $digest, published_version: $version, alias_target: $version, connect_request_id: ("connect-" + ($epoch | tostring)), add_request_id: ("add-" + ($epoch | tostring)), sync_request_id: ("sync-" + ($epoch | tostring)), oauth_ok: true, secure_cookies_ok: true, add_ok: true, sync_ok: true, passed: true}' >>"$output"
	done
}

run_gate() {
	release=$1
	evidence=$2
	environment=$3
	RELEASE_RECORD="$release" EVIDENCE_FILE="$evidence" ENVIRONMENT="$environment" sh "$gate"
}

dev_rollback="$tmp_dir/development-rollback.json"
prod_rollback="$tmp_dir/production-rollback.json"
alarm_delivery_path="$tmp_dir/alarm-delivery.json"
boolean_alarm_delivery_path="$tmp_dir/boolean-alarm-delivery.json"
dev_release="$tmp_dir/development-release.json"
prod_release="$tmp_dir/production-release.json"
boolean_alarm_prod_release="$tmp_dir/boolean-alarm-production-release.json"
dev_evidence="$tmp_dir/development-observation.jsonl"
prod_evidence="$tmp_dir/production-observation.jsonl"

make_rollback development https://legacy-app-runner.example.com "$dev_rollback"
make_rollback production https://legacy-amplify.example.com "$prod_rollback"
make_alarm_delivery "$alarm_delivery_path"
make_boolean_only_alarm_delivery "$boolean_alarm_delivery_path"
make_release development "$dev_rollback" "" "$dev_release"
make_release production "$prod_rollback" "$alarm_delivery_path" "$prod_release"
make_release production "$prod_rollback" "$boolean_alarm_delivery_path" "$boolean_alarm_prod_release"
make_evidence development "$dev_evidence"
make_evidence production "$prod_evidence"

expect_pass "exact seven-day development window" run_gate "$dev_release" "$dev_evidence" development
expect_fail "boolean-only production alarm evidence" run_gate "$boolean_alarm_prod_release" "$prod_evidence" production
expect_pass "exact seven-day production window" run_gate "$prod_release" "$prod_evidence" production

mutate_evidence_and_reject() {
	name=$1
	filter=$2
	mutated="$tmp_dir/mutated.jsonl"
	jq -c -s "$filter | .[]" "$dev_evidence" >"$mutated"
	expect_fail "$name" run_gate "$dev_release" "$mutated" development
}

mutate_evidence_and_reject "too-short window" 'map(if .kind == "sample" and .observed_at_epoch == 605800 then .observed_at_epoch = 605799 | .observed_at = (605799 | todateiso8601) else . end) | map(if .kind == "workflow" and .observed_at_epoch == 605800 then .observed_at_epoch = 605799 | .observed_at = (605799 | todateiso8601) else . end)'
mutate_evidence_and_reject "missing sample" 'del(.[7])'
mutate_evidence_and_reject "sample gap over 26 hours" 'map(if .kind == "sample" and .observed_at_epoch == 260200 then .observed_at_epoch = 267801 | .observed_at = (267801 | todateiso8601) else . end)'
mutate_evidence_and_reject "first sample more than 26 hours after cutover" 'map(if .kind == "sample" then .observed_at_epoch += 259200 | .observed_at = (.observed_at_epoch | todateiso8601) else . end)'
mutate_evidence_and_reject "future workflow evidence" 'map(if .kind == "workflow" and .observed_at_epoch == 605800 then .observed_at_epoch = (now | floor) + 3600 | .observed_at = (.observed_at_epoch | todateiso8601) else . end)'
mutate_evidence_and_reject "observed timestamp does not match epoch" 'map(if .kind == "sample" and .observed_at_epoch == 173800 then .observed_at = "1970-01-01T00:00:00Z" else . end)'
mutate_evidence_and_reject "cutover and final workflows reuse request IDs" 'map(if .kind == "workflow" and .observed_at_epoch == 605800 then .connect_request_id = "connect-1000" | .add_request_id = "add-1000" | .sync_request_id = "sync-1000" else . end)'
mutate_evidence_and_reject "release coordinate drift" 'map(if .kind == "sample" and .observed_at_epoch == 173800 then .image_digest = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" else . end)'
mutate_evidence_and_reject "alarm state" 'map(if .kind == "sample" and .observed_at_epoch == 346600 then .alarms[0].state = "ALARM" else . end)'
mutate_evidence_and_reject "wrong alarm identity" 'map(if .kind == "sample" and .observed_at_epoch == 346600 then .alarms[0].name = "portfolio-legacy-api-5xx" else . end)'
mutate_evidence_and_reject "failed route hidden behind pass flag" 'map(if .kind == "sample" and .observed_at_epoch == 346600 then .routes[0].status = 500 else . end)'
mutate_evidence_and_reject "missing day-seven workflow" 'map(select(.kind != "workflow" or .observed_at_epoch != 605800))'
mutate_evidence_and_reject "failed workflow proof" 'map(if .kind == "workflow" and .observed_at_epoch == 605800 then .sync_ok = false | .passed = false else . end)'
mutate_evidence_and_reject "rollback-origin failure" 'map(if .kind == "sample" and .observed_at_epoch == 519400 then .rollback_origin.passed = false | .passed = false else . end)'
mutate_evidence_and_reject "unresolved blocker" 'map(if .kind == "sample" and .observed_at_epoch == 433000 then .unresolved_blockers = ["route failure"] | .passed = false else . end)'

bad_alarm_release="$tmp_dir/bad-alarm-release.json"
jq '.production.alarm_delivery_evidence = "/missing/alarm-evidence.json"' "$prod_release" >"$bad_alarm_release"
expect_fail "missing production alarm evidence" run_gate "$bad_alarm_release" "$prod_evidence" production

mutate_alarm_delivery_and_reject() {
	name=$1
	filter=$2
	invalid_alarm="$tmp_dir/invalid-alarm.json"
	invalid_release="$tmp_dir/invalid-alarm-release.json"
	jq "$filter" "$alarm_delivery_path" >"$invalid_alarm"
	make_release production "$prod_rollback" "$invalid_alarm" "$invalid_release"
	expect_fail "$name" run_gate "$invalid_release" "$prod_evidence" production
}

mutate_alarm_delivery_and_reject "zero confirmed alarm subscriptions" '.confirmed_subscription_count = 0'
mutate_alarm_delivery_and_reject "missing alarm message ID" 'del(.message_id)'
mutate_alarm_delivery_and_reject "alarm receipt precedes send" '.receipt_confirmed_at = "1970-01-01T00:09:59Z"'
mutate_alarm_delivery_and_reject "alarm receipt exceeds five minutes" '.receipt_confirmed_at = "1970-01-01T00:15:01Z"'
mutate_alarm_delivery_and_reject "invalid alarm receipt-token hash" '.receipt_token_sha256 = "not-a-sha256"'
mutate_alarm_delivery_and_reject "raw alarm receipt token" '.receipt_token = "secret-value"'
mutate_alarm_delivery_and_reject "alarm subscriber endpoint" '.subscriber_endpoint = "owner@example.com"'

query_rollback="$tmp_dir/query-rollback.json"
query_rollback_release="$tmp_dir/query-rollback-release.json"
make_rollback development 'https://legacy.example.com/?oauth_token=secret-value' "$query_rollback"
make_release development "$query_rollback" "" "$query_rollback_release"
expect_fail "rollback evidence rejects a query string" run_gate "$query_rollback_release" "$dev_evidence" development

fake_bin="$tmp_dir/fake-bin"
mkdir "$fake_bin"
command_log="$tmp_dir/commands.log"
: >"$command_log"

cat >"$fake_bin/tofu" <<'EOF'
#!/bin/sh
set -eu
printf 'tofu %s\n' "$*" >>"$COMMAND_LOG"
case "$*" in
	*"artifacts output -raw ecr_repository_url"*) printf '180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases\n' ;;
	*"output -raw lambda_function_name"*)
		case "$*" in *"/prod"*) printf 'portfolio-lambda-prod\n' ;; *) printf 'portfolio-lambda-dev\n' ;; esac
		;;
	*"output -raw api_id"*) printf 'api-123\n' ;;
	*"output -json alarm_names"*)
		case "$*" in *"/prod"*) prefix=portfolio-lambda-prod ;; *) prefix=portfolio-lambda-dev ;; esac
		printf '["%s-api-5xx","%s-api-latency","%s-lambda-duration","%s-lambda-errors","%s-lambda-throttles"]\n' "$prefix" "$prefix" "$prefix" "$prefix" "$prefix"
		;;
	*) printf 'unexpected fake tofu command: %s\n' "$*" >&2; exit 1 ;;
esac
EOF

cat >"$fake_bin/aws" <<'EOF'
#!/bin/sh
set -eu
printf 'aws %s\n' "$*" >>"$COMMAND_LOG"
case "$*" in
	*"lambda get-alias"*) printf '42\n' ;;
	*"lambda get-function"*) printf '180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n' ;;
	*"cloudwatch describe-alarms"*)
		if [ "${ENVIRONMENT:-development}" = production ]; then prefix=portfolio-lambda-prod; action=arn:aws:sns:us-west-2:180294223248:portfolio-lambda-prod-alerts; else prefix=portfolio-lambda-dev; action=; fi
		[ "${FAKE_WRONG_ALARM_ACTION:-false}" = true ] && action=arn:aws:sns:us-east-1:000000000000:wrong
		jq -nc --arg prefix "$prefix" --arg action "$action" '{MetricAlarms: ["api-5xx", "api-latency", "lambda-duration", "lambda-errors", "lambda-throttles"] | map({AlarmName: ($prefix + "-" + .), StateValue: "OK", AlarmActions: (if $action == "" then [] else [$action] end)})}'
		;;
	*"cloudwatch get-metric-statistics"*)
		case "$*" in
			*"--extended-statistics p95"*) printf '{"Datapoints":[{"Timestamp":"2026-08-22T00:00:00Z","ExtendedStatistics":{"p95":100},"SampleCount":20}]}\n' ;;
			*) printf '{"Datapoints":[{"Timestamp":"2026-08-22T00:00:00Z","Sum":0,"SampleCount":20}]}\n' ;;
		esac
		;;
	*) printf 'unexpected fake aws command: %s\n' "$*" >&2; exit 1 ;;
esac
EOF

cat >"$fake_bin/curl" <<'EOF'
#!/bin/sh
set -eu
printf 'curl %s\n' "$*" >>"$COMMAND_LOG"
url=
for argument in "$@"; do
	case "$argument" in https://*) url=$argument ;; esac
done
case "$*" in
	*"--write-out"*)
		case "$url" in
			*/healthz) content_type=application/json ;;
			*/static/css/*) content_type=text/css ;;
			*.jpg | *.png | *.webp) content_type=image/jpeg ;;
			*) content_type=text/html ;;
		esac
		printf '200|%s\n' "$content_type"
		;;
	*"/healthz"*) printf '{"revision":"0123456789abcdef0123456789abcdef01234567","status":"ok"}\n' ;;
	*) printf 'unexpected fake curl command: %s\n' "$*" >&2; exit 1 ;;
esac
EOF
chmod +x "$fake_bin"/*

record_sample="$repo_root/scripts/record-lambda-observation.sh"
record_workflow="$repo_root/scripts/record-lambda-workflow-proof.sh"
recorded_evidence="$tmp_dir/recorded.jsonl"
: >"$recorded_evidence"

run_sample_recorder() {
	release=$1
	environment=$2
	env PATH="$fake_bin:$PATH" \
		COMMAND_LOG="$command_log" \
		AWS_PROFILE=portfolio-deployer \
		AWS_REGION=us-west-2 \
		ENVIRONMENT="$environment" \
		RELEASE_RECORD="$release" \
		EVIDENCE_FILE="$recorded_evidence" \
		sh "$record_sample"
}

expect_pass "development observation recorder" run_sample_recorder "$dev_release" development
jq -e 'select(.kind == "sample") | (.alarms | length) == 5 and (.routes | length) == 4 and (.metrics | keys | sort) == (["api_5xx", "api_latency_p95_ms", "lambda_duration_p95_ms", "lambda_errors", "lambda_throttles"] | sort) and .rollback_origin.passed == true' "$recorded_evidence" >/dev/null || {
	printf 'FAIL: recorded observation omitted required sanitized fields\n' >&2
	exit 1
}
pass "observation recorder emitted the required sanitized fields"

expect_pass "production observation recorder validates alarm delivery" run_sample_recorder "$prod_release" production
expect_fail "production observation recorder rejects boolean-only alarm evidence" run_sample_recorder "$boolean_alarm_prod_release" production
expect_fail "production observation recorder rejects an alarm-action mismatch" env PATH="$fake_bin:$PATH" COMMAND_LOG="$command_log" FAKE_WRONG_ALARM_ACTION=true AWS_PROFILE=portfolio-deployer AWS_REGION=us-west-2 ENVIRONMENT=production RELEASE_RECORD="$prod_release" EVIDENCE_FILE="$recorded_evidence" sh "$record_sample"

make_bad_rollback_release_and_reject() {
	name=$1
	origin=$2
	bad_rollback="$tmp_dir/bad-rollback.json"
	bad_release="$tmp_dir/bad-rollback-release.json"
	make_rollback development "$origin" "$bad_rollback"
	make_release development "$bad_rollback" "" "$bad_release"
	expect_fail "$name" run_sample_recorder "$bad_release" development
}

make_bad_rollback_release_and_reject "observation recorder rejects rollback credentials" 'https://user:password@legacy.example.com'
make_bad_rollback_release_and_reject "observation recorder rejects rollback query strings" 'https://legacy.example.com/?oauth_token=secret-value'
make_bad_rollback_release_and_reject "observation recorder rejects rollback fragments" 'https://legacy.example.com/#oauth-token'
make_bad_rollback_release_and_reject "observation recorder rejects rollback paths" 'https://legacy.example.com/private'
make_bad_rollback_release_and_reject "observation recorder rejects encoded URL delimiters" 'https://legacy.example.com%2Fprivate'

run_workflow_recorder() {
	env PATH="$fake_bin:$PATH" \
		COMMAND_LOG="$command_log" \
		AWS_PROFILE=portfolio-deployer \
		AWS_REGION=us-west-2 \
		ENVIRONMENT=development \
		RELEASE_RECORD="$dev_release" \
		EVIDENCE_FILE="$recorded_evidence" \
		PUBLIC_HOST=dev.craigdevjohnson.com \
		CONNECT_REQUEST_ID=connect-123 \
		ADD_REQUEST_ID=add-123 \
		SYNC_REQUEST_ID=sync-123 \
		OAUTH_OK=true \
		SECURE_COOKIES_OK=true \
		ADD_OK=true \
		SYNC_OK=true \
		sh "$record_workflow"
}

expect_pass "workflow recorder revalidates release coordinates" run_workflow_recorder
expect_fail "workflow recorder rejects a missing request ID" env PATH="$fake_bin:$PATH" COMMAND_LOG="$command_log" AWS_PROFILE=portfolio-deployer AWS_REGION=us-west-2 ENVIRONMENT=development RELEASE_RECORD="$dev_release" EVIDENCE_FILE="$recorded_evidence" PUBLIC_HOST=dev.craigdevjohnson.com CONNECT_REQUEST_ID= ADD_REQUEST_ID=add-123 SYNC_REQUEST_ID=sync-123 OAUTH_OK=true SECURE_COOKIES_OK=true ADD_OK=true SYNC_OK=true sh "$record_workflow"
expect_fail "workflow recorder rejects a false pass flag" env PATH="$fake_bin:$PATH" COMMAND_LOG="$command_log" AWS_PROFILE=portfolio-deployer AWS_REGION=us-west-2 ENVIRONMENT=development RELEASE_RECORD="$dev_release" EVIDENCE_FILE="$recorded_evidence" PUBLIC_HOST=dev.craigdevjohnson.com CONNECT_REQUEST_ID=connect-123 ADD_REQUEST_ID=add-123 SYNC_REQUEST_ID=sync-123 OAUTH_OK=true SECURE_COOKIES_OK=true ADD_OK=false SYNC_OK=true sh "$record_workflow"

jq -s -e '
	all(.[];
		[.. | objects | keys[]] as $keys |
		all($keys[]; test("^(cookies?|secret|password|body|event_body|query|query_string|oauth_code|token|subscriber_endpoint|calendar_name|jwt)$"; "i") | not))
' "$recorded_evidence" >/dev/null || {
	printf 'FAIL: evidence contains a forbidden field\n' >&2
	exit 1
}
pass "recorded evidence contains no forbidden fields"

printf 'PASS: %s Lambda observation contracts\n' "$pass_count"
