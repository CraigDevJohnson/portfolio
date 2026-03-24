# Portfolio Server justfile
# Set shell for Windows

set windows-shell := ["powershell.exe", "-NoLogo", "-Command"]

# Default values - use .exe extension on Windows

BINARY := if os() == "windows" { "portfolio-server.exe" } else { "portfolio-server" }
GO := "go"
GOFLAGS := ""

# Default recipe (runs when you just run 'just')
default: build

# Generate Templ components
[group('build')]
generate: templ

# Check and generate Templ components
[group('build')]
templ:
  {{ GO }} tool templ generate

# Build the binary
[group('build')]
build: generate
    {{ GO }} build {{ GOFLAGS }} -o {{ BINARY }} .

# Build and run the server
[group('run')]
run: build
    {{ if os() == "windows" { ".\\" + BINARY } else { "./" + BINARY } }}

# Run with Docker Compose
[group('run')]
compose:
    docker compose -f docker-compose.yml up -d --build portfolio

# Remove binary and clean cached files
[group('clean')]
clean:
    {{ if os() == "windows" { "Remove-Item -Force " + BINARY + " -ErrorAction SilentlyContinue" } else { "rm -f " + BINARY } }}
    {{ GO }} clean

# Format Go source files
[group('quality')]
fmt:
    golangci-lint fmt --config .golangci.toml

# Run go vet
[group('quality')]
vet:
    {{ GO }} vet ./...

# Lint Go source files and CSS files
[group('quality')]
lint:
    #!/usr/bin/env sh
    set -eu
    if ! command -v golangci-lint >/dev/null 2>&1 || golangci-lint version --short 2>/dev/null | grep -qE '^[0-1]'; then
      echo "golangci-lint v2+ not found -- please run 'just install-lint' to install it"
      exit 1
    fi
    golangci-lint run --config .golangci.toml --fix
    if ! npm list stylelint --depth=0 >/dev/null 2>&1; then
      echo "stylelint not found -- please run 'just install-lint' to install it"
      exit 1
    fi
    npx stylelint "**/*.css" --fix

# Check quality, run tests and build the binary
[group('quality')]
ci: fmt vet lint test build

# Run with air for hot-reload development
[group('run')]
dev:
    air

# Install air for hot-reload development
[group('tools')]
install-air:
    @echo "Installing air for hot-reload development..."
    {{ GO }} install github.com/air-verse/air@latest
    @echo "air installed successfully!"

# Install golangci-lint v2 for linting
[group('tools')]
install-lint:
    #!/usr/bin/env sh
    set -eu
    echo "Checking golangci-lint..."
    if ! command -v golangci-lint >/dev/null 2>&1 || golangci-lint version --short 2>/dev/null | grep -qE '^[0-1]'; then
      echo "golangci-lint not found or version is older than v2 -- installing latest (v2+)..."
      curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b "$(go env GOPATH)/bin" latest
    fi
    echo "golangci-lint is installed and up to date!"
    echo "Checking stylelint..."
    if ! command -v npx >/dev/null 2>&1; then
      echo "npx not found -- please install Node.js and npm"
      exit 1
    fi
    if ! npx stylelint --version >/dev/null 2>&1; then
      echo "stylelint not found -- installing..."
      npm install stylelint stylelint-config-standard --save-dev
    fi
    echo "stylelint is installed and ready to use!"

# Install all development tools
[group('tools')]
install-tools: install-air install-lint
    @echo "All development tools installed successfully!"

# Run tests
[group('test')]
test:
    {{ GO }} test -v ./...

# Show this help message
[group('help')]
help:
    @just --list

[group('deploy')]
deploy:
    #!/usr/bin/env sh
    set -eu
    # Verify prerequisites
    for cmd in tofu aws docker; do
      if ! command -v "$cmd" >/dev/null 2>&1; then
        echo "$cmd not found -- see DEPLOY-INSTRUCTIONS.md for installation steps"
        exit 1
      fi
    done
    if ! aws sts get-caller-identity >/dev/null 2>&1; then
      echo "AWS credentials not configured -- run 'aws configure' or export AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY"
      exit 1
    fi
    cd infra
    # Initialize if needed
    if [ ! -d .terraform ]; then
      tofu init
    fi
    # Create ECR repo first (App Runner needs an image to exist)
    tofu apply -target=aws_ecr_repository.app -target=aws_ecr_lifecycle_policy.app --auto-approve
    ECR_URL=$(tofu output -raw ecr_repository_url)
    AWS_REGION=$(echo "$ECR_URL" | sed 's/.*\.ecr\.\(.*\)\.amazonaws\.com.*/\1/')
    AWS_ACCOUNT_ID=$(echo "$ECR_URL" | sed 's/\..*//')
    # Authenticate Docker with ECR
    aws ecr get-login-password --region "$AWS_REGION" | \
      docker login --username AWS --password-stdin \
        "$AWS_ACCOUNT_ID.dkr.ecr.$AWS_REGION.amazonaws.com"
    cd ..
    # Build, tag, and push the Docker image
    docker build --platform linux/amd64 -t portfolio .
    docker tag portfolio:latest "$ECR_URL:latest"
    docker push "$ECR_URL:latest"
    # Apply full infrastructure (App Runner can now reference the image)
    cd infra
    tofu apply --auto-approve
    echo "Deployment complete!"
    tofu output app_runner_service_url

[group('deploy')]
redeploy:
    #!/usr/bin/env sh
    set -eu
    # Derive ECR URL and region from Terraform state
    ECR_URL=$(cd infra && tofu output -raw ecr_repository_url)
    AWS_REGION=$(echo "$ECR_URL" | sed 's/.*\.ecr\.\(.*\)\.amazonaws\.com.*/\1/')
    AWS_ACCOUNT_ID=$(echo "$ECR_URL" | sed 's/\..*//')
    # Authenticate Docker with ECR
    aws ecr get-login-password --region "$AWS_REGION" | \
      docker login --username AWS --password-stdin \
        "$AWS_ACCOUNT_ID.dkr.ecr.$AWS_REGION.amazonaws.com"
    # Build, tag, and push
    docker build --platform linux/amd64 -t portfolio .
    docker tag portfolio:latest "$ECR_URL:latest"
    docker push "$ECR_URL:latest"
    # Trigger App Runner redeployment
    SERVICE_ARN=$(cd infra && tofu output -raw app_runner_service_arn)
    aws apprunner start-deployment --service-arn "$SERVICE_ARN"
    echo "Redeployment triggered. Check status with:"
    echo "  aws apprunner describe-service --service-arn $SERVICE_ARN --query Service.Status"
