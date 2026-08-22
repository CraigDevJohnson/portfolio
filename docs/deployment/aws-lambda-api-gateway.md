# AWS Lambda and API Gateway

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
- `infra/lambda.tf` declares Lambda, API Gateway, IAM, and runtime settings.

The Lambda and App Runner deployments share an ECR repository, DynamoDB tables,
and OpenTofu state.

## Deploy and inspect

Use `task deploy` for the first complete shared infrastructure deployment.
`task deploy-lambda` can bootstrap the targeted Lambda path by creating the ECR
repository and initializing OpenTofu first, but it does not reconcile the full
shared infrastructure. Later image updates use `task redeploy-lambda`.

Read the API endpoint after deployment:

```bash
cd infra
tofu output -raw lambda_api_url
```

## Runtime behavior

OpenTofu passes SSM paths through `CLIENT_ID_KEY`, `CLIENT_SECRET_KEY`, and
`LPS_SESSION_KEY`. During cold start, `cmd/lambda/secrets.go` replaces those
paths with decrypted values before application configuration loads. A missing
or inaccessible configured parameter fails handler initialization.

OpenTofu also sets the managed Google connection and Soccer import-baseline
table names. The default function configuration is 512 MB with a 30-second timeout.
`lambda_memory_mb` and `lambda_timeout_seconds` control those values.

Register the API Gateway HTTPS URL ending in `/soccer` as a Google OAuth
redirect URI. Google returns the callback to that same route.

The Lambda resources do not pass `MGMT_*` settings or grant the EC2 and
CloudWatch permissions used by the optional management portal. The portal is
therefore unavailable on the managed Lambda path.

## Verify

Run the repository gate before deployment:

```bash
task ci
```

After deployment, verify these paths:

```text
GET /
GET /soccer
GET /static/css/tailwind.css
```

If cold start fails, inspect CloudWatch Logs. Confirm the function role can read
each configured SSM parameter and decrypt its KMS key.
