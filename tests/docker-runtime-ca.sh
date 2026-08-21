#!/bin/sh

set -eu

runtime_ca_dir=$(mktemp -d)
runtime_ca_image="portfolio-runtime-ca-contract:$$"
runtime_ca_container="portfolio-runtime-ca-contract-$$"

cleanup() {
	docker rm -f "$runtime_ca_container" >/dev/null 2>&1 || true
	docker image rm "$runtime_ca_image" >/dev/null 2>&1 || true
	rm -rf "$runtime_ca_dir"
}
trap cleanup EXIT INT TERM

openssl req -x509 -newkey rsa:2048 -nodes \
	-keyout "$runtime_ca_dir/test-ca.key" \
	-out "$runtime_ca_dir/test-ca.pem" \
	-subj "/CN=Portfolio Runtime CA Contract" \
	-days 1 >/dev/null 2>&1

: > "$runtime_ca_dir/build-ca.pem"
if command -v security >/dev/null 2>&1 && [ -r /Library/Keychains/System.keychain ]; then
	security find-certificate -a -c "Gateway CA" -p \
		/Library/Keychains/System.keychain > "$runtime_ca_dir/build-ca.pem" || true
fi
cat "$runtime_ca_dir/test-ca.pem" >> "$runtime_ca_dir/build-ca.pem"
runtime_ca_digest=$(shasum -a 256 "$runtime_ca_dir/build-ca.pem" | awk '{print $1}')

docker build \
	--secret "id=build_ca_bundle,src=$runtime_ca_dir/build-ca.pem" \
	--build-arg "BUILD_CA_BUNDLE_DIGEST=$runtime_ca_digest" \
	-t "$runtime_ca_image" \
	. >/dev/null

docker create --name "$runtime_ca_container" "$runtime_ca_image" >/dev/null
docker cp "$runtime_ca_container:/etc/ssl/certs/ca-certificates.crt" "$runtime_ca_dir/runtime-ca.pem"

openssl verify -CAfile "$runtime_ca_dir/runtime-ca.pem" "$runtime_ca_dir/test-ca.pem"
