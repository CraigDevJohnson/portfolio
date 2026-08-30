#!/bin/sh
set -eu

fail() {
	printf 'CI role plan contract failed: %s\n' "$1" >&2
	exit 1
}

: "${PLAN_JSON:?set PLAN_JSON to the absolute saved-plan JSON path}"

case "$PLAN_JSON" in
	/*) ;;
	*) fail "PLAN_JSON must be absolute" ;;
esac
test -f "$PLAN_JSON" || fail "PLAN_JSON does not exist"

# Reject malformed, failed, destructive, imported, moved, or sensitive plans
# before interpreting any provider-specific values.
jq -e '
	(.format_version | type == "string" and test("^1\\.[0-9]+$")) and
	(.errored == false) and
	(.resource_changes | type == "array") and
	((.output_changes // {}) | type == "object") and
	(.configuration.root_module | type == "object") and
	all(.resource_changes[];
		(.address | type == "string") and
		(.type | type == "string") and
		(.mode == "managed" or .mode == "data") and
		(.change | type == "object") and
		(.change.actions | type == "array") and
		((.change.actions | index("delete")) == null) and
		((.change.actions | index("forget")) == null) and
		(.previous_address? == null) and
		(.deposed? == null) and
		(.change.importing? == null) and
		(.change.generated_config? == null)
	) and
	([
		(.resource_changes[].change, .output_changes[]?)
		| [(.before_sensitive // false), (.after_sensitive // false)][]
		| ..
		| select(type == "boolean" and .)
	] | length) == 0
' "$PLAN_JSON" >/dev/null || fail "plan JSON is invalid, errored, destructive, moved, imported, or sensitive"

# The state owns exactly three roles and their three inline policies. Data
# sources may be absent from resource_changes after OpenTofu reads them during
# planning, but no other data source is permitted. Checking the unexpanded
# configuration also catches dormant resources whose count or for_each is empty.
jq -e '
	def expected_managed: [
		{address: "aws_iam_role.ci[\"dev\"]", type: "aws_iam_role"},
		{address: "aws_iam_role.ci[\"prod\"]", type: "aws_iam_role"},
		{address: "aws_iam_role.ci[\"release\"]", type: "aws_iam_role"},
		{address: "aws_iam_role_policy.environment[\"dev\"]", type: "aws_iam_role_policy"},
		{address: "aws_iam_role_policy.environment[\"prod\"]", type: "aws_iam_role_policy"},
		{address: "aws_iam_role_policy.release", type: "aws_iam_role_policy"}
	];
	def expected_configuration: [
		{address: "aws_iam_role.ci", mode: "managed", type: "aws_iam_role"},
		{address: "aws_iam_role_policy.environment", mode: "managed", type: "aws_iam_role_policy"},
		{address: "aws_iam_role_policy.release", mode: "managed", type: "aws_iam_role_policy"},
		{address: "data.aws_iam_policy_document.environment_trust", mode: "data", type: "aws_iam_policy_document"},
		{address: "data.aws_iam_policy_document.release_trust", mode: "data", type: "aws_iam_policy_document"}
	];
	def allowed_data($address; $type):
		$type == "aws_iam_policy_document" and
		([
			"data.aws_iam_policy_document.environment_trust[\"dev\"]",
			"data.aws_iam_policy_document.environment_trust[\"prod\"]",
			"data.aws_iam_policy_document.release_trust"
		] | index($address) != null);
	def configuration_contract($address; $expected_keys):
		first(.configuration.root_module.resources[] | select(.address == $address)) as $resource |
		($resource | type) == "object" and
		($resource | keys) == ($expected_keys | sort) and
		$resource.provider_config_key == "aws" and
		$resource.schema_version == 0;

	([.resource_changes[] | select(.mode == "managed") | {address, type}] | sort_by(.address)) ==
		(expected_managed | sort_by(.address)) and
	all(.resource_changes[]; .provider_name == "registry.opentofu.org/hashicorp/aws") and
	all(.resource_changes[] | select(.mode == "managed");
		(.change.after | type == "object") and
		(.change.actions == ["create"] or .change.actions == ["update"] or .change.actions == ["no-op"])) and
	all(.resource_changes[] | select(.mode == "data");
		allowed_data(.address; .type) and
		(.change.actions == ["read"] or .change.actions == ["no-op"])) and
	([.resource_changes[] | select(.mode == "data") | .address] as $addresses |
		($addresses | length) == ($addresses | unique | length)) and
	([.configuration.root_module.resources[]? | {address, mode, type}] | sort_by(.address)) ==
		(expected_configuration | sort_by(.address)) and
	((.output_changes // {}) | length) == 0 and
	(.configuration | keys) == ["provider_config", "root_module"] and
	(.configuration.root_module | keys) == ["resources"] and
	.configuration.provider_config == {
		aws: {
			name: "aws",
			full_name: "registry.opentofu.org/hashicorp/aws",
			version_constraint: "6.38.0",
			expressions: {
				allowed_account_ids: {constant_value: ["180294223248"]},
				profile: {constant_value: "portfolio-ci-roles-administrator"},
				region: {references: ["local.region"]}
			}
		}
	} and
	configuration_contract("aws_iam_role.ci"; [
		"address", "expressions", "for_each_expression", "mode", "name",
		"provider_config_key", "schema_version", "type"
	]) and
	configuration_contract("aws_iam_role_policy.environment"; [
		"address", "depends_on", "expressions", "for_each_expression", "mode", "name",
		"provider_config_key", "schema_version", "type"
	]) and
	configuration_contract("aws_iam_role_policy.release"; [
		"address", "depends_on", "expressions", "mode", "name",
		"provider_config_key", "schema_version", "type"
	]) and
	configuration_contract("data.aws_iam_policy_document.environment_trust"; [
		"address", "expressions", "for_each_expression", "mode", "name",
		"provider_config_key", "schema_version", "type"
	]) and
	configuration_contract("data.aws_iam_policy_document.release_trust"; [
		"address", "expressions", "mode", "name", "provider_config_key", "schema_version", "type"
	])
' "$PLAN_JSON" >/dev/null || fail "plan contains an unexpected or missing CI role resource"

# Compare policy documents semantically. AWS may collapse single-element
# arrays or reorder set-like fields, so normalize those representations while
# preserving every unrecognized key; extra authority therefore still fails the
# final object equality.
jq -e '
	def values_array: if type == "array" then . else [.] end;
	def canonical_statement:
		with_entries(
			if (.key == "Action" or .key == "NotAction" or .key == "Resource" or .key == "NotResource") then
				.value |= (values_array | sort)
			elif (.key == "Principal" or .key == "NotPrincipal") and (.value | type) == "object" then
				.value |= with_entries(.value |= (values_array | sort))
			elif .key == "Condition" and (.value | type) == "object" then
				.value |= with_entries(
					if (.value | type) == "object" then
						.value |= with_entries(.value |= (values_array | sort))
					else
						.
					end
				)
			else
				.
			end
		);
	def canonical_policy:
		(if type == "string" then try fromjson catch null
		 elif type == "object" then .
		 else null
		 end) as $policy |
		if ($policy | type) != "object" or (($policy | has("Statement")) | not) then
			null
		else
			$policy | .Statement |= (
				values_array |
				map(canonical_statement) |
				sort_by([.Sid // "", .Effect // "", .Action // [], .Resource // [], .Principal // {}, .Condition // {}])
			)
		end;
	def policy_matches($actual; $expected):
		($actual | type) == "string" and
		($actual | canonical_policy) == ($expected | canonical_policy);
	def allow($sid; $actions; $resources; $condition):
		{Effect: "Allow", Action: $actions, Resource: $resources} |
		if $sid != null then . + {Sid: $sid} else . end |
		if $condition != null then . + {Condition: $condition} else . end;

	def expected_trust($subject): {
		Version: "2012-10-17",
		Statement: [{
			Effect: "Allow",
			Action: ["sts:AssumeRoleWithWebIdentity"],
			Principal: {
				Federated: ["arn:aws:iam::180294223248:oidc-provider/token.actions.githubusercontent.com"]
			},
			Condition: {
				StringEquals: {
					"token.actions.githubusercontent.com:aud": ["sts.amazonaws.com"],
					"token.actions.githubusercontent.com:sub": [$subject]
				}
			}
		}]
	};
	def expected_release_policy: {
		Version: "2012-10-17",
		Statement: [
			allow(null;
				["ecr:GetAuthorizationToken"];
				["*"];
				{StringEquals: {"aws:RequestedRegion": ["us-west-2"]}}),
			allow(null;
				[
					"ecr:BatchCheckLayerAvailability",
					"ecr:CompleteLayerUpload",
					"ecr:DescribeImageScanFindings",
					"ecr:DescribeImages",
					"ecr:DescribeRepositories",
					"ecr:GetDownloadUrlForLayer",
					"ecr:InitiateLayerUpload",
					"ecr:ListImages",
					"ecr:PutImage",
					"ecr:UploadLayerPart"
				];
				["arn:aws:ecr:us-west-2:180294223248:repository/portfolio-lambda-releases"];
				null)
		]
	};
	def expected_environment_policy($environment):
		(if $environment == "dev" then "portfolio-lambda-dev" else "portfolio-lambda-prod" end) as $function |
		("portfolio-lambda-http-api/" + $environment + "/terraform.tfstate") as $state_key |
		"arn:aws:s3:::portfolio-tofu-state-180294223248" as $state_bucket |
		{
			Version: "2012-10-17",
			Statement: ([
				allow("CallerIdentity";
					["sts:GetCallerIdentity"]; ["*"]; null),
				allow("StateBucketMetadata";
					["s3:GetBucketLocation", "s3:GetBucketVersioning"]; [$state_bucket]; null),
				allow("StatePrefix";
					["s3:ListBucket"]; [$state_bucket];
					{StringLike: {"s3:prefix": [$state_key + "*"]}}),
				allow("StateRead";
					["s3:GetObject"]; [$state_bucket + "/" + $state_key]; null),
				allow("StateLock";
					["s3:GetObject", "s3:PutObject", "s3:DeleteObject"];
					[$state_bucket + "/" + $state_key + ".tflock"]; null),
				allow("ReleaseImageRead";
					["ecr:BatchGetImage", "ecr:DescribeImages", "ecr:GetDownloadUrlForLayer"];
					["arn:aws:ecr:us-west-2:180294223248:repository/portfolio-lambda-releases"]; null),
				allow("ExecutionRoleRead";
					["iam:GetRole", "iam:GetRolePolicy", "iam:ListAttachedRolePolicies", "iam:ListRolePolicies", "iam:ListRoleTags"];
					["arn:aws:iam::180294223248:role/" + $function + "-execution"]; null),
				allow("LambdaRead";
					[
						"lambda:GetAlias",
						"lambda:GetFunction",
						"lambda:GetFunctionCodeSigningConfig",
						"lambda:GetFunctionConcurrency",
						"lambda:GetFunctionConfiguration",
						"lambda:GetPolicy",
						"lambda:GetRuntimeManagementConfig",
						"lambda:ListTags",
						"lambda:ListVersionsByFunction"
					];
					[
						"arn:aws:lambda:us-west-2:180294223248:function:" + $function,
						"arn:aws:lambda:us-west-2:180294223248:function:" + $function + ":*"
					]; null),
				allow("ApiGatewayRead";
					["apigateway:GET"];
					["arn:aws:apigateway:us-west-2::/apis*", "arn:aws:apigateway:us-west-2::/domainnames*"]; null),
				allow("LogGroupRead";
					["logs:ListTagsForResource"];
					[
						"arn:aws:logs:us-west-2:180294223248:log-group:/aws/apigateway/" + $function + "/access",
						"arn:aws:logs:us-west-2:180294223248:log-group:/aws/apigateway/" + $function + "/access:*",
						"arn:aws:logs:us-west-2:180294223248:log-group:/aws/lambda/" + $function,
						"arn:aws:logs:us-west-2:180294223248:log-group:/aws/lambda/" + $function + ":*"
					]; null),
				allow("LogGroupList";
					["logs:DescribeLogGroups"]; ["*"];
					{StringEquals: {"aws:RequestedRegion": ["us-west-2"]}}),
				allow("TableRead";
					["dynamodb:DescribeContinuousBackups", "dynamodb:DescribeTable", "dynamodb:DescribeTimeToLive", "dynamodb:ListTagsOfResource"];
					[
						"arn:aws:dynamodb:us-west-2:180294223248:table/" + $function + "-google-connections",
						"arn:aws:dynamodb:us-west-2:180294223248:table/" + $function + "-soccer-sessions"
					]; null),
				allow("KmsAliasList";
					["kms:ListAliases"]; ["*"];
					{StringEquals: {"aws:RequestedRegion": ["us-west-2"]}}),
				allow("KmsSsmKeyRead";
					["kms:DescribeKey"]; ["arn:aws:kms:us-west-2:180294223248:key/*"];
					{"ForAnyValue:StringEquals": {"kms:ResourceAliases": ["alias/aws/ssm"]}}),
				allow("CertificateRead";
					["acm:DescribeCertificate", "acm:ListTagsForCertificate"];
					["arn:aws:acm:us-west-2:180294223248:certificate/*"];
					{StringEquals: {
						"aws:RequestedRegion": ["us-west-2"],
						"aws:ResourceTag/Environment": [$environment],
						"aws:ResourceTag/ManagedBy": ["opentofu"],
						"aws:ResourceTag/Platform": ["lambda-http-api"],
						"aws:ResourceTag/Project": ["portfolio"]
					}}),
				allow("AlarmRead";
					["cloudwatch:DescribeAlarms", "cloudwatch:ListTagsForResource"];
					(["api-5xx", "api-latency", "lambda-duration", "lambda-errors", "lambda-throttles"] |
						map("arn:aws:cloudwatch:us-west-2:180294223248:alarm:" + $function + "-" + .)); null)
			] + if $environment == "dev" then [
				allow("DevelopmentStateWrite";
					["s3:PutObject", "s3:DeleteObject"];
					[$state_bucket + "/" + $state_key]; null),
				allow("DevelopmentReleaseWrite";
					["lambda:PublishVersion", "lambda:UpdateAlias", "lambda:UpdateFunctionCode"];
					[
						"arn:aws:lambda:us-west-2:180294223248:function:" + $function,
						"arn:aws:lambda:us-west-2:180294223248:function:" + $function + ":live"
					];
					{StringEquals: {
						"aws:ResourceTag/Environment": ["dev"],
						"aws:ResourceTag/ManagedBy": ["opentofu"],
						"aws:ResourceTag/Platform": ["lambda-http-api"],
						"aws:ResourceTag/Project": ["portfolio"]
					}})
			] else [] end)
		};

	def by_address($address):
		first(.resource_changes[] | select(.mode == "managed" and .address == $address));
	def configuration_by_address($address):
		first(.configuration.root_module.resources[] | select(.address == $address));
	def true_paths($value):
		[($value // {}) | paths(scalars) as $path | select(getpath($path) == true) | $path];
	def only_unknown_attributes($change; $allowed):
		all(true_paths($change.after_unknown)[]; .[0] as $attribute | $allowed | index($attribute) != null);
	def exact_keys_or_fewer($value; $allowed):
		($value | type) == "object" and (($value | keys) - $allowed | length) == 0;
	def exact_references($expression; $expected):
		(($expression.references // []) | sort) == ($expected | sort);
	def inline_policy_contract($value; $name; $expected):
		($value // []) as $policies |
		($policies | type) == "array" and
		(($policies | length) == 0 or
		 (($policies | length) == 1 and
		  exact_keys_or_fewer($policies[0]; ["name", "policy"]) and
		  $policies[0].name == $name and
		  policy_matches($policies[0].policy; $expected)));
	def role_contract($address; $name; $subject; $inline_name; $inline_policy):
		by_address($address) as $resource |
		$resource.change as $change |
		$change.after as $after |
		($change.actions == ["create"]) as $creating |
		exact_keys_or_fewer($after; [
			"arn", "assume_role_policy", "create_date", "description", "force_detach_policies", "id",
			"inline_policy", "managed_policy_arns", "max_session_duration", "name", "name_prefix", "path",
			"permissions_boundary", "tags", "tags_all", "unique_id"
		]) and
		$after.name == $name and
		$after.description == null and
		$after.force_detach_policies == false and
		$after.max_session_duration == 3600 and
		$after.path == "/" and
		$after.permissions_boundary == null and
		$after.tags == {ManagedBy: "opentofu", Project: "portfolio", Purpose: "github-release"} and
		$after.tags_all == $after.tags and
		policy_matches($after.assume_role_policy; expected_trust($subject)) and
		inline_policy_contract($after.inline_policy; $inline_name; $inline_policy) and
		($after.managed_policy_arns // []) == [] and
		only_unknown_attributes($change;
			["arn", "create_date", "id", "name_prefix", "unique_id"] +
			(if $creating then ["inline_policy", "managed_policy_arns"] else [] end));
	def policy_target_contract($change; $role):
		$change.after.role == $role and (($change.after_unknown.role // false) == false);
	def policy_contract($address; $name; $role; $expected):
		by_address($address) as $resource |
		$resource.change as $change |
		$change.after as $after |
		exact_keys_or_fewer($after; ["id", "name", "name_prefix", "policy", "role"]) and
		$after.name == $name and
		policy_matches($after.policy; $expected) and
		policy_target_contract($change; $role) and
		only_unknown_attributes($change; ["id", "name_prefix"]);

	role_contract(
		"aws_iam_role.ci[\"release\"]";
		"portfolio-release-builder-ci";
		"repo:CraigDevJohnson/portfolio:ref:refs/heads/main";
		"portfolio-release-builder";
		expected_release_policy
	) and
	role_contract(
		"aws_iam_role.ci[\"dev\"]";
		"portfolio-development-deployer-ci";
		"repo:CraigDevJohnson/portfolio:environment:development";
		"portfolio-development-runtime-release";
		expected_environment_policy("dev")
	) and
	role_contract(
		"aws_iam_role.ci[\"prod\"]";
		"portfolio-production-planner-ci";
		"repo:CraigDevJohnson/portfolio:environment:production-plan";
		"portfolio-production-read-only-plan";
		expected_environment_policy("prod")
	) and
	policy_contract(
		"aws_iam_role_policy.release";
		"portfolio-release-builder";
		"portfolio-release-builder-ci";
		expected_release_policy
	) and
	policy_contract(
		"aws_iam_role_policy.environment[\"dev\"]";
		"portfolio-development-runtime-release";
		"portfolio-development-deployer-ci";
		expected_environment_policy("dev")
	) and
	policy_contract(
		"aws_iam_role_policy.environment[\"prod\"]";
		"portfolio-production-read-only-plan";
		"portfolio-production-planner-ci";
		expected_environment_policy("prod")
	) and
	(configuration_by_address("aws_iam_role.ci") as $roles |
		($roles.expressions | keys) == ["assume_role_policy", "max_session_duration", "name", "tags"] and
		exact_references($roles.for_each_expression; ["local.roles"]) and
		exact_references($roles.expressions.name; ["each.value.name", "each.value"]) and
		exact_references($roles.expressions.assume_role_policy; ["each.value.trust", "each.value"])) and
	(configuration_by_address("aws_iam_role_policy.environment") as $environment_policy |
		($environment_policy.expressions | keys) == ["name", "policy", "role"] and
		$environment_policy.depends_on == ["aws_iam_role.ci"] and
		exact_references($environment_policy.for_each_expression; ["local.environment_configuration"]) and
		exact_references($environment_policy.expressions.role; ["each.value.role_name", "each.value"])) and
	(configuration_by_address("aws_iam_role_policy.release") as $release_policy |
		($release_policy.expressions | keys) == ["name", "policy", "role"] and
		$release_policy.depends_on == ["aws_iam_role.ci"] and
		exact_references($release_policy.expressions.role; [
			"local.roles.release.name",
			"local.roles.release",
			"local.roles"
		]))
' "$PLAN_JSON" >/dev/null || fail "CI role names, trust, attachments, or inline policies drifted"

printf 'CI role plan contract passed\n'
