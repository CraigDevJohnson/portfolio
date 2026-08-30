#!/bin/sh
set -eu

account_id=180294223248
region=us-west-2
service_arn=arn:aws:apprunner:us-west-2:180294223248:service/portfolio/c5490e71b0e84aba90a9648e94d240fb
access_role_name=portfolio-apprunner-ecr-access
access_role_id=AROAST6S7QWIFWIJU3SEX
instance_role_name=portfolio-apprunner-instance
instance_role_id=AROAST6S7QWIK7PZV2BTQ
runtime_policy_arn=arn:aws:iam::180294223248:policy/portfolio-apprunner-runtime-secrets

fail() {
	printf 'App Runner retirement preflight failed: %s\n' "$1" >&2
	exit 1
}

test "${AWS_PROFILE:-}" = portfolio-deployer || fail "AWS_PROFILE must be portfolio-deployer"
test "${AWS_REGION:-}" = "$region" || fail "AWS_REGION must be $region"

for variable in AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN; do
	if printenv "$variable" >/dev/null 2>&1; then
		fail "$variable must be unset"
	fi
done

identity=$(aws sts get-caller-identity --output json --no-cli-pager)
actual_account=$(printf '%s\n' "$identity" | jq -er '.Account') || fail "could not read AWS account"
test "$actual_account" = "$account_id" || fail "unexpected AWS account $actual_account"
identity_arn=$(printf '%s\n' "$identity" | jq -er '.Arn') || fail "could not read AWS identity ARN"
case "$identity_arn" in
	arn:aws:sts::180294223248:assumed-role/AWSReservedSSO_PortfolioDeployer_*/*) ;;
	*) fail "AWS identity must use AWSReservedSSO_PortfolioDeployer_" ;;
esac

service=$(aws apprunner describe-service \
	--service-arn "$service_arn" \
	--output json \
	--no-cli-pager)
printf '%s\n' "$service" | jq -e \
	--arg service_arn "$service_arn" \
	'.Service.ServiceArn == $service_arn and
	 .Service.ServiceName == "portfolio" and
	 .Service.Status == "RUNNING" and
	 .Service.SourceConfiguration.AuthenticationConfiguration.AccessRoleArn == "arn:aws:iam::180294223248:role/portfolio-apprunner-ecr-access" and
	 .Service.SourceConfiguration.ImageRepository.ImageRepositoryType == "ECR" and
	 (.Service.SourceConfiguration.ImageRepository.ImageIdentifier | startswith("180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio:")) and
	 .Service.InstanceConfiguration.InstanceRoleArn == "arn:aws:iam::180294223248:role/portfolio-apprunner-instance"' \
	>/dev/null || fail "service contract failed"

custom_domains=$(aws apprunner describe-custom-domains \
	--service-arn "$service_arn" \
	--output json \
	--no-cli-pager)
printf '%s\n' "$custom_domains" | jq -e \
	--arg service_arn "$service_arn" \
	'.ServiceArn == $service_arn and (.CustomDomains | type == "array" and length == 0)' \
	>/dev/null || fail "custom-domain association remains"

tags=$(aws apprunner list-tags-for-resource \
	--resource-arn "$service_arn" \
	--output json \
	--no-cli-pager)
printf '%s\n' "$tags" | jq -e \
	'([.Tags[] | (.Key + "=" + .Value)] | sort) == [
		"Environment=development",
		"ManagedBy=opentofu",
		"Name=portfolio",
		"Project=portfolio"
	]' \
	>/dev/null || fail "service tag contract failed"

access_role=$(aws iam get-role \
	--role-name "$access_role_name" \
	--output json \
	--no-cli-pager) || fail "could not read ECR access role"
printf '%s\n' "$access_role" | jq -e \
	--arg role_id "$access_role_id" '
	.Role.RoleName == "portfolio-apprunner-ecr-access" and
	.Role.RoleId == $role_id and
	.Role.Arn == "arn:aws:iam::180294223248:role/portfolio-apprunner-ecr-access" and
	.Role.AssumeRolePolicyDocument == {
		Version: "2012-10-17",
		Statement: [{
			Effect: "Allow",
			Principal: {Service: "build.apprunner.amazonaws.com"},
			Action: "sts:AssumeRole"
		}]
	}' >/dev/null || fail "ECR access role identity contract failed"

instance_role=$(aws iam get-role \
	--role-name "$instance_role_name" \
	--output json \
	--no-cli-pager) || fail "could not read instance role"
printf '%s\n' "$instance_role" | jq -e \
	--arg role_id "$instance_role_id" '
	.Role.RoleName == "portfolio-apprunner-instance" and
	.Role.RoleId == $role_id and
	.Role.Arn == "arn:aws:iam::180294223248:role/portfolio-apprunner-instance" and
	.Role.AssumeRolePolicyDocument == {
		Version: "2012-10-17",
		Statement: [{
			Effect: "Allow",
			Principal: {Service: "tasks.apprunner.amazonaws.com"},
			Action: "sts:AssumeRole"
		}]
	}' >/dev/null || fail "instance role identity contract failed"

access_attachments=$(aws iam list-attached-role-policies \
	--role-name "$access_role_name" \
	--output json \
	--no-cli-pager)
printf '%s\n' "$access_attachments" | jq -e \
	'([.AttachedPolicies[].PolicyArn] | sort) == ["arn:aws:iam::aws:policy/service-role/AWSAppRunnerServicePolicyForECRAccess"]' \
	>/dev/null || fail "ECR access role attachment contract failed"

instance_attachments=$(aws iam list-attached-role-policies \
	--role-name "$instance_role_name" \
	--output json \
	--no-cli-pager)
printf '%s\n' "$instance_attachments" | jq -e \
	'([.AttachedPolicies[].PolicyArn] | sort) == [
		"arn:aws:iam::180294223248:policy/portfolio-apprunner-runtime-secrets",
		"arn:aws:iam::180294223248:policy/portfolio-google-connections-dynamodb",
		"arn:aws:iam::180294223248:policy/portfolio-soccer-sessions-dynamodb"
	]' >/dev/null || fail "instance role attachment contract failed"

for role_name in "$access_role_name" "$instance_role_name"; do
	inline_policies=$(aws iam list-role-policies \
		--role-name "$role_name" \
		--output json \
		--no-cli-pager)
	printf '%s\n' "$inline_policies" | jq -e '.PolicyNames | type == "array" and length == 0' \
		>/dev/null || fail "inline role policies remain on $role_name"

	instance_profiles=$(aws iam list-instance-profiles-for-role \
		--role-name "$role_name" \
		--output json \
		--no-cli-pager)
	printf '%s\n' "$instance_profiles" | jq -e '.InstanceProfiles | type == "array" and length == 0' \
		>/dev/null || fail "instance profiles remain on $role_name"
done

policy_entities=$(aws iam list-entities-for-policy \
	--policy-arn "$runtime_policy_arn" \
	--policy-usage-filter PermissionsPolicy \
	--output json \
	--no-cli-pager) || fail "could not read runtime policy attachments"
printf '%s\n' "$policy_entities" | jq -e \
	--arg role_id "$instance_role_id" '
	.PolicyGroups == [] and
	.PolicyUsers == [] and
	.PolicyRoles == [{
		RoleName: "portfolio-apprunner-instance",
		RoleId: $role_id
	}]' >/dev/null || fail "runtime policy attachment contract failed"

policy_boundaries=$(aws iam list-entities-for-policy \
	--policy-arn "$runtime_policy_arn" \
	--policy-usage-filter PermissionsBoundary \
	--output json \
	--no-cli-pager) || fail "could not read runtime policy boundary use"
printf '%s\n' "$policy_boundaries" | jq -e '
	.PolicyGroups == [] and
	.PolicyUsers == [] and
	.PolicyRoles == []' >/dev/null || fail "runtime policy boundary contract failed"

policy_versions=$(aws iam list-policy-versions \
	--policy-arn "$runtime_policy_arn" \
	--output json \
	--no-cli-pager)
printf '%s\n' "$policy_versions" | jq -e \
	'.Versions | type == "array" and length == 1 and .[0].VersionId == "v1" and .[0].IsDefaultVersion == true' \
	>/dev/null || fail "runtime policy version contract failed"

printf 'App Runner retirement preflight passed: service=%s account=%s region=%s\n' \
	"$service_arn" "$account_id" "$region"
