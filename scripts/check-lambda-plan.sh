#!/bin/sh
set -eu

fail() {
	printf 'Lambda plan contract failed: %s\n' "$1" >&2
	exit 1
}

: "${PLAN_JSON:?set PLAN_JSON to the absolute saved-plan JSON path}"
: "${ENVIRONMENT:?set ENVIRONMENT to artifacts, dev, or prod}"
: "${NAME_PREFIX:?set NAME_PREFIX to the exact root-owned name}"
: "${IMAGE_URI:?set IMAGE_URI to the digest-qualified release URI}"
: "${EXPECTED_ALARM_ACTIONS_JSON:?set EXPECTED_ALARM_ACTIONS_JSON to a JSON array}"

case "$PLAN_JSON" in
	/*) ;;
	*) fail "PLAN_JSON must be absolute" ;;
esac
test -f "$PLAN_JSON" || fail "PLAN_JSON does not exist"

case "$ENVIRONMENT:$NAME_PREFIX" in
	artifacts:portfolio-lambda-releases | dev:portfolio-lambda-dev | prod:portfolio-lambda-prod) ;;
	*) fail "environment and name prefix do not match a replacement root" ;;
esac

case "$IMAGE_URI" in
	180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases@sha256:????????????????????????????????????????????????????????????????) ;;
	*) fail "IMAGE_URI must use the replacement repository and a SHA-256 digest" ;;
esac
printf '%s\n' "$IMAGE_URI" | grep -Eq '@sha256:[0-9a-f]{64}$' ||
	fail "IMAGE_URI digest must contain 64 lowercase hexadecimal characters"

printf '%s\n' "$EXPECTED_ALARM_ACTIONS_JSON" |
	jq -e 'type == "array" and all(.[]; type == "string")' >/dev/null ||
	fail "EXPECTED_ALARM_ACTIONS_JSON must be an array of strings"
jq -e '.resource_changes | type == "array"' "$PLAN_JSON" >/dev/null ||
	fail "plan JSON has no resource_changes array"

jq -e '
	all(.resource_changes[]; (.change.actions | index("delete")) == null)
' "$PLAN_JSON" >/dev/null || fail "delete or replace action found"

jq -e '
	[
		.resource_changes[]
		| select(
			(.address | test("aws_apprunner|aws_amplify"; "i")) or
			(.change.after | tostring | test("aws_apprunner|aws_amplify"; "i"))
		)
	] | length == 0
' "$PLAN_JSON" >/dev/null || fail "legacy App Runner or Amplify resource found"

jq -e '
	[
		.resource_changes[].change
		| [.before, .after][]
		| ..
		| strings
		| select(test("(^|[:/])(latest|lambda-latest)$"; "i"))
	] | length == 0
' "$PLAN_JSON" >/dev/null || fail "mutable image tag found"

jq -e --arg environment "$ENVIRONMENT" '
	def allowed_secret_path:
		type == "string" and
		test("^/portfolio/lambda/(dev|prod)/CLIENT_SECRET_KEY$");
	[
		.resource_changes[].change
		| [.before, .after][]
		| ..
		| objects
		| to_entries[]
		| select(.key | test("secret|password|token"; "i"))
		| .value
		| select(. != null and . != false and . != "" and (allowed_secret_path | not))
	] | length == 0
' "$PLAN_JSON" >/dev/null || fail "secret value found in plan"

jq -e '
	[
		.resource_changes[].change
		| [.before_sensitive, .after_sensitive][]
		| ..
		| select(type == "boolean" and .)
	] | length == 0
' "$PLAN_JSON" >/dev/null || fail "sensitive value found in plan"

if [ "$ENVIRONMENT" = artifacts ]; then
	jq -e '
		def by_address($address): first(.resource_changes[] | select(.address == $address));
		([.resource_changes[].address] | sort) == ([
			"aws_ecr_lifecycle_policy.lambda_releases",
			"aws_ecr_repository.lambda_releases",
			"aws_ecr_repository_policy.lambda_releases"
		] | sort) and
		(by_address("aws_ecr_repository.lambda_releases").change.after |
			.name == "portfolio-lambda-releases" and
			.image_tag_mutability == "IMMUTABLE" and
			.force_delete == false and
			.image_scanning_configuration[0].scan_on_push == true and
			.encryption_configuration[0].encryption_type == "AES256") and
		(by_address("aws_ecr_lifecycle_policy.lambda_releases").change.after |
			.repository == "portfolio-lambda-releases" and
			(.policy | fromjson) == {
				rules: [{
					rulePriority: 1,
					description: "Expire untagged images after 30 days",
					selection: {
						tagStatus: "untagged",
						countType: "sinceImagePushed",
						countUnit: "days",
						countNumber: 30
					},
					action: {type: "expire"}
				}]
			}) and
		(by_address("aws_ecr_repository_policy.lambda_releases").change.after |
			.repository == "portfolio-lambda-releases" and
			(.policy | fromjson) as $policy |
			$policy.Version == "2012-10-17" and
			($policy.Statement | length) == 1 and
			$policy.Statement[0].Sid == "LambdaPull" and
			$policy.Statement[0].Effect == "Allow" and
			$policy.Statement[0].Principal == {Service: "lambda.amazonaws.com"} and
			($policy.Statement[0].Action | sort) == (["ecr:BatchGetImage", "ecr:GetDownloadUrlForLayer"] | sort) and
			$policy.Statement[0].Condition == {
				StringEquals: {"aws:SourceAccount": "180294223248"},
				ArnLike: {"aws:SourceArn": "arn:aws:lambda:us-west-2:180294223248:function:portfolio-lambda-*"}
			})
	' "$PLAN_JSON" >/dev/null || fail "artifact root ownership or repository policy drifted"
	printf 'Lambda artifact plan contract passed\n'
	exit 0
fi

expected_boundary=arn:aws:iam::180294223248:policy/portfolio/boundaries/PortfolioLambdaExecutionBoundary
expected_retention=14
expected_protection=false
if [ "$ENVIRONMENT" = prod ]; then
	expected_retention=90
	expected_protection=true
fi

jq -e \
	--arg prefix "$NAME_PREFIX" \
	--arg image "$IMAGE_URI" \
	--arg boundary "$expected_boundary" \
	--argjson retention "$expected_retention" \
	--argjson protection "$expected_protection" \
	--argjson expected_actions "$EXPECTED_ALARM_ACTIONS_JSON" '
	def resources($type): [.resource_changes[] | select(.type == $type)];
	def after($type; $name): first(.resource_changes[] | select(.type == $type and .name == $name) | .change.after);
	def exact_alarm($name; $metric; $threshold; $statistic):
		after("aws_cloudwatch_metric_alarm"; $name) as $alarm |
		$alarm.alarm_name == ($prefix + "-" + ($name | gsub("_"; "-"))) and
		$alarm.metric_name == $metric and
		$alarm.period == 300 and
		$alarm.evaluation_periods == 1 and
		$alarm.threshold == $threshold and
		$alarm.treat_missing_data == "notBreaching" and
		($alarm.alarm_actions | sort) == ($expected_actions | sort) and
		(if $statistic == "p95" then $alarm.extended_statistic == "p95" else $alarm.statistic == $statistic end);

	(resources("aws_iam_role") | length) == 1 and
	all(resources("aws_iam_role")[]; .change.after.name == ($prefix + "-execution") and .change.after.permissions_boundary == $boundary) and
	(resources("aws_iam_role_policy") | length) == 1 and
	all(resources("aws_iam_role_policy")[]; .change.after.name == ($prefix + "-runtime")) and
	(resources("aws_lambda_function") | length) == 1 and
	all(resources("aws_lambda_function")[]; .change.after.function_name == $prefix and .change.after.image_uri == $image) and
	(resources("aws_apigatewayv2_api") | length) == 1 and
	all(resources("aws_apigatewayv2_api")[]; .change.after.name == ($prefix + "-http")) and
	(resources("aws_cloudwatch_log_group") | length) == 2 and
	all(resources("aws_cloudwatch_log_group")[];
		.change.after.retention_in_days == $retention and
		(.change.after.name == ("/aws/lambda/" + $prefix) or .change.after.name == ("/aws/apigateway/" + $prefix + "/access"))) and
	(resources("aws_dynamodb_table") | length) == 2 and
	all(resources("aws_dynamodb_table")[];
		.change.after.deletion_protection_enabled == $protection and
		.change.after.point_in_time_recovery[0].enabled == $protection and
		(.change.after.name == ($prefix + "-google-connections") or .change.after.name == ($prefix + "-soccer-sessions"))) and
	(resources("aws_cloudwatch_metric_alarm") | length) == 5 and
	exact_alarm("lambda_errors"; "Errors"; 1; "Sum") and
	exact_alarm("lambda_throttles"; "Throttles"; 1; "Sum") and
	exact_alarm("lambda_duration"; "Duration"; 24000; "p95") and
	exact_alarm("api_5xx"; "5xx"; 1; "Sum") and
	exact_alarm("api_latency"; "Latency"; 25000; "p95")
' "$PLAN_JSON" >/dev/null || fail "environment boundary, names, protection, retention, image, or alarm contract drifted"

printf 'Lambda %s plan contract passed\n' "$ENVIRONMENT"
