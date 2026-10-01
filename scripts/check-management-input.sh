#!/bin/sh
# The development management input is an identity-free portal switch: null
# (portal grants off) or exactly {"aws_region":"us-west-2"}, which grants the
# portal read-only EC2 inventory and metrics in that region and sets
# MGMT_AWS_REGION. The retired Cognito, callback and allowlist fields are
# refused, and the input is never echoed.
set -eu
fail() {
  printf 'Management input rejected: %s\n' "$1" >&2
  exit 1
}
EXPECTED_MANAGEMENT_JSON=${EXPECTED_MANAGEMENT_JSON:-null}
printf '%s\n' "$EXPECTED_MANAGEMENT_JSON" | jq -e -s --arg env "$ENVIRONMENT" '
  if length != 1 then false
  else .[0] |
    if . == null then true
    else $env == "dev" and . == {"aws_region": "us-west-2"}
    end
  end
' >/dev/null 2>&1 || fail 'expected management must be null or exactly {"aws_region":"us-west-2"} for development'
