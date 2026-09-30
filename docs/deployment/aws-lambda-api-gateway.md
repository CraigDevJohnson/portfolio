# AWS Lambda and API Gateway

The Lambda image runs the same Go HTTP handler as the regular server. The
`aws-lambda-go-api-proxy` adapter converts API Gateway HTTP API events into
`net/http` requests.

Accounts, roots, approvals, the release workflow and rollback are described in
[`DEPLOY-INSTRUCTIONS.md`](../../DEPLOY-INSTRUCTIONS.md). DNS records are in the
[Cloudflare runbook](cloudflare-dns.md). This page covers the Lambda path itself.

## Code path

- `Dockerfile.lambda` builds the image.
- `cmd/lambda/main.go` initializes the API Gateway adapter.
- `cmd/lambda/secrets.go` resolves configured SSM parameter paths during cold
  start.
- `internal/app.NewLambdaHandler` constructs the application routes without a
  TCP listener.
- `infra/lambda/modules/service/` declares the Lambda, API Gateway, IAM, data,
  domain, log, and alarm resources for one environment.
- `infra/lambda/environments/dev/` and `infra/lambda/environments/prod/` call
  that module with their own state; `infra/lambda/artifacts/` owns the shared
  release repository; `infra/lambda/ci-roles/` owns the CI roles, the execution
  boundary and the state bucket.

## Per-environment resources

| Resource | Name |
| --- | --- |
| Lambda (container, x86_64, 512 MB, 29 s) | `portfolio-lambda-<env>`, alias `live` |
| HTTP API | `portfolio-lambda-<env>-http`, `$default` route and stage |
| DynamoDB | `portfolio-lambda-<env>-google-connections`, `portfolio-lambda-<env>-soccer-sessions` |
| Execution role | `portfolio-lambda-<env>-execution`, inside `PortfolioLambdaExecutionBoundary` |
| Logs | `/aws/lambda/portfolio-lambda-<env>`, `/aws/apigateway/portfolio-lambda-<env>/access` |
| Alarms | `portfolio-lambda-<env>-{lambda-errors,lambda-throttles,lambda-duration,api-5xx,api-latency}` |
| Certificate and domains | ACM certificate and regional API Gateway custom domains for the environment's hostnames |

The service module can also plan the durable Soccer history table,
`portfolio-lambda-<env>-soccer-history`, with `enable_soccer_history`. Both
environments leave it off until the issue #80 activation review. Turning it on
also needs the table added to `PortfolioLambdaExecutionBoundary` and to the CI
roles' table-read grant in `infra/lambda/ci-roles/`.

Every resource carries the lowercase `project = portfolio` tag through provider
`default_tags`.

## Runtime behavior

The environment roots pass environment-owned SSM paths through
`CLIENT_ID_KEY`, `CLIENT_SECRET_KEY`, and `LPS_SESSION_KEY`. During cold start,
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

OpenTofu also sets the environment-owned Google connection and Soccer
import-baseline table names. The application's Google Calendar add and
result-sync handlers each use a 24-second child context, leaving five seconds of
the 29-second Lambda timeout outside their work budget. If a deadline ends a
multi-game batch, the response reports the completed work counts and recommends
a retry. Retries match the existing Google game ID and update completed events
instead of inserting duplicates.

Register each environment's HTTPS URL ending in `/soccer` as a Google OAuth
redirect URI (`oauth_redirect_uris` output). Google returns the callback to that
same route.

The Lambda resources pass no `SITE_*` settings, so neither site sign-in nor the
management portal that follows it is available on the Lambda path. The
development `management` input, which no environment sets, still controls the
portal's read-only IAM grants; the application ignores the retired `MGMT_*`
identity values it would also pass.

## Verify an environment

Run the repository gate before any deployment:

```bash
task ci
```

After a deployment, the Release workflow's `scripts/verify-lambda-release.sh`
checks these routes through the environment's API Gateway custom domain:

```text
GET /healthz
GET /
GET /soccer
GET /static/css/tailwind.css
GET /static/images/backgrounds/home-hero.jpg
```

`GET /healthz` returns `application/json` with the configured,
linker-injected revision in `{"revision":"<build revision>","status":"ok"}` and
`Cache-Control: no-store`. The handler does not probe SSM, DynamoDB, Google, or
Soccer during request handling. Direct builds that omit `BUILD_REVISION` may
report `development`; do not use that value as provenance.

For a cold-start failure, inspect the function's CloudWatch Logs. Confirm its
role can read each configured SSM parameter and decrypt through SSM.

## Local image build and verification

```bash
task build-image
task build-lambda-image
task test-images
```

The two build tasks pass the current full Git SHA as `BUILD_REVISION` by
default, or inject the caller's supplied `BUILD_REVISION`. They do not log in to
ECR, push images, apply OpenTofu, or update a running service.
