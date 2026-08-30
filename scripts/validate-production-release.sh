#!/bin/sh
set -eu

manifest=${1:?usage: validate-production-release.sh MANIFEST}
: "${ECR_REPOSITORY:?set ECR_REPOSITORY}"
: "${GITHUB_REPOSITORY:?set GITHUB_REPOSITORY}"

source_sha=$(jq -er '.source_sha | select(type == "string" and test("^[0-9a-f]{40}$"))' "$manifest")
image_digest=$(jq -er '.image_digest | select(type == "string" and test("^sha256:[0-9a-f]{64}$"))' "$manifest")
deployment_id=$(jq -er '.development_deployment_id | select(type == "number" and . > 0 and floor == .)' "$manifest")

deployment=$(gh api "repos/$GITHUB_REPOSITORY/deployments/$deployment_id")
printf '%s\n' "$deployment" | jq -e \
	--arg source_sha "$source_sha" \
	--arg image_digest "$image_digest" '
		.ref == $source_sha
		and .environment == "development"
		and .description == ("Lambda " + $image_digest)
	' >/dev/null

statuses=$(gh api "repos/$GITHUB_REPOSITORY/deployments/$deployment_id/statuses")
printf '%s\n' "$statuses" | jq -e \
	--arg source_sha "$source_sha" \
	--arg image_digest "$image_digest" '
		length > 0
		and .[0].state == "success"
		and .[0].environment == "development"
		and .[0].description == ("Verified " + $source_sha + " at " + $image_digest)
	' >/dev/null

tag_digest=$(aws ecr describe-images \
	--repository-name "$ECR_REPOSITORY" \
	--image-ids "imageTag=git-$source_sha" \
	--query 'imageDetails[0].imageDigest' \
	--output text)
test "$tag_digest" = "$image_digest" || {
	echo 'the immutable source-SHA tag does not resolve to the promoted digest' >&2
	exit 1
}
