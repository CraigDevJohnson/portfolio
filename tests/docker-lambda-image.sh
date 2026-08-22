#!/bin/sh
set -eu

image="portfolio-lambda-contract:$$"
container="portfolio-lambda-contract-$$"
tmp_dir=$(mktemp -d)
contract_revision=0123456789abcdef0123456789abcdef01234567
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
	--build-arg "BUILD_REVISION=$contract_revision" \
	-f Dockerfile.lambda -t "$image" .

test "$(docker image inspect --format '{{.Architecture}}' "$image")" = "amd64"
docker run -d --platform linux/amd64 --name "$container" \
	-p 127.0.0.1::8080 "$image" >/dev/null
docker cp "$container:/var/task/bootstrap" "$tmp_dir/bootstrap"
file "$tmp_dir/bootstrap" | grep -Eq 'x86-64|x86_64'

runtime_port=$(docker port "$container" 8080/tcp | sed -n 's/^127\.0\.0\.1://p')
test -n "$runtime_port"
invocation_url="http://127.0.0.1:$runtime_port/2015-03-31/functions/function/invocations"
invocation_response="$tmp_dir/invocation.json"
gateway_event='{"version":"2.0","rawPath":"/healthz","headers":{"host":"contract.example"},"requestContext":{"domainName":"contract.example","http":{"method":"GET","path":"/healthz","protocol":"HTTP/1.1","sourceIp":"127.0.0.1"}}}'
attempt=0
until curl -fsS -X POST -H 'Content-Type: application/json' \
	--data "$gateway_event" -o "$invocation_response" "$invocation_url" 2>/dev/null; do
	attempt=$((attempt + 1))
	if [ "$attempt" -ge 100 ]; then
		echo "Lambda image did not become ready at $invocation_url" >&2
		docker logs "$container" >&2
		exit 1
	fi
	sleep 0.1
done
expected_lambda_body="\"body\":\"{\\\"revision\\\":\\\"$contract_revision\\\",\\\"status\\\":\\\"ok\\\"}\\n\""
if ! grep -F -q '"statusCode":200' "$invocation_response" \
	|| ! grep -F -q "$expected_lambda_body" "$invocation_response"; then
	echo "Lambda invocation response did not expose revision $contract_revision:" >&2
	sed -n '1p' "$invocation_response" >&2
	exit 1
fi

docker cp -L "$container:/etc/pki/tls/certs/ca-bundle.crt" "$tmp_dir/runtime-ca.pem"
build_ca_payload=$(sed -n '2p' "$tmp_dir/build-only-ca.pem")
if grep -F -q "$build_ca_payload" "$tmp_dir/runtime-ca.pem"; then
	echo "build-only CA unexpectedly present in the Lambda runtime image" >&2
	exit 1
fi

docker build --platform linux/amd64 --target builder \
	--secret "id=build_ca_bundle,src=$tmp_dir/build-ca.pem" \
	--build-arg "BUILD_CA_BUNDLE_DIGEST=$build_ca_digest" \
	--build-arg "BUILD_REVISION=$contract_revision" \
	--output "type=oci,dest=$tmp_dir/builder.oci.tar,compression=uncompressed" \
	-f Dockerfile.lambda . >/dev/null

if tar -xOf "$tmp_dir/builder.oci.tar" | grep -a -F "$build_ca_payload" >/dev/null; then
	echo "build-only CA unexpectedly present in committed Lambda builder layers" >&2
	exit 1
fi
