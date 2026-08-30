#!/bin/sh
set -eu

: "${PLAN_FILE:?set PLAN_FILE to a new absolute saved-plan path}"

case "$PLAN_FILE" in
	/*) ;;
	*)
		printf 'PLAN_FILE must be absolute\n' >&2
		exit 1
		;;
esac

test ! -e "$PLAN_FILE" || {
	printf 'Refusing existing PLAN_FILE: %s\n' "$PLAN_FILE" >&2
	exit 1
}

plan_json=$(mktemp)
retain_plan=false
cleanup() {
	rm -f "$plan_json"
	if [ "$retain_plan" != true ]; then
		rm -f "$PLAN_FILE"
	fi
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

tofu -chdir=infra/lambda/ci-roles plan -lock-timeout=5m -input=false -out="$PLAN_FILE"
tofu -chdir=infra/lambda/ci-roles show -json "$PLAN_FILE" >"$plan_json"
PLAN_JSON="$plan_json" sh scripts/check-ci-roles-plan.sh
tofu -chdir=infra/lambda/ci-roles show -no-color "$PLAN_FILE"
retain_plan=true
