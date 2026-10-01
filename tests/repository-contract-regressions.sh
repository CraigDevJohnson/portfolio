#!/bin/sh
# Offline regressions for tests/repository-contract.sh. Each case runs the
# contract in a scratch copy holding only the files it reads.
set -eu

root_dir=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT HUP INT TERM

fail() {
	printf 'FAIL: %s\n' "$1" >&2
	exit 1
}

# scratch NAME: print the path of a new copy of what the contract reads.
scratch() {
	copy="$test_dir/$1"
	mkdir -p "$copy/tests" "$copy/infra/lambda"
	cp "$root_dir/Taskfile.yaml" "$root_dir/AGENTS.md" "$root_dir/README.md" \
		"$root_dir/DEPLOY-INSTRUCTIONS.md" "$copy/"
	cp "$root_dir/tests/repository-contract.sh" "$copy/tests/"
	printf '%s\n' "$copy"
}

copy=$(scratch clean)
sh "$copy/tests/repository-contract.sh" > /dev/null 2>&1 ||
	fail 'a clean copy fails the repository contract'

# A checkout that ever ran the retired cognito-dev-ci keeps the ignored
# OpenTofu data directory after Git deletes the root's tracked files.
copy=$(scratch ignored-data)
mkdir -p "$copy/infra/lambda/auth/dev/.terraform/providers"
sh "$copy/tests/repository-contract.sh" > /dev/null 2>&1 ||
	fail 'the retired root ignored .terraform directory fails the repository contract'

# Authored configuration of the retired root, or a retired script, still
# fails it.
retired_root=infra/lambda/auth/dev
for path in "$retired_root/main.tf" "$retired_root/variables.tf.json" \
	"$retired_root/backend.hcl" "$retired_root/.terraform.lock.hcl" \
	"$retired_root/dev.auto.tfvars" "$retired_root/tests/auth_contract.tftest.hcl" \
	scripts/create-cognito-dev-plan.py; do
	copy=$(scratch "authored-$(printf '%s' "$path" | tr '/.' '--')")
	mkdir -p "$(dirname "$copy/$path")"
	: > "$copy/$path"
	if sh "$copy/tests/repository-contract.sh" > "$copy/out" 2>&1; then
		fail "retired $path passes the repository contract"
	fi
	grep -q "retired management-only identity is back: $path" "$copy/out" ||
		fail "retired $path fails the repository contract for another reason"
done

printf 'PASS: the repository contract refuses the retired root, not its ignored data\n'
