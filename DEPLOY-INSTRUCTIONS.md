# Deployment instructions

The repository deploys container images from one Amazon ECR repository to both
AWS App Runner and AWS Lambda with API Gateway. OpenTofu manages both runtimes,
their shared DynamoDB tables, and their IAM roles in one state.

> [!IMPORTANT]
> AWS App Runner is closed to new customers. Existing customers can continue
> using it. New AWS accounts should use the Lambda path or choose another
> container service. See the
> [AWS App Runner availability change](https://docs.aws.amazon.com/apprunner/latest/dg/apprunner-availability-change.html).

## Current infrastructure

`infra/*.tf` is the source of truth. A full apply manages:

- one ECR repository with mutable `latest` and `lambda-latest` tags;
- an App Runner service using `latest`;
- a Lambda container function and API Gateway HTTP API using `lambda-latest`;
- DynamoDB tables for Google connections and imported Soccer session baselines;
- App Runner and Lambda IAM roles for DynamoDB and SSM Parameter Store access.

The default AWS region is `us-west-2`. The S3 backend in
`infra/versions.tf` is pinned to Craig's existing state bucket and key. Change
that backend before `tofu init` if you are deploying from another AWS account.

AWS charges vary by region and traffic. Check the current
[App Runner pricing](https://aws.amazon.com/apprunner/pricing/) and
[Lambda pricing](https://aws.amazon.com/lambda/pricing/) instead of relying on
a fixed monthly estimate.

## Prerequisites

Install and configure:

- Docker;
- AWS CLI v2;
- OpenTofu 1.6 or newer;
- Task;
- AWS credentials that can manage ECR, App Runner, Lambda, API Gateway,
  DynamoDB, IAM, SSM Parameter Store, KMS, and the configured S3 state backend.

Verify the local tools and identity:

```bash
docker version
aws --version
tofu --version
task --version
aws sts get-caller-identity
```

The OpenTofu default is `us-west-2`. Set `AWS_PROFILE` when you do not want the
AWS CLI's default profile.

## Configure runtime secrets

With the default `app_name = "portfolio"`, both deployed runtimes expect these
SecureString parameters:

- `/portfolio/LPS_SESSION_KEY`
- `/portfolio/CLIENT_ID_KEY`
- `/portfolio/CLIENT_SECRET_KEY`

`LPS_SESSION_KEY` must be a 64-character hexadecimal value. Create or
update the parameters in the infrastructure region before the first full apply:

```bash
AWS_REGION=us-west-2
APP_NAME=portfolio

aws ssm put-parameter \
  --region "$AWS_REGION" \
  --name "/$APP_NAME/LPS_SESSION_KEY" \
  --type SecureString \
  --value "$(openssl rand -hex 32)" \
  --overwrite

aws ssm put-parameter \
  --region "$AWS_REGION" \
  --name "/$APP_NAME/CLIENT_ID_KEY" \
  --type SecureString \
  --value "YOUR_GOOGLE_CLIENT_ID" \
  --overwrite

aws ssm put-parameter \
  --region "$AWS_REGION" \
  --name "/$APP_NAME/CLIENT_SECRET_KEY" \
  --type SecureString \
  --value "YOUR_GOOGLE_CLIENT_SECRET" \
  --overwrite
```

The infrastructure supplies `GOOGLE_CONNECTION_TABLE_NAME` and
`SOCCER_SESSION_TABLE_NAME`. Do not duplicate those values in SSM.

Register each deployed `/soccer` URL in the Google OAuth client. Include the
App Runner custom domain, the API Gateway URL if Lambda is used directly, and
`http://localhost:8080/soccer` for local testing.

## First deployment

From the repository root:

```bash
task deploy
```

The task performs these steps:

1. checks AWS, Docker, and OpenTofu access;
2. creates the ECR repository and lifecycle policy if needed;
3. builds and pushes the App Runner image as `latest`;
4. builds and pushes the Lambda image as `lambda-latest`;
5. runs a full `tofu apply --auto-approve`.

Both images must exist before the full apply because the same OpenTofu state
declares both runtimes.

Inspect the deployed endpoints:

```bash
cd infra
tofu output -raw app_runner_service_url
tofu output -raw lambda_api_url
```

Open each URL and confirm the home page loads. Confirm `/soccer` separately if
you configured Google OAuth and Soccer authentication.

## Updates

Use the runtime-specific update command after the first deployment:

```bash
# Push latest and trigger App Runner.
task redeploy

# Push lambda-latest and update the Lambda function.
task redeploy-lambda
```

`task deploy-lambda` is a targeted first-deploy helper for Lambda resources. It
does not replace a full infrastructure reconciliation with `task deploy`.

## EC2 management portal

The portal routes are disabled unless the session key, Cognito domain, and
client ID are valid. A working sign-in also requires a registered redirect URI.
Local mock review uses:

```bash
task portal-preview
```

The current OpenTofu files do not provision Cognito, portal IAM permissions, or
`MGMT_*` runtime values. To enable the portal on App Runner, configure these
values in the service runtime and add least-privilege permissions to the App
Runner instance role:

- `MGMT_SESSION_KEY`
- `MGMT_COGNITO_DOMAIN`
- `MGMT_COGNITO_CLIENT_ID`
- `MGMT_COGNITO_REDIRECT_URI`, required for sign-in and registered with Cognito
- `MGMT_COGNITO_LOGOUT_URI`, optional
- `MGMT_AWS_REGION`, defaults to `us-east-1`

Required AWS actions are:

- `ec2:DescribeInstances`
- `ec2:StartInstances`
- `ec2:StopInstances`
- `cloudwatch:GetMetricStatistics`
- `logs:FilterLogEvents`

The Terraform-managed Lambda deployment does not pass or resolve `MGMT_*`
values, so the portal is not currently supported on that path.

## App Runner custom domain

This section applies only to accounts that already have App Runner access.

Associate the domain:

```bash
SERVICE_ARN=$(cd infra && tofu output -raw app_runner_service_arn)

aws apprunner associate-custom-domain \
  --service-arn "$SERVICE_ARN" \
  --domain-name craigdevjohnson.com \
  --enable-www-subdomain
```

Use the DNS target and certificate-validation records returned by AWS. Do not
copy a region-specific hostname from an example. In Cloudflare, keep the AWS
certificate-validation records set to DNS only. Keep those records after
activation so ACM can renew the certificate.

Check status with:

```bash
aws apprunner describe-custom-domains --service-arn "$SERVICE_ARN"
```

AWS says activation can take up to 24 to 48 hours. See
[Managing App Runner custom domains](https://docs.aws.amazon.com/apprunner/latest/dg/manage-custom-domains.html).

## Teardown

Disassociate an App Runner custom domain before destroying its service:

```bash
SERVICE_ARN=$(cd infra && tofu output -raw app_runner_service_arn)

aws apprunner disassociate-custom-domain \
  --service-arn "$SERVICE_ARN" \
  --domain-name craigdevjohnson.com
```

Then review the destroy plan:

```bash
cd infra
tofu plan -destroy
tofu destroy
```

The ECR repository uses `force_delete = false`. OpenTofu will refuse to delete
it while images remain. Emptying ECR is a separate destructive decision; do
not change that guard or delete images without confirming the exact repository
and recovery impact.

## Troubleshooting

### Image not found

Run `task deploy`, not a bare full `tofu apply`, for the first deployment. The
full state needs both `latest` and `lambda-latest` in ECR.

### App Runner does not become healthy

The service expects an `amd64` container listening on port `8080`. Reproduce the
runtime locally:

```bash
docker build --platform linux/amd64 -t portfolio .
docker run --rm -e APP_BIND_ALL=true -p 8080:8080 portfolio
```

Then inspect App Runner operations:

```bash
SERVICE_ARN=$(cd infra && tofu output -raw app_runner_service_arn)
aws apprunner list-operations --service-arn "$SERVICE_ARN"
```

Follow application logs from the service's CloudWatch Logs group with
`task logs`.

### Lambda fails during cold start

Confirm all three SSM parameter paths exist in the configured `aws_region`
(`us-west-2` by default) and the Lambda role can call `ssm:GetParameters` and
`kms:Decrypt`. The cold-start resolver treats a missing configured parameter as
an error.

### ECR login expired

Derive the region and registry from OpenTofu instead of hard-coding them:

```bash
ECR_URL=$(cd infra && tofu output -raw ecr_repository_url)
AWS_REGION=$(echo "$ECR_URL" | sed 's/.*\.ecr\.\(.*\)\.amazonaws\.com.*/\1/')
ECR_REGISTRY=${ECR_URL%%/*}

aws ecr get-login-password --region "$AWS_REGION" | \
  docker login --username AWS --password-stdin "$ECR_REGISTRY"
```
