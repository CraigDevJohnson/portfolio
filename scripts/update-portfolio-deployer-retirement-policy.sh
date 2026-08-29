#!/bin/sh
set -eu

account_id=180294223248
region=us-west-2
permission_set_name=PortfolioDeployer

repo_root=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
development_policy="$repo_root/infra/lambda/bootstrap/portfolio-deployer-development-bootstrap-policy.json"
retirement_policy="$repo_root/infra/lambda/bootstrap/portfolio-deployer-app-runner-retirement-policy.json"

fail() {
	printf 'PortfolioDeployer policy update failed: %s\n' "$1" >&2
	exit 1
}

normalize_policy_file() {
	jq -S -c . "$1"
}

read_live_policy() {
	aws sso-admin get-inline-policy-for-permission-set \
		--instance-arn "$instance_arn" \
		--permission-set-arn "$permission_set_arn" \
		--query InlinePolicy \
		--output text \
		--no-cli-pager
}

provision_and_verify() {
	expected_policy=$1
	request_id=$(aws sso-admin provision-permission-set \
		--instance-arn "$instance_arn" \
		--permission-set-arn "$permission_set_arn" \
		--target-type AWS_ACCOUNT \
		--target-id "$account_id" \
		--query PermissionSetProvisioningStatus.RequestId \
		--output text \
		--no-cli-pager) || return 1

	case "$request_id" in
		''|None) return 1 ;;
	esac

	attempt=0
	provisioning_status=
	while [ "$attempt" -lt 60 ]; do
		provisioning_status=$(aws sso-admin describe-permission-set-provisioning-status \
			--instance-arn "$instance_arn" \
			--provision-permission-set-request-id "$request_id" \
			--query PermissionSetProvisioningStatus.Status \
			--output text \
			--no-cli-pager) || return 1
		case "$provisioning_status" in
			SUCCEEDED) break ;;
			FAILED) return 1 ;;
			IN_PROGRESS) ;;
			*) return 1 ;;
		esac
		attempt=$((attempt + 1))
		sleep 5
	done
	[ "$provisioning_status" = SUCCEEDED ] || return 1

	installed_policy=$(read_live_policy) || return 1
	installed_normalized=$(printf '%s\n' "$installed_policy" | jq -S -c .) || return 1
	[ "$installed_normalized" = "$(normalize_policy_file "$expected_policy")" ] || return 1

	printf 'PortfolioDeployer provisioning succeeded: request_id=%s\n' "$request_id"
}

rollback_needed=false
rollback_on_failure() {
	exit_code=$?
	trap - EXIT
	if [ "$exit_code" -ne 0 ] && [ "$rollback_needed" = true ]; then
		printf 'Retirement policy installation failed; restoring the reviewed development policy\n' >&2
		if aws sso-admin put-inline-policy-to-permission-set \
			--instance-arn "$instance_arn" \
			--permission-set-arn "$permission_set_arn" \
			--inline-policy "file://$development_policy" \
			--no-cli-pager >/dev/null 2>&1 && \
			provision_and_verify "$development_policy"; then
			printf 'Reviewed development policy restored after failed installation\n' >&2
		else
			printf 'URGENT: automatic development-policy restoration failed; rerun restore with root\n' >&2
		fi
	fi
	exit "$exit_code"
}
trap rollback_on_failure EXIT

case "${1:-}" in
	install)
		source_policy=$development_policy
		target_policy=$retirement_policy
		approved_checksum=${APPROVED_POLICY_SHA256:-}
		;;
	restore)
		source_policy=$retirement_policy
		target_policy=$development_policy
		approved_checksum=${APPROVED_RESTORE_POLICY_SHA256:-}
		;;
	*) fail "usage: $0 install|restore" ;;
esac

[ "${AWS_PROFILE:-}" = root ] || fail "AWS_PROFILE must be root"
[ "${AWS_REGION:-}" = "$region" ] || fail "AWS_REGION must be $region"
for variable in AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN; do
	if printenv "$variable" >/dev/null 2>&1; then
		fail "$variable must be unset"
	fi
done

identity=$(aws sts get-caller-identity --output json --no-cli-pager) || fail "could not read root identity"
printf '%s\n' "$identity" | jq -e \
	--arg account "$account_id" \
	'.Account == $account and .Arn == ("arn:aws:iam::" + $account + ":root")' \
	>/dev/null || fail "identity must be the exact account root"

instances=$(aws sso-admin list-instances --output json --no-cli-pager) || fail "could not list Identity Center instances"
printf '%s\n' "$instances" | jq -e \
	'[.Instances[] | select(.Status == "ACTIVE") | .InstanceArn] | length == 1' \
	>/dev/null || fail "expected exactly one active Identity Center instance"
instance_arn=$(printf '%s\n' "$instances" | jq -er \
	'[.Instances[] | select(.Status == "ACTIVE") | .InstanceArn][0]') ||
	fail "could not resolve active instance ARN"

