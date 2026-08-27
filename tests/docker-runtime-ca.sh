#!/bin/sh

set -eu

runtime_ca_dir=$(mktemp -d)
runtime_ca_image="portfolio-runtime-ca-contract:$$"
runtime_ca_container="portfolio-runtime-ca-contract-$$"
contract_revision=0123456789abcdef0123456789abcdef01234567

cleanup() {
	docker rm -f "$runtime_ca_container" >/dev/null 2>&1 || true
	docker image rm "$runtime_ca_image" >/dev/null 2>&1 || true
	rm -rf "$runtime_ca_dir"
}
trap cleanup EXIT INT TERM

openssl req -x509 -newkey rsa:2048 -nodes \
	-keyout "$runtime_ca_dir/build-only-ca.key" \
	-out "$runtime_ca_dir/build-only-ca.pem" \
	-subj "/CN=Portfolio Build Only CA Contract" \
	-days 1 >/dev/null 2>&1

openssl req -x509 -newkey rsa:2048 -nodes \
	-keyout "$runtime_ca_dir/runtime-only-ca.key" \
	-out "$runtime_ca_dir/runtime-only-ca.pem" \
	-subj "/CN=Portfolio Runtime Only CA Contract" \
	-days 1 >/dev/null 2>&1

: > "$runtime_ca_dir/build-ca.pem"
if command -v security >/dev/null 2>&1 && [ -r /Library/Keychains/System.keychain ]; then
	security find-certificate -a -c "Gateway CA" -p \
		/Library/Keychains/System.keychain > "$runtime_ca_dir/build-ca.pem" || true
fi
cat "$runtime_ca_dir/build-only-ca.pem" >> "$runtime_ca_dir/build-ca.pem"
cp "$runtime_ca_dir/runtime-only-ca.pem" "$runtime_ca_dir/runtime-ca-secret.pem"
build_ca_digest=$(shasum -a 256 "$runtime_ca_dir/build-ca.pem" | awk '{print $1}')
runtime_ca_digest=$(shasum -a 256 "$runtime_ca_dir/runtime-ca-secret.pem" | awk '{print $1}')

missing_tailwind_version=v0.0.0-portfolio-contract-missing
invalid_tailwind_log="$runtime_ca_dir/invalid-tailwind.log"
if docker build --platform linux/amd64 --target builder \
	--secret "id=build_ca_bundle,src=$runtime_ca_dir/build-ca.pem" \
	--secret "id=runtime_ca_bundle,src=$runtime_ca_dir/runtime-ca-secret.pem" \
	--build-arg "BUILD_CA_BUNDLE_DIGEST=$build_ca_digest" \
	--build-arg "RUNTIME_CA_BUNDLE_DIGEST=$runtime_ca_digest" \
	--build-arg "TAILWIND_VERSION=$missing_tailwind_version" \
	. >"$invalid_tailwind_log" 2>&1; then
	echo "regular image unexpectedly built with a missing Tailwind release" >&2
	exit 1
fi
if grep -F -q 'tailwindcss: not found' "$invalid_tailwind_log" \
	|| ! grep 'ERROR: process' "$invalid_tailwind_log" | grep -F -q 'curl --cacert'; then
	echo "regular Tailwind download failure was masked or reported by a later layer" >&2
	tail -n 40 "$invalid_tailwind_log" >&2
	exit 1
fi

docker build --platform linux/amd64 \
	--secret "id=build_ca_bundle,src=$runtime_ca_dir/build-ca.pem" \
	--secret "id=runtime_ca_bundle,src=$runtime_ca_dir/runtime-ca-secret.pem" \
	--build-arg "BUILD_CA_BUNDLE_DIGEST=$build_ca_digest" \
	--build-arg "RUNTIME_CA_BUNDLE_DIGEST=$runtime_ca_digest" \
	--build-arg "BUILD_REVISION=$contract_revision" \
	-t "$runtime_ca_image" \
	. >/dev/null

test "$(docker image inspect --format '{{.Architecture}}' "$runtime_ca_image")" = "amd64"
docker run -d --platform linux/amd64 --name "$runtime_ca_container" \
	-e APP_BIND_ALL=true -p 127.0.0.1::8080 "$runtime_ca_image" >/dev/null
docker cp "$runtime_ca_container:/app/portfolio-server" "$runtime_ca_dir/portfolio-server"
file "$runtime_ca_dir/portfolio-server" | grep -Eq 'x86-64|x86_64'

runtime_port=$(docker port "$runtime_ca_container" 8080/tcp | sed -n 's/^127\.0\.0\.1://p')
test -n "$runtime_port"
health_url="http://127.0.0.1:$runtime_port/healthz"
health_response="$runtime_ca_dir/health.json"
attempt=0
until curl -fsS -o "$health_response" "$health_url" 2>/dev/null; do
	attempt=$((attempt + 1))
	if [ "$attempt" -ge 100 ]; then
		echo "regular image did not become ready at $health_url" >&2
		docker logs "$runtime_ca_container" >&2
		exit 1
	fi
	sleep 0.1
done
actual_health=$(tr -d '\n' < "$health_response")
expected_health="{\"revision\":\"$contract_revision\",\"status\":\"ok\"}"
if [ "$actual_health" != "$expected_health" ]; then
	echo "regular health response = $actual_health, want $expected_health" >&2
	exit 1
fi

docker cp "$runtime_ca_container:/etc/ssl/certs/ca-certificates.crt" "$runtime_ca_dir/runtime-ca.pem"

openssl verify -CAfile "$runtime_ca_dir/runtime-ca.pem" "$runtime_ca_dir/runtime-only-ca.pem"
if openssl verify -CAfile "$runtime_ca_dir/runtime-ca.pem" \
	"$runtime_ca_dir/build-only-ca.pem" >/dev/null 2>&1; then
	echo "build-only CA unexpectedly present in the runtime trust store" >&2
	exit 1
fi

docker build --platform linux/amd64 --target builder \
	--secret "id=build_ca_bundle,src=$runtime_ca_dir/build-ca.pem" \
	--secret "id=runtime_ca_bundle,src=$runtime_ca_dir/runtime-ca-secret.pem" \
	--build-arg "BUILD_CA_BUNDLE_DIGEST=$build_ca_digest" \
	--build-arg "RUNTIME_CA_BUNDLE_DIGEST=$runtime_ca_digest" \
	--build-arg "BUILD_REVISION=$contract_revision" \
	--output "type=oci,dest=$runtime_ca_dir/builder.oci.tar,compression=uncompressed" \
	. >/dev/null

build_ca_payload=$(sed -n '2p' "$runtime_ca_dir/build-only-ca.pem")
if tar -xOf "$runtime_ca_dir/builder.oci.tar" | grep -a -F "$build_ca_payload" >/dev/null; then
	echo "build-only CA unexpectedly present in committed regular builder layers" >&2
	exit 1
fi
