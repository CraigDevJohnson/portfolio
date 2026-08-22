# AWS Lambda and API Gateway

> [!WARNING]
> The checked-in `infra/` directory and all existing deployment commands target
> the legacy shared App Runner and Lambda state. They do not create the pending
> replacement environment. Do not deploy the new release to App Runner.

The Lambda image runs the same Go HTTP handler as the regular server. The
`aws-lambda-go-api-proxy` adapter converts API Gateway HTTP API events into
`net/http` requests.

Shared ECR, OpenTofu, secret, deploy, update, and teardown instructions live in
[`DEPLOY-INSTRUCTIONS.md`](../../DEPLOY-INSTRUCTIONS.md). This page covers only
the Lambda path.

## Code path

- `Dockerfile.lambda` builds the image.
- `cmd/lambda/main.go` initializes the API Gateway adapter.
- `cmd/lambda/secrets.go` resolves configured SSM parameter paths during cold
  start.
- `internal/app.NewLambdaHandler` constructs the application routes without a
  TCP listener.
- `infra/lambda.tf` declares the legacy shared-stack Lambda, API Gateway, IAM,
  and runtime settings.

The legacy Lambda and App Runner deployments share an ECR repository, DynamoDB
tables, and OpenTofu state.

## Legacy deployment and rollback reference

`task deploy` and `task redeploy` operate on the legacy shared stack, including
App Runner. `task deploy-lambda` can bootstrap the legacy targeted Lambda path
by creating the ECR repository and initializing OpenTofu first, but it does not
reconcile the full shared infrastructure. `task redeploy-lambda` updates that
legacy function. Preserve these commands for rollback; do not use any of them
to deploy the replacement release.

Read the API endpoint after deployment:

```bash
cd infra
tofu output -raw lambda_api_url
```

## Runtime behavior

OpenTofu passes SSM paths through `CLIENT_ID_KEY`, `CLIENT_SECRET_KEY`, and
`LPS_SESSION_KEY` in the legacy stack. During cold start,
`cmd/lambda/secrets.go` fetches every path-valued setting in one decrypted
`GetParameters` call. It validates the complete response, including missing,
invalid, and unusable values, before it replaces any environment value. A
failed or partial response leaves the original configuration unchanged and
fails handler initialization.

The full cold-start path has an eight-second bound. That window covers SSM
resolution and application construction. The process constructs the API
Gateway proxy once before starting Lambda and reuses it for warm invocations.

The adapter requires API Gateway's typed V2 request context and reads its
domain name as the trusted host with an `https` scheme. Generated callback URLs
and cookie security use this context-backed origin. Client-controlled `Host`
and forwarding headers cannot override it, and a missing typed gateway domain
fails closed before application routing.

OpenTofu also sets the managed Google connection and Soccer import-baseline
table names. The legacy shared Terraform defaults to 512 MB and a 30-second
timeout. `lambda_memory_mb` and `lambda_timeout_seconds` control those legacy
values.

The pending replacement environment has a 29-second deployment contract. A
later environment plan must implement that setting; no replacement environment
is defined or live in this repository. The application's Google Calendar add
and result-sync handlers each use a 24-second child context, leaving five
seconds outside their work budget under the pending timeout. If a deadline ends
a multi-game batch, the response reports the completed work counts and
recommends a retry. Retries match the existing Google game ID and update
completed events instead of inserting duplicates.

Register the API Gateway HTTPS URL ending in `/soccer` as a Google OAuth
redirect URI. Google returns the callback to that same route.

The Lambda resources do not pass `MGMT_*` settings or grant the EC2 and
CloudWatch permissions used by the optional management portal. The portal is
therefore unavailable on the managed Lambda path.

## Local image build and verification

The supported readiness tasks build and inspect local Linux amd64 images:

```bash
task build-image
task build-lambda-image
task test-images
```

The two build tasks pass the current full Git SHA as `BUILD_REVISION` by
default, or inject the caller's supplied `BUILD_REVISION`. Comparing `/healthz`
with that exact expected value can prove the identity of those artifacts.

They do not log in to ECR, push images, apply OpenTofu, or update a running
service.

## Verify locally and for legacy rollback

Run the repository gate before any approved deployment:

```bash
task ci
```

After a legacy rollback deployment, verify these paths:

```text
GET /healthz
GET /
GET /soccer
GET /static/css/tailwind.css
```

`GET /healthz` returns `application/json` with the configured,
linker-injected revision in
`{"revision":"<build revision>","status":"ok"}` and `Cache-Control:
no-store`. The handler does not probe SSM, DynamoDB, Google, or Soccer during
request handling. Legacy deployment helpers and direct builds that omit
`BUILD_REVISION` may report `development`. Do not use that value as immutable
provenance proof.

For a legacy cold-start failure, inspect that function's CloudWatch Logs.
Confirm its role can read each configured SSM parameter and decrypt its KMS key.
