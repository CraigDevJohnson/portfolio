#!/bin/sh
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)

fail() {
	printf 'FAIL: %s\n' "$1" >&2
	exit 1
}

test ! -e "$repo_root/infra/lambda.tf" || fail "legacy root still declares absent Lambda/API resources"

actual_resources=$(
	rg --no-filename -o '^resource "[^"]+" "[^"]+"' "$repo_root"/infra/*.tf |
		sed 's/^resource "//; s/" "/./; s/"$//' |
		sort
)
expected_resources=$(printf '%s\n' \
	'aws_dynamodb_table.google_connections' \
	'aws_dynamodb_table.soccer_sessions' \
	'aws_ecr_lifecycle_policy.app' \
	'aws_ecr_repository.app' \
	'aws_iam_policy.google_connections_dynamodb' \
	'aws_iam_policy.soccer_sessions_dynamodb')
test "$actual_resources" = "$expected_resources" || {
	printf 'Unexpected legacy-root managed resources:\n%s\n' "$actual_resources" >&2
	exit 1
}

actual_outputs=$(
	rg --no-filename -o '^output "[^"]+"' "$repo_root"/infra/*.tf |
		sed 's/^output "//; s/"$//' |
		sort
)
expected_outputs=$(printf '%s\n' \
	'ecr_repository_url' \
	'google_connection_table_arn' \
	'google_connection_table_name')
test "$actual_outputs" = "$expected_outputs" || {
	printf 'Unexpected legacy-root outputs:\n%s\n' "$actual_outputs" >&2
	exit 1
}

if rg -n '^  (deploy-lambda|redeploy-lambda):' "$repo_root/Taskfile.yaml" >/dev/null; then
	fail "Taskfile can recreate absent legacy Lambda/API resources"
fi

if rg -n '^variable "lambda_' "$repo_root/infra/variables.tf" >/dev/null; then
	fail "legacy root still exposes variables for absent Lambda resources"
fi

rg -F 'version = "= 5.100.0"' "$repo_root/infra/versions.tf" >/dev/null ||
	fail "legacy retirement provider version is not pinned to reviewed v5.100.0"

legacy_lock="$repo_root/infra/.terraform.lock.hcl"
test -f "$legacy_lock" || fail "legacy retirement provider lock file is missing"
rg -F 'version     = "5.100.0"' "$legacy_lock" >/dev/null ||
	fail "legacy retirement provider lock does not select v5.100.0"
rg -F 'constraints = "5.100.0"' "$legacy_lock" >/dev/null ||
	fail "legacy retirement provider lock does not record the exact constraint"
test "$(rg -c '^    "(h1|zh):' "$legacy_lock")" -ge 2 ||
	fail "legacy retirement provider lock lacks reviewed checksums"
test "$(shasum -a 256 "$legacy_lock" | awk '{print $1}')" = \
	56bac28a0a2876d61b064ce149df8ba59285a8ede77d1c444cf0b2145ce1ec45 ||
	fail "legacy retirement provider lock checksum changed"

rg -F 'scripts/update-portfolio-deployer-retirement-policy.sh install' \
	"$repo_root/DEPLOY-INSTRUCTIONS.md" >/dev/null ||
	fail "retirement runbook does not use the checked install helper"
rg -F 'scripts/update-portfolio-deployer-retirement-policy.sh restore' \
	"$repo_root/DEPLOY-INSTRUCTIONS.md" >/dev/null ||
	fail "retirement runbook does not use the checked restore helper"
rg -F '.CustomDomains == []' "$repo_root/DEPLOY-INSTRUCTIONS.md" >/dev/null ||
	fail "retirement runbook does not assert that custom domains are empty"
rg -F '.PolicyNames == []' "$repo_root/DEPLOY-INSTRUCTIONS.md" >/dev/null ||
	fail "retirement runbook does not assert that inline role policies are empty"

printf 'PASS: legacy root retains only shared data and artifact resources\n'
