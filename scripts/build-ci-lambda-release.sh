#!/bin/sh
set -eu

: "${SOURCE_SHA:?set SOURCE_SHA}"
: "${GITHUB_OUTPUT:?set GITHUB_OUTPUT}"
: "${GITHUB_REPOSITORY:?set GITHUB_REPOSITORY}"
: "${ECR_REPOSITORY:?set ECR_REPOSITORY}"
: "${ECR_URL:?set ECR_URL}"
script_dir=$(CDPATH='' cd -- "$(dirname "$0")" && pwd)
SCAN_FILE=${SCAN_FILE:-scan.json}

printf '%s\n' "$SOURCE_SHA" | grep -Eq '^[0-9a-f]{40}$' || {
  echo 'SOURCE_SHA must be a full lowercase commit SHA' >&2
  exit 1
}

tag="git-$SOURCE_SHA"
sh "$script_dir/check-current-main.sh" "$SOURCE_SHA"
aws ecr describe-repositories \
  --repository-names "$ECR_REPOSITORY" \
  --query 'repositories[0].imageTagMutability' \
  --output text | grep -Fx IMMUTABLE

lookup_error=$(mktemp)
cleanup() {
  rm -f "$lookup_error"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM
if existing_digest=$(aws ecr describe-images \
  --repository-name "$ECR_REPOSITORY" \
  --image-ids "imageTag=$tag" \
  --query 'imageDetails[0].imageDigest' \
  --output text 2> "$lookup_error"); then
  printf '%s\n' "$existing_digest" | grep -Eq '^sha256:[0-9a-f]{64}$' || {
    echo 'ECR returned an invalid digest for the existing immutable release tag' >&2
    exit 1
  }
  echo 'Immutable full-SHA tag already exists; reusing it for scan validation.'
else
  grep -Fq \
    'An error occurred (ImageNotFoundException) when calling the DescribeImages operation:' \
    "$lookup_error" || {
    cat "$lookup_error" >&2
    echo 'Could not prove that the immutable release tag is absent' >&2
    exit 1
  }
  aws ecr get-login-password | docker login \
    --username AWS \
    --password-stdin "${ECR_URL%%/*}"
  docker buildx build \
    --platform linux/amd64 \
    --provenance=false \
    --sbom=false \
    --build-arg "BUILD_REVISION=$SOURCE_SHA" \
    -f Dockerfile.lambda \
    -t "$ECR_URL:$tag" \
    --load .
  sh "$script_dir/check-current-main.sh" "$SOURCE_SHA"
  docker push "$ECR_URL:$tag"
fi

digest=$(aws ecr describe-images \
  --repository-name "$ECR_REPOSITORY" \
  --image-ids "imageTag=$tag" \
  --query 'imageDetails[0].imageDigest' \
  --output text)
printf '%s\n' "$digest" | grep -Eq '^sha256:[0-9a-f]{64}$'

scan_visible=false
scan_attempt=0
while [ "$scan_attempt" -lt 12 ]; do
  scan_attempt=$((scan_attempt + 1))
  if aws ecr describe-image-scan-findings \
    --repository-name "$ECR_REPOSITORY" \
    --image-id "imageDigest=$digest" \
    --no-paginate \
    --query 'imageScanStatus.status' \
    --output text > /dev/null 2> "$lookup_error"; then
    scan_visible=true
    break
  fi
  grep -Fq \
    'An error occurred (ScanNotFoundException) when calling the DescribeImageScanFindings operation:' \
    "$lookup_error" || {
    cat "$lookup_error" >&2
    exit 1
  }
  if [ "$scan_attempt" -lt 12 ]; then
    sleep 5
  fi
done
[ "$scan_visible" = true ] || {
  echo 'ECR scan record did not appear within the bounded release wait' >&2
  exit 1
}
aws ecr wait image-scan-complete \
  --repository-name "$ECR_REPOSITORY" \
  --image-id "imageDigest=$digest"
aws ecr describe-image-scan-findings \
  --repository-name "$ECR_REPOSITORY" \
  --image-id "imageDigest=$digest" \
  --no-paginate > "$SCAN_FILE"
jq -e '
  .imageScanStatus.status == "COMPLETE" and
  ((.imageScanFindings.findingSeverityCounts.CRITICAL // 0) == 0)
' "$SCAN_FILE" > /dev/null
sh "$script_dir/check-current-main.sh" "$SOURCE_SHA"
printf 'digest=%s\n' "$digest" >> "$GITHUB_OUTPUT"