permission_sets=$(aws sso-admin list-permission-sets \
	--instance-arn "$instance_arn" \
	--output json \
	--no-cli-pager) || fail "could not list permission sets"
permission_set_arn=
permission_set_count=0
for candidate in $(printf '%s\n' "$permission_sets" | jq -er '.PermissionSets[]'); do
	name=$(aws sso-admin describe-permission-set \
		--instance-arn "$instance_arn" \
		--permission-set-arn "$candidate" \
		--query PermissionSet.Name \
		--output text \
		--no-cli-pager) || fail "could not describe permission set"
	if [ "$name" = "$permission_set_name" ]; then
		permission_set_arn=$candidate
		permission_set_count=$((permission_set_count + 1))
	fi
done
[ "$permission_set_count" -eq 1 ] || fail "expected exactly one PortfolioDeployer permission set"

provisioned_accounts=$(aws sso-admin list-accounts-for-provisioned-permission-set \
	--instance-arn "$instance_arn" \
	--permission-set-arn "$permission_set_arn" \
	--output json \
	--no-cli-pager) || fail "could not list provisioned accounts"
printf '%s\n' "$provisioned_accounts" | jq -e \
	--arg account "$account_id" \
	'.AccountIds == [$account]' \
	>/dev/null || fail "PortfolioDeployer must be provisioned only to the target account"

managed_policies=$(aws sso-admin list-managed-policies-in-permission-set \
	--instance-arn "$instance_arn" \
	--permission-set-arn "$permission_set_arn" \
	--output json \
	--no-cli-pager) || fail "could not list attached AWS managed policies"
printf '%s\n' "$managed_policies" | jq -e '.AttachedManagedPolicies == []' >/dev/null ||
	fail "PortfolioDeployer has unexpected AWS managed policies"

customer_policies=$(aws sso-admin list-customer-managed-policy-references-in-permission-set \
	--instance-arn "$instance_arn" \
	--permission-set-arn "$permission_set_arn" \
	--output json \
	--no-cli-pager) || fail "could not list customer managed policy references"
printf '%s\n' "$customer_policies" | jq -e '.CustomerManagedPolicyReferences == []' >/dev/null ||
	fail "PortfolioDeployer has unexpected customer managed policy references"

if boundary_output=$(aws sso-admin get-permissions-boundary-for-permission-set \
	--instance-arn "$instance_arn" \
	--permission-set-arn "$permission_set_arn" \
	--output json \
	--no-cli-pager 2>&1); then
	fail "PortfolioDeployer has an unexpected permissions boundary"
fi
case "$boundary_output" in
	*ResourceNotFoundException*"PermissionsBoundary not present"*) ;;
	*) fail "could not prove that PortfolioDeployer lacks a permissions boundary" ;;
esac

case "$approved_checksum" in
	????????????????????????????????????????????????????????????????) ;;
	*) fail "approved policy checksum must be exactly 64 lowercase hexadecimal characters" ;;
esac
printf '%s\n' "$approved_checksum" | grep -Eq '^[0-9a-f]{64}$' ||
	fail "approved policy checksum must be exactly 64 lowercase hexadecimal characters"
target_checksum=$(shasum -a 256 "$target_policy" | awk '{print $1}')
[ "$target_checksum" = "$approved_checksum" ] || fail "target policy checksum does not match approval"
[ "$(tr -d '[:space:]' < "$target_policy" | wc -c | tr -d ' ')" -le 10240 ] ||
	fail "target policy exceeds the Identity Center inline-policy limit"

validation=$(aws accessanalyzer validate-policy \
	--policy-document "file://$target_policy" \
	--policy-type IDENTITY_POLICY \
	--output json \
	--no-cli-pager) || fail "Access Analyzer validation failed"
printf '%s\n' "$validation" | jq -e '.findings == []' >/dev/null ||
	fail "Access Analyzer returned policy findings"

live_policy=$(read_live_policy) || fail "could not read the live inline policy"
live_normalized=$(printf '%s\n' "$live_policy" | jq -S -c .) ||
	fail "live inline policy is not valid JSON"
source_normalized=$(normalize_policy_file "$source_policy")
target_normalized=$(normalize_policy_file "$target_policy")
if [ "$live_normalized" = "$source_normalized" ]; then
	put_required=true
elif [ "$live_normalized" = "$target_normalized" ]; then
	put_required=false
else
	fail "live inline policy matches neither the reviewed source nor target"
fi

if [ "$1" = install ]; then
	rollback_needed=true
fi
if [ "$put_required" = true ]; then
	aws sso-admin put-inline-policy-to-permission-set \
		--instance-arn "$instance_arn" \
		--permission-set-arn "$permission_set_arn" \
		--inline-policy "file://$target_policy" \
		--no-cli-pager >/dev/null ||
		fail "could not replace the permission-set inline policy"
fi
provision_and_verify "$target_policy" ||
	fail "target policy provisioning or read-back verification failed"
rollback_needed=false

printf 'PortfolioDeployer policy update complete: action=%s sha256=%s\n' "$1" "$target_checksum"
