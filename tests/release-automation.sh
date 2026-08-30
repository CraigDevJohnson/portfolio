#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
git -C "$tmp" init -q
git -C "$tmp" config user.email test@example.com
git -C "$tmp" config user.name Test
mkdir -p "$tmp/internal/app" "$tmp/docs" "$tmp/deploy" "$tmp/.github/workflows"
echo a >"$tmp/internal/app/app.go"; git -C "$tmp" add .; git -C "$tmp" commit -qm base; base=$(git -C "$tmp" rev-parse HEAD)
echo b >>"$tmp/internal/app/app.go"; git -C "$tmp" commit -qam runtime; head=$(git -C "$tmp" rev-parse HEAD)
test "$(cd "$tmp" && sh "$root/scripts/classify-release-change.sh" "$base" "$head")" = development
base=$head; echo doc >"$tmp/docs/readme.md"; git -C "$tmp" add .; git -C "$tmp" commit -qm docs; head=$(git -C "$tmp" rev-parse HEAD)
test "$(cd "$tmp" && sh "$root/scripts/classify-release-change.sh" "$base" "$head")" = skip
base=$head; echo '{}' >"$tmp/deploy/production-release.json"; git -C "$tmp" add .; git -C "$tmp" commit -qm promote; head=$(git -C "$tmp" rev-parse HEAD)
test "$(cd "$tmp" && sh "$root/scripts/classify-release-change.sh" "$base" "$head")" = production
base=$head; echo x >"$tmp/.github/workflows/x.yml"; git -C "$tmp" add .; git -C "$tmp" commit -qm workflow; head=$(git -C "$tmp" rev-parse HEAD)
test "$(cd "$tmp" && sh "$root/scripts/classify-release-change.sh" "$base" "$head")" = review
printf 'Release automation contracts passed\n'
