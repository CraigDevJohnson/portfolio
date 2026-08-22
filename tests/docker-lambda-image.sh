#!/bin/sh
set -eu

image="portfolio-lambda-contract:$$"
container="portfolio-lambda-contract-$$"
tmp_dir=$(mktemp -d)
cleanup() {
	docker rm -f "$container" >/dev/null 2>&1 || true
	docker image rm "$image" >/dev/null 2>&1 || true
	rm -rf "$tmp_dir"
}
trap cleanup EXIT INT TERM

openssl req -x509 -newkey rsa:2048 -nodes \
	-keyout "$tmp_dir/build-only-ca.key" \
	-out "$tmp_dir/build-only-ca.pem" \
	-subj "/CN=Portfolio Lambda Build Only CA Contract" \
	-days 1 >/dev/null 2>&1

: > "$tmp_dir/build-ca.pem"
if command -v security >/dev/null 2>&1 && [ -r /Library/Keychains/System.keychain ]; then
	security find-certificate -a -c "Gateway CA" -p \
		/Library/Keychains/System.keychain > "$tmp_dir/build-ca.pem" || true
fi
cat "$tmp_dir/build-only-ca.pem" >> "$tmp_dir/build-ca.pem"
build_ca_digest=$(shasum -a 256 "$tmp_dir/build-ca.pem" | awk '{print $1}')

docker build --platform linux/amd64 \
	--secret "id=build_ca_bundle,src=$tmp_dir/build-ca.pem" \
	--build-arg "BUILD_CA_BUNDLE_DIGEST=$build_ca_digest" \
	--build-arg BUILD_REVISION=0123456789abcdef0123456789abcdef01234567 \
	-f Dockerfile.lambda -t "$image" .

test "$(docker image inspect --format '{{.Architecture}}' "$image")" = "amd64"
docker create --name "$container" "$image" >/dev/null
docker cp "$container:/var/task/bootstrap" "$tmp_dir/bootstrap"
file "$tmp_dir/bootstrap" | grep -Eq 'x86-64|x86_64'
strings "$tmp_dir/bootstrap" | \
	grep -F -q '0123456789abcdef0123456789abcdef01234567'
docker cp -L "$container:/etc/pki/tls/certs/ca-bundle.crt" "$tmp_dir/runtime-ca.pem"
build_ca_payload=$(sed -n '2p' "$tmp_dir/build-only-ca.pem")
if grep -F -q "$build_ca_payload" "$tmp_dir/runtime-ca.pem"; then
	echo "build-only CA unexpectedly present in the Lambda runtime image" >&2
	exit 1
fi
