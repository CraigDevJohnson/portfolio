# Deployment Instructions

This guide walks you through deploying the portfolio site to **AWS App Runner** using
**OpenTofu** for infrastructure and **Amazon ECR** for container images.

---

## Table of Contents

1. [Why App Runner?](#why-app-runner)
2. [Prerequisites](#prerequisites)
3. [Cost Estimate](#cost-estimate)
4. [Step 1 — Install Tools](#step-1--install-tools)
5. [Step 2 — Configure AWS Credentials](#step-2--configure-aws-credentials)
6. [Step 3 — Deploy Infrastructure with OpenTofu](#step-3--deploy-infrastructure-with-opentofu)
7. [Step 4 — Build and Push the Docker Image](#step-4--build-and-push-the-docker-image)
8. [Step 5 — Deploy to App Runner](#step-5--deploy-to-app-runner)
9. [Step 6 — Configure CloudFlare DNS](#step-6--configure-cloudflare-dns)
10. [Updating the Site](#updating-the-site)
11. [Future Integrations](#future-integrations)
12. [Tearing Down](#tearing-down)
13. [Troubleshooting](#troubleshooting)

---

## Why App Runner?

| Factor | App Runner | ECS Fargate | Lightsail Containers | EC2 |
|---|---|---|---|---|
| **Monthly cost (< 100 visits)** | **~$5–7** | ~$10–15 | $7 (fixed) | $3–8 |
| **Setup complexity** | Very low | Medium | Low | High |
| **Auto-scaling** | ✅ Built-in | Manual config | ❌ | ❌ |
| **TLS/HTTPS** | ✅ Automatic | Manual (ALB+ACM) | ✅ Automatic | Manual |
| **Custom domain** | ✅ Built-in | Manual (Route53/ALB) | ✅ Built-in | Manual |
| **IAM role support** | ✅ Instance role | ✅ Task role | ❌ | ✅ Instance profile |
| **DynamoDB/SES/Lambda ready** | ✅ Via instance role | ✅ Via task role | ❌ Limited | ✅ Via instance profile |

**App Runner wins** for this use case because:

- **Cheapest for low traffic** — you pay per compute-second; an idle site costs almost nothing
  beyond the minimum.
- **Zero ops** — no load balancers, VPCs, or security groups to manage.
- **Auto TLS** — HTTPS is provided automatically on the `*.awsapprunner.com` domain and on
  custom domains.
- **Future-proof** — the instance IAM role lets you add DynamoDB, SES, and Lambda access by
  simply attaching policies (no architecture changes needed).

---

## Prerequisites

Before you begin, make sure you have:

- An **AWS account** with admin access (or at least permission to create IAM roles, ECR repos,
  and App Runner services).
- **Docker** installed and running — [Install Docker](https://docs.docker.com/get-docker/).
- **AWS CLI v2** installed — [Install AWS CLI](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html).
- **OpenTofu** installed (see [Step 1](#step-1--install-tools) below).
- A **CloudFlare account** managing DNS for your domain.

---

## Cost Estimate

For a site with fewer than 100 visits per month:

| Resource | Monthly Cost |
|---|---|
| App Runner (0.25 vCPU / 512 MB, minimal traffic) | ~$5.00 |
| ECR storage (< 500 MB image) | ~$0.05 |
| Data transfer (< 1 GB) | Free tier |
| **Total** | **~$5/month** |

> **Note:** App Runner charges per compute-second when handling requests. An idle service with
> near-zero traffic costs approximately $5/month for the provisioned minimum. This is
> significantly cheaper than running an always-on ECS task or EC2 instance.

---

## Step 1 — Install Tools

### OpenTofu

OpenTofu is an open-source fork of Terraform. Install it for your platform:

**macOS (Homebrew):**

```bash
brew install opentofu
```

**Linux (Debian/Ubuntu):**

```bash
curl -fsSL https://get.opentofu.org/install-opentofu.sh -o install-opentofu.sh
chmod +x install-opentofu.sh
./install-opentofu.sh --install-method deb
rm install-opentofu.sh
```

**Windows (Chocolatey):**

```powershell
choco install opentofu
```

Verify the installation:

```bash
tofu --version
```

### AWS CLI v2

If not already installed:

```bash
# macOS
brew install awscli

# Linux
curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "awscliv2.zip"
unzip awscliv2.zip && sudo ./aws/install && rm -rf aws awscliv2.zip

# Verify
aws --version
```

---

## Step 2 — Configure AWS Credentials

Set up your AWS credentials so both the AWS CLI and OpenTofu can authenticate:

```bash
aws configure
```

You will be prompted for:

| Prompt | What to enter |
|---|---|
| AWS Access Key ID | Your IAM access key |
| AWS Secret Access Key | Your IAM secret key |
| Default region name | `us-east-1` (or your preferred region) |
| Default output format | `json` |

> **Tip:** If you use AWS SSO, run `aws sso login --profile your-profile` and set
> `export AWS_PROFILE=your-profile` instead.

Verify your credentials:

```bash
aws sts get-caller-identity
```

You should see your account ID, user ARN, and user ID.

---

## Step 3 — Deploy Infrastructure with OpenTofu

The `infra/` directory contains all the OpenTofu configuration files:

| File | Purpose |
|---|---|
| `versions.tf` | Provider and backend configuration |
| `variables.tf` | Input variables (region, app name, CPU/memory) |
| `main.tf` | ECR repository, IAM roles, App Runner service |
| `outputs.tf` | Outputs (ECR URL, App Runner URL, etc.) |

### 3a. Initialize OpenTofu

```bash
cd infra
tofu init
```

This downloads the AWS provider plugin. You should see:

```
OpenTofu has been successfully initialized!
```

### 3b. Preview the changes

```bash
tofu plan
```

Review the output. It should show the creation of:

- 1 ECR repository
- 1 ECR lifecycle policy
- 2 IAM roles (ECR access + instance role)
- 1 IAM role policy attachment
- 1 App Runner service

### 3c. Build and push the Docker image first

**Important:** The App Runner service references the ECR image, so the image must exist in ECR
before `tofu apply` can succeed. You have two options:

**Option A — Create ECR first, then apply everything:**

```bash
# Create just the ECR repository
tofu apply -target=aws_ecr_repository.app -target=aws_ecr_lifecycle_policy.app

# Now build and push the image (see Step 4 below)
# ...

# Then apply the rest
tofu apply
```

**Option B — Apply all at once** (requires the image to already be in ECR):

If you've already pushed an image, simply run:

```bash
tofu apply
```

### 3d. Apply the infrastructure

When prompted, type `yes` to confirm:

```bash
tofu apply
```

After a few minutes, you'll see the outputs:

```
Outputs:

app_runner_service_url = "https://xxxxxxxxxx.us-east-1.awsapprunner.com"
ecr_repository_url     = "123456789012.dkr.ecr.us-east-1.amazonaws.com/portfolio"
instance_role_arn      = "arn:aws:iam::123456789012:role/portfolio-apprunner-instance"
```

Save the `ecr_repository_url` — you'll need it in the next step.

---

## Step 4 — Build and Push the Docker Image

### 4a. Authenticate Docker with ECR

```bash
# Get your AWS account ID
AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
AWS_REGION="us-east-1"  # Change if using a different region

# Log in to ECR
aws ecr get-login-password --region $AWS_REGION | \
  docker login --username AWS --password-stdin \
  $AWS_ACCOUNT_ID.dkr.ecr.$AWS_REGION.amazonaws.com
```

You should see `Login Succeeded`.

### 4b. Build the Docker image

From the **repository root** (not the `infra/` directory):

```bash
cd ..  # Back to repository root, if you were in infra/
docker build -t portfolio .
```

### 4c. Tag and push to ECR

```bash
# Tag the image for ECR
ECR_URL="$AWS_ACCOUNT_ID.dkr.ecr.$AWS_REGION.amazonaws.com/portfolio"
docker tag portfolio:latest $ECR_URL:latest

# Push to ECR
docker push $ECR_URL:latest
```

---

## Step 5 — Deploy to App Runner

If you followed **Option A** in Step 3, the App Runner service was already created. If not,
return to the `infra/` directory and apply:

```bash
cd infra
tofu apply
```

### Verify the deployment

1. Get the service URL from the OpenTofu output:

   ```bash
   tofu output app_runner_service_url
   ```

2. Open the URL in your browser. You should see your portfolio site.

3. You can also verify in the AWS Console:
   - Go to **App Runner** > **Services** > **portfolio**
   - Check the status is **Running**
   - Click the default domain link to view the site

---

## Step 6 — Configure CloudFlare DNS

To point your custom domain (`craigdevjohnson.com`) at the App Runner service:

### 6a. Add a custom domain in App Runner

```bash
# Get the App Runner service ARN
SERVICE_ARN=$(cd infra && tofu output -raw app_runner_service_arn)

# Associate your custom domain
aws apprunner associate-custom-domain \
  --service-arn "$SERVICE_ARN" \
  --domain-name "craigdevjohnson.com" \
  --enable-www-subdomain
```

This returns **DNS validation records** you'll need in CloudFlare. Note the CNAME records
from the output.

### 6b. Add DNS records in CloudFlare

1. Log in to [CloudFlare Dashboard](https://dash.cloudflare.com/).
2. Select your domain (`craigdevjohnson.com`).
3. Go to **DNS** > **Records**.

**Add the validation CNAME records** (from the AWS CLI output above):

| Type | Name | Content | Proxy status |
|---|---|---|---|
| CNAME | `_xxxxxxxxxx.craigdevjohnson.com` | `_yyyyyyyyyy.acm-validations.aws` | DNS only (grey cloud) |

> **Important:** Set proxy status to **DNS only** (grey cloud icon) for validation records.
> AWS needs to reach the CNAME directly.

**Add the domain CNAME record:**

| Type | Name | Content | Proxy status |
|---|---|---|---|
| CNAME | `@` (or `craigdevjohnson.com`) | `xxxxxxxxxx.us-east-1.awsapprunner.com` | DNS only (grey cloud) |
| CNAME | `www` | `xxxxxxxxxx.us-east-1.awsapprunner.com` | DNS only (grey cloud) |

> **Note:** If you're using CloudFlare's root CNAME flattening, the `@` CNAME record will
> work for the apex domain. CloudFlare automatically handles CNAME flattening at the zone
> apex.

### 6c. Wait for validation

DNS validation typically takes 5–30 minutes. Check the status:

```bash
aws apprunner describe-custom-domains --service-arn "$SERVICE_ARN"
```

Look for `"Status": "active"` on your domain entries.

### 6d. CloudFlare SSL/TLS settings

Since App Runner provides its own publicly trusted TLS certificate, configure CloudFlare's SSL mode:

1. In CloudFlare, go to **SSL/TLS** > **Overview**.
2. Set the encryption mode to **Full (strict)** so CloudFlare fully validates the App Runner
   ACM certificate while still terminating TLS at the edge.

> **Note:** If you keep the CloudFlare proxy enabled (orange cloud), you should use
> **Full (strict)** so CloudFlare validates the origin certificate. If you use **DNS only**
> (grey cloud), CloudFlare won't terminate TLS and the App Runner certificate handles
> everything directly between the browser and App Runner.

---

## Updating the Site

When you make changes to the site, redeploy with these steps:

```bash
# 1. Build the new Docker image
docker build -t portfolio .

# 2. Tag and push to ECR
AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
AWS_REGION="us-east-1"
ECR_URL="$AWS_ACCOUNT_ID.dkr.ecr.$AWS_REGION.amazonaws.com/portfolio"

docker tag portfolio:latest $ECR_URL:latest
docker push $ECR_URL:latest

# 3. Trigger a new deployment in App Runner
SERVICE_ARN=$(cd infra && tofu output -raw app_runner_service_arn)
aws apprunner start-deployment --service-arn "$SERVICE_ARN"
```

The deployment takes 2–3 minutes. Monitor progress:

```bash
aws apprunner describe-service --service-arn "$SERVICE_ARN" \
  --query 'Service.Status' --output text
```

Wait until the status is `RUNNING`.

> **Tip:** You can automate this with a GitHub Actions workflow. See [Future
> Integrations](#future-integrations) for a CI/CD example.

---

## Future Integrations

The infrastructure is designed to easily support the integrations mentioned in the issue.
Here's how to add each one:

### DynamoDB

1. Create a DynamoDB table in `infra/main.tf`:

   ```hcl
   resource "aws_dynamodb_table" "app_data" {
     name         = "${var.app_name}-data"
     billing_mode = "PAY_PER_REQUEST"  # No cost when idle
     hash_key     = "PK"
     range_key    = "SK"

     attribute {
       name = "PK"
       type = "S"
     }
     attribute {
       name = "SK"
       type = "S"
     }
   }
   ```

2. Attach a DynamoDB policy to the instance role:

   ```hcl
   resource "aws_iam_role_policy" "dynamodb_access" {
     name = "${var.app_name}-dynamodb-access"
     role = aws_iam_role.apprunner_instance.id

     policy = jsonencode({
       Version = "2012-10-17"
       Statement = [
         {
           Effect   = "Allow"
           Action   = ["dynamodb:GetItem", "dynamodb:PutItem", "dynamodb:Query", "dynamodb:Scan"]
           Resource = aws_dynamodb_table.app_data.arn
         }
       ]
     })
   }
   ```

3. Use the AWS SDK for Go in your application to interact with DynamoDB. The instance role
   credentials are automatically available.

### AWS SES (Email)

1. Attach an SES policy to the instance role:

   ```hcl
   resource "aws_iam_role_policy" "ses_access" {
     name = "${var.app_name}-ses-access"
     role = aws_iam_role.apprunner_instance.id

     policy = jsonencode({
       Version = "2012-10-17"
       Statement = [
         {
           Effect   = "Allow"
           Action   = ["ses:SendEmail", "ses:SendRawEmail"]
           Resource = [
             "arn:aws:ses:REGION:ACCOUNT_ID:identity/yourdomain.com",
             "arn:aws:ses:REGION:ACCOUNT_ID:identity/verified@example.com",
           ]
         }
       ]
     })
   }
   ```

2. Verify your domain or email address in the SES console.

3. Use the AWS SDK for Go to send emails from your contact form handler.

### Lambda Functions

1. Define Lambda functions in `infra/main.tf`:

   ```hcl
   resource "aws_lambda_function" "example" {
     function_name = "${var.app_name}-example"
     runtime       = "provided.al2023"
     handler       = "bootstrap"
     role          = aws_iam_role.lambda_exec.arn
     filename      = "lambda.zip"
   }
   ```

2. Invoke Lambda from your Go backend using the AWS SDK, or trigger it via API Gateway,
   DynamoDB streams, or SES receipt rules.

### CI/CD with GitHub Actions

Add a `.github/workflows/deploy.yml`:

```yaml
name: Deploy

on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    permissions:
      id-token: write
      contents: read

    steps:
      - uses: actions/checkout@v4

      - name: Configure AWS credentials
        uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: arn:aws:iam::YOUR_ACCOUNT_ID:role/github-actions-deploy
          aws-region: us-east-1

      - name: Login to ECR
        id: ecr-login
        uses: aws-actions/amazon-ecr-login@v2

      - name: Build, tag, and push image
        env:
          ECR_REGISTRY: ${{ steps.ecr-login.outputs.registry }}
        run: |
          docker build -t $ECR_REGISTRY/portfolio:latest .
          docker push $ECR_REGISTRY/portfolio:latest

      - name: Deploy to App Runner
        run: |
          SERVICE_ARN=$(aws apprunner list-services \
            --query "ServiceSummaryList[?ServiceName=='portfolio'].ServiceArn" \
            --output text)
          aws apprunner start-deployment --service-arn "$SERVICE_ARN"
```

> **Note:** You'll need to create an IAM role for GitHub Actions OIDC federation. See the
> [AWS docs on GitHub OIDC](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_providers_create_oidc.html).

---

## Tearing Down

To completely remove all AWS resources:

```bash
cd infra

# Remove the custom domain association first (if configured)
SERVICE_ARN=$(tofu output -raw app_runner_service_arn)
aws apprunner disassociate-custom-domain \
  --service-arn "$SERVICE_ARN" \
  --domain-name "craigdevjohnson.com"

# Destroy all infrastructure
tofu destroy
```

When prompted, type `yes` to confirm. This removes:

- The App Runner service
- The ECR repository (and all images)
- The IAM roles

> **Note:** Remove the CloudFlare DNS records manually after destroying the infrastructure.

---

## Troubleshooting

### App Runner service fails to start

**Check the App Runner logs:**

```bash
SERVICE_ARN=$(cd infra && tofu output -raw app_runner_service_arn)
aws apprunner list-operations --service-arn "$SERVICE_ARN"
```

**Common issues:**

- **Image not found in ECR** — make sure you pushed the image before running `tofu apply`.
- **Health check failing** — verify the app responds on port 8080 at `/`. Test locally with
  `docker run -p 8080:8080 portfolio` first.

### Docker build fails

```bash
# Make sure you're in the repository root (where the Dockerfile is)
docker build -t portfolio .

# Test the image locally
docker run -p 8080:8080 portfolio
# Visit http://localhost:8080 in your browser
```

### OpenTofu state issues

If you need to refresh state:

```bash
cd infra
tofu refresh
```

If a resource was manually deleted:

```bash
# Remove it from state
tofu state rm aws_apprunner_service.app

# Re-create it
tofu apply
```

### ECR login expired

ECR tokens expire after 12 hours. Re-authenticate:

```bash
AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
aws ecr get-login-password --region us-east-1 | \
  docker login --username AWS --password-stdin \
  $AWS_ACCOUNT_ID.dkr.ecr.us-east-1.amazonaws.com
```

### CloudFlare DNS not resolving

- Ensure validation CNAME records are set to **DNS only** (grey cloud), not proxied.
- Wait up to 30 minutes for DNS propagation.
- Check `aws apprunner describe-custom-domains --service-arn "$SERVICE_ARN"` for validation
  status.
