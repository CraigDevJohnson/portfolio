#!/bin/sh
set -eu

base=${1:?usage: classify-release-change.sh BASE HEAD}
head=${2:?usage: classify-release-change.sh BASE HEAD}
scope=${3:-current}

case "$scope" in
  current | development-backlog | release-review | release-review-current) ;;
  *)
    echo 'unsupported release classification scope' >&2
    exit 1
    ;;
esac

# Treat renames as a deletion plus an addition so both policy domains are
# classified. Otherwise Git reports only the destination path for a rename.
files=$(git -c core.quotePath=true diff --no-renames --name-only "$base" "$head")
test -n "$files" || {
  printf 'skip\n'
  exit 0
}
if printf '%s\n' "$files" | grep -q '^"'; then
  echo 'release classification refuses C-quoted paths' >&2
  exit 1
fi

if [ "$scope" = release-review ] || [ "$scope" = release-review-current ]; then
  review_change=false
  runtime_change=false
  current_promotion=false
  while IFS= read -r path; do
    case "$path" in
      docs/* | tests/* | *_test.go | .github/ISSUE_TEMPLATE/*) ;;
      deploy/production-release.json) current_promotion=true ;;
      .github/* | infra/* | scripts/* | Taskfile.yaml | Dockerfile* | deploy/*)
        review_change=true
        ;;
      */*) runtime_change=true ;;
      *.md) ;;
      *) runtime_change=true ;;
    esac
  done << EOF
$files
EOF
  if [ "$runtime_change" = true ] || [ "$current_promotion" = true ]; then
    printf 'development\n'
  elif [ "$review_change" = true ]; then
    printf 'review\n'
  else
    printf 'skip\n'
  fi
  exit 0
fi

promotion=true
runtime=false
skip_only=true
while IFS= read -r path; do
  case "$path" in
    deploy/production-release.json) [ "$scope" = current ] || promotion=false ;;
    *) promotion=false ;;
  esac
  case "$path" in
    docs/* | tests/* | *_test.go | .github/ISSUE_TEMPLATE/*) ;;
    deploy/production-release.json) [ "$scope" = development-backlog ] || skip_only=false ;;
    .github/* | infra/* | scripts/* | Taskfile.yaml | Dockerfile* | deploy/*)
      skip_only=false
      ;;
    cmd/* | internal/* | types/* | pkg/* | vendor/* | go.mod | go.sum | go.work | go.work.sum)
      runtime=true
      skip_only=false
      ;;
    */*) skip_only=false ;;
    *.md) ;;
    *) skip_only=false ;;
  esac
done << EOF
$files
EOF

if [ "$promotion" = true ]; then
  printf 'production\n'
elif [ "$runtime" = true ]; then
  # Mixed runtime and authority/infrastructure changes require review rather
  # than inheriting the runtime classification.
  authority_files=$files
  if [ "$scope" = development-backlog ]; then
    authority_files=$(printf '%s\n' "$files" | grep -Fvx 'deploy/production-release.json' || true)
  fi
  if printf '%s\n' "$authority_files" |
    grep -Eq '^(\.github/|infra/|scripts/|Taskfile\.yaml$|Dockerfile|deploy/)'; then
    printf 'review\n'
  else
    printf 'development\n'
  fi
elif [ "$skip_only" = true ]; then
  printf 'skip\n'
else
  printf 'review\n'
fi
