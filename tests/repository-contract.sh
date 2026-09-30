#!/bin/sh
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)

fail() {
	printf 'FAIL: %s\n' "$1" >&2
	exit 1
}

# The legacy shared root under infra/ is deleted; every OpenTofu root lives
# under infra/lambda/. A configuration or lock file directly under infra/
# would bring it back.
for path in "$repo_root"/infra/*.tf "$repo_root"/infra/*.tf.json \
	"$repo_root"/infra/*.tofu "$repo_root"/infra/*.tofu.json \
	"$repo_root"/infra/*.tfvars "$repo_root"/infra/.terraform.lock.hcl; do
	test ! -e "$path" || fail "retired legacy root file is back: ${path#"$repo_root"/}"
done

if grep -Eq '^  (deploy|redeploy|logs|deploy-lambda|redeploy-lambda|legacy-apprunner-retirement-(init|preflight|plan|apply)):' \
	"$repo_root/Taskfile.yaml"; then
	fail "Taskfile exposes a retired shared-stack deployment or App Runner interface"
fi

# The management-only development identity is retired (decision 8,
# 2026-09-30): the portal signs in through site identity, and the site roots
# have their own operator tasks.
for path in infra/lambda/auth/dev scripts/create-cognito-dev-plan.py \
	scripts/check-cognito-dev-plan.py scripts/test_cognito_dev_plan.py; do
	test ! -e "$repo_root/$path" || fail "retired management-only identity is back: $path"
done
if grep -Eq '^  cognito-dev-[a-z-]+:' "$repo_root/Taskfile.yaml"; then
	fail "Taskfile exposes a retired management-only cognito-dev task"
fi

# The backticks are Markdown delimiters, not shell syntax.
# shellcheck disable=SC2016
for document in AGENTS.md README.md DEPLOY-INSTRUCTIONS.md; do
	advertised_tasks=$(
		grep -Eo '`task [a-z0-9-]+`' "$repo_root/$document" |
			sed 's/^`task //; s/`$//' |
			sort -u
	)
	for task_name in $advertised_tasks; do
		task --dir "$repo_root" --summary "$task_name" >/dev/null 2>&1 ||
			fail "$document advertises nonexistent task $task_name"
	done
done

printf 'PASS: the retired roots stay deleted and the docs advertise only existing tasks\n'
