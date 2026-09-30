#!/bin/sh
# Verify that GitHub OIDC credentials belong to the exact expected CI role in
# the portfolio account.
set -eu

expected=${1:?set the exact expected CI role ARN}
account=${PORTFOLIO_ACCOUNT_ID:?set PORTFOLIO_ACCOUNT_ID}
arn=$(aws sts get-caller-identity --query Arn --output text)
actual_account=$(aws sts get-caller-identity --query Account --output text)

test "$actual_account" = "$account" || {
  echo "unexpected AWS account" >&2
  exit 1
}
role=${expected#"arn:aws:iam::$account:role/"}
[ "$role" != "$expected" ] || {
  echo "expected role ARN is not in the portfolio account: $expected" >&2
  exit 1
}
case "$arn" in
  arn:aws:sts::$account:assumed-role/"$role"/*) ;;
  *)
    echo "AWS identity is not the exact expected CI role: $expected" >&2
    exit 1
    ;;
esac
