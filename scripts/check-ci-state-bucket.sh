#!/bin/sh
set -eu

: "${STATE_BUCKET:?set STATE_BUCKET}"
status=$(aws s3api get-bucket-versioning \
	--bucket "$STATE_BUCKET" \
	--query Status \
	--output text)
test "$status" = Enabled || {
	echo "Terraform state bucket versioning is not enabled: $STATE_BUCKET" >&2
	exit 1
}
