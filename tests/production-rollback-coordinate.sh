#!/bin/sh
set -eu
root=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
mkdir "$tmp/bin"
cat > "$tmp/bin/gh" <<'GH'
#!/bin/sh
set -eu
case "$*" in
  *'deployments?environment=production'*) cat "$DEPLOYMENTS_FIXTURE" ;;
  *'deployments/102/statuses'*) cat "$STATUS_102_FIXTURE" ;;
  *'deployments/101/statuses'*) cat "$STATUS_101_FIXTURE" ;;
  *) exit 97 ;;
esac
GH
chmod +x "$tmp/bin/gh"
sha=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
digest=sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
cat > "$tmp/deployments.json" <<EOF2
[[{"id":102,"created_at":"2026-09-20T02:00:00Z","ref":"cccccccccccccccccccccccccccccccccccccccc","sha":"cccccccccccccccccccccccccccccccccccccccc","task":"portfolio-lambda-production","environment":"production","description":"Lambda $digest rollback-v7","creator":{"login":"github-actions[bot]","type":"Bot"}},{"id":101,"created_at":"2026-09-20T01:00:00Z","ref":"$sha","sha":"$sha","task":"portfolio-lambda-production","environment":"production","description":"Lambda $digest rollback-v6","creator":{"login":"github-actions[bot]","type":"Bot"}}]]
EOF2
cat > "$tmp/status102.json" <<EOF2
[[{"id":202,"created_at":"2026-09-20T02:10:00Z","state":"failure","environment":"production","description":"Failed cccccccccccccccccccccccccccccccccccccccc at $digest","creator":{"login":"github-actions[bot]","type":"Bot"}}]]
EOF2
cat > "$tmp/status101.json" <<EOF2
[[{"id":201,"created_at":"2026-09-20T01:10:00Z","state":"success","environment":"production","environment_url":"https://craigdevjohnson.com","description":"Verified $sha $digest v7 public-apex=ok public-www=ok","creator":{"login":"github-actions[bot]","type":"Bot"}}]]
EOF2
export PATH="$tmp/bin:$PATH" GITHUB_REPOSITORY=CraigDevJohnson/portfolio
export DEPLOYMENTS_FIXTURE="$tmp/deployments.json" STATUS_102_FIXTURE="$tmp/status102.json" STATUS_101_FIXTURE="$tmp/status101.json"
out=$(sh "$root/scripts/resolve-production-rollback-coordinate.sh")
[ "$out" = "101	$sha	$digest	7" ]
# Current alias is deliberately unavailable to the resolver; durable status is sufficient.
grep -q '^101' <<EOF2
$out
EOF2
# A success without both public-host verdicts is not a durable verified target.
sed 's/ public-www=ok//' "$tmp/status101.json" > "$tmp/unverified.json"
STATUS_101_FIXTURE="$tmp/unverified.json"; export STATUS_101_FIXTURE
if sh "$root/scripts/resolve-production-rollback-coordinate.sh" >/dev/null 2>&1; then
  echo 'accepted production status without both public host verifications' >&2; exit 1
fi
echo 'Production rollback coordinate contracts passed'

grep -Fq 'resolve-production-rollback-coordinate.sh' "$root/scripts/plan-ci-lambda-production.sh"
if grep -Fq 'aws lambda get-alias' "$root/scripts/plan-ci-lambda-production.sh"; then
  echo 'production planning still trusts the mutable live alias as rollback evidence' >&2
  exit 1
fi
for field in apply_authorized manifest_sha256 plan_json_sha256 plan_text_sha256 policy_sha256 \
  backend_sha256 production_deployment_id prior_verified_version planning_environment; do
  grep -Fq "$field" "$root/scripts/plan-ci-lambda-production.sh" || {
    echo "production release identity omits $field" >&2; exit 1;
  }
done
if grep -Fq 'apply_ready' "$root/scripts/plan-ci-lambda-production.sh"; then
  echo 'production preparation uses ambiguous apply-ready terminology' >&2
  exit 1
fi
