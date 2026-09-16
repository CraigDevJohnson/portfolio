#!/bin/sh
set -eu

development_base_sha=${1:?usage: validate-release-review-backlog.sh DEVELOPMENT_BASE REVIEWED_BASE SOURCE}
reviewed_base_sha=${2:?usage: validate-release-review-backlog.sh DEVELOPMENT_BASE REVIEWED_BASE SOURCE}
source_sha=${3:?usage: validate-release-review-backlog.sh DEVELOPMENT_BASE REVIEWED_BASE SOURCE}
script_dir=$(CDPATH='' cd -- "$(dirname "$0")" && pwd)

fail() {
  printf 'Release review backlog validation failed: %s\n' "$1" >&2
  exit 1
}

for candidate_sha in "$development_base_sha" "$reviewed_base_sha" "$source_sha"; do
  printf '%s\n' "$candidate_sha" | grep -Eq '^[0-9a-f]{40}$' ||
    fail 'all coordinates must be full lowercase commit SHAs'
  git cat-file -e "$candidate_sha^{commit}" 2> /dev/null ||
    fail "commit $candidate_sha is unavailable"
done
git merge-base --is-ancestor "$development_base_sha" "$reviewed_base_sha" ||
  fail 'reviewed pull-request base is not descended from the development base'
git merge-base --is-ancestor "$reviewed_base_sha" "$source_sha" ||
  fail 'source is not descended from the reviewed pull-request base'

checkpoint_classification=$(sh "$script_dir/classify-release-change.sh" \
  "$development_base_sha" "$source_sha" release-review)
current_classification=$(sh "$script_dir/classify-release-change.sh" \
  "$reviewed_base_sha" "$source_sha")
[ "$current_classification" = review ] ||
  fail 'current pull request is not review-class'
current_checkpoint_classification=$(sh "$script_dir/classify-release-change.sh" \
  "$reviewed_base_sha" "$source_sha" release-review-current)
[ "$current_checkpoint_classification" = review ] ||
  fail 'current pull request is not review-only'
[ "$checkpoint_classification" = review ] && exit 0
[ "$checkpoint_classification" = development ] ||
  fail 'checkpoint range is not review-class'

cursor_sha=$source_sha
pull_base_sha=$reviewed_base_sha
pull_count=0
promotion_count=0
while [ "$cursor_sha" != "$development_base_sha" ]; do
  pull_count=$((pull_count + 1))
  [ "$pull_count" -le 100 ] ||
    fail 'checkpoint recovery exceeds 100 reviewed pull requests'
  [ "$pull_base_sha" != "$cursor_sha" ] ||
    fail 'reviewed pull-request ancestry did not advance'
  git merge-base --is-ancestor "$development_base_sha" "$pull_base_sha" ||
    fail 'reviewed pull request starts before the development base'
  git merge-base --is-ancestor "$pull_base_sha" "$cursor_sha" ||
    fail 'reviewed pull-request base is not an ancestor of its merge'

  pull_classification=$(sh "$script_dir/classify-release-change.sh" \
    "$pull_base_sha" "$cursor_sha")
  pull_checkpoint_classification=$(sh "$script_dir/classify-release-change.sh" \
    "$pull_base_sha" "$cursor_sha" release-review-current)
  case "$pull_classification:$pull_checkpoint_classification" in
    review:review | skip:skip) ;;
    production:development)
      promotion_count=$((promotion_count + 1))
      [ "$promotion_count" -eq 1 ] ||
        fail 'checkpoint recovery contains multiple production promotions'
      ;;
    *) fail 'checkpoint recovery contains a runtime, mixed, or unknown pull request' ;;
  esac

  cursor_sha=$pull_base_sha
  [ "$cursor_sha" = "$development_base_sha" ] && break
  pull_base_sha=$(sh "$script_dir/resolve-reviewed-release-base.sh" \
    "$cursor_sha") ||
    fail 'checkpoint recovery contains a commit without a unique reviewed pull request'
done

[ "$promotion_count" -eq 1 ] ||
  fail 'checkpoint recovery does not contain exactly one production promotion'
