#!/bin/sh
set -eu

expected=${1:?set the exact expected CI role ARN}
account=${AWS_ACCOUNT_ID:-180294223248}
arn=$(aws sts get-caller-identity --query Arn --output text)
actual_account=$(aws sts get-caller-identity --query Account --output text)

test "$actual_account" = "$account" || {
  echo "unexpected AWS account" >&2
  exit 1
}
role=${expected#"arn:aws:iam::$account:role/"}
case "$arn" in
  arn:aws:sts::$account:assumed-role/"$role"/*) ;;
  *)
    echo "AWS identity is not the exact expected CI role: $expected" >&2
    exit 1
    ;;
esac
