#!/bin/sh
# Classify the change between two commits for the Release workflow:
#   skip    - only docs and tests changed; nothing to release
#   release - application change; build and deploy without extra review
#   review  - release tooling or infrastructure changed; the release first
#             waits for approval in the release-review GitHub Environment
set -eu

base=${1:?usage: classify-release-change.sh BASE HEAD}
head=${2:?usage: classify-release-change.sh BASE HEAD}

# Treat renames as a deletion plus an addition so both paths are classified.
files=$(git -c core.quotePath=true diff --no-renames --name-only "$base" "$head")
if printf '%s\n' "$files" | grep -q '^"'; then
  echo 'release classification refuses C-quoted paths' >&2
  exit 1
fi

classification=skip
while IFS= read -r path; do
  case "$path" in
    '' | docs/* | tests/* | *_test.go | .github/ISSUE_TEMPLATE/* | .claude/* | .cursor/*) ;;
    .github/* | infra/* | scripts/* | Taskfile.yaml | Dockerfile* | deploy/*)
      classification=review
      ;;
    */*) [ "$classification" = review ] || classification=release ;;
    *.md) ;;
    *) [ "$classification" = review ] || classification=release ;;
  esac
done << EOF_FILES
$files
EOF_FILES

printf '%s\n' "$classification"
