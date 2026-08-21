# AWS Lambda and API Gateway

The Lambda deployment packages the Go HTTP application as a container image and
adapts API Gateway HTTP API requests through
`github.com/awslabs/aws-lambda-go-api-proxy`.

## Repository path

- `Dockerfile.lambda` builds the Lambda image.
- `cmd/lambda/main.go` starts the Lambda handler.
- `cmd/lambda/secrets.go` resolves configured SSM parameter paths during cold
  start.
- `internal/app.NewLambdaHandler` creates the same application routes without a
  listening TCP server.
- `infra/lambda.tf` declares the function, API Gateway integration, IAM, and
  runtime configuration.

The Lambda and App Runner resources share one ECR repository, two DynamoDB
tables, and one OpenTofu state. See `DEPLOY-INSTRUCTIONS.md` for backend,
region, secret, and first-deploy setup.

## Deploy

For the repository's first full infrastructure deployment, run:

```bash
task deploy
```

That command pushes both `latest` and `lambda-latest` before the full OpenTofu
apply. To create or refresh only the Lambda path after shared infrastructure
exists, use:

```bash
task deploy-lambda
```

For later Lambda image updates:

```bash
task redeploy-lambda
```

Read the API endpoint from OpenTofu:

```bash
cd infra
tofu output -raw lambda_api_url
```

## Runtime configuration

OpenTofu sets:

- `CLIENT_ID_KEY=/portfolio/CLIENT_ID_KEY`
- `CLIENT_SECRET_KEY=/portfolio/CLIENT_SECRET_KEY`
- `LPS_SESSION_KEY=/portfolio/LPS_SESSION_KEY`
- `GOOGLE_CONNECTION_TABLE_NAME` from the managed DynamoDB table
- `SOCCER_SESSION_TABLE_NAME` from the managed DynamoDB table
- JSON logging defaults

At cold start, `cmd/lambda/secrets.go` replaces the three SSM paths with
decrypted values. A missing or inaccessible configured parameter fails handler
startup. Create all three SecureString parameters before deployment.

The default Lambda configuration is 512 MB with a 30-second timeout. Change
`lambda_memory_mb` or `lambda_timeout_seconds` through OpenTofu variables, then
review `tofu plan` before applying.

## Google OAuth

Register the API Gateway HTTPS URL ending in `/soccer` as an authorized redirect
URI. The callback returns to the same route.

## EC2 management portal

The Terraform-managed Lambda path does not currently supply or resolve
`MGMT_*` values and does not attach EC2 or CloudWatch portal permissions.
Therefore, the optional management portal is not supported on this deployment
path without additional infrastructure work.

## Cost and behavior

Lambda charges by requests and execution duration, while API Gateway,
DynamoDB, logs, ECR storage, and data transfer can add charges. Use the current
[AWS Lambda pricing](https://aws.amazon.com/lambda/pricing/) and AWS Pricing
Calculator rather than a fixed estimate.

Cold starts can make the first request slower than the long-lived App Runner
service. Static assets are embedded in the container deployment and served by
the same handler.

## Verification

Before deployment, run the repository gates:

```bash
task generate
task fmt
task lint
task vet
task test
task build
```

After deployment, verify at least:

```text
GET /
GET /soccer
GET /static/css/tailwind.css
```

If cold start fails, check CloudWatch Logs and confirm the function role can
read each configured SSM path and decrypt its KMS key.
