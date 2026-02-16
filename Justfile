# Portfolio Server justfile

# Set shell for Windows
set windows-shell := ["powershell.exe", "-NoLogo", "-Command"]

# Default values - use .exe extension on Windows
BINARY := if os() == "windows" { "portfolio-server.exe" } else { "portfolio-server" }
GO := "go"
GOFLAGS := ""
# Use Go-installed templ binary (avoids PATH conflict with .cargo/bin/templ on Windows)
TEMPL := if os() == "windows" { `go env GOPATH` + "\\bin\\templ.exe" } else { "templ" }

# Default recipe (runs when you just run 'just')
default: build

# Generate Templ components
[group('build')]
generate: templ

# Check and generate Templ components
[group('build')]
templ:
  {{TEMPL}} generate

# Build the binary
[group('build')]
build: generate
  {{GO}} build {{GOFLAGS}} -o {{BINARY}} .

# Build and run the server
[group('run')]
run: build
  {{ if os() == "windows" { ".\\{{BINARY}}" } else { "./{{BINARY}}" } }}

# Run with Docker Compose
[group('run')]
compose:
  docker compose -f docker-compose.yml up -d --build portfolio

# Run with air for hot-reload development
[group('run')]
dev:
  air

# Install air for hot-reload development
[group('tools')]
install-air:
  @echo "Installing air for hot-reload development..."
  {{GO}} install github.com/air-verse/air@latest
  @echo "air installed successfully!"

# Install templ for code generation
[group('tools')]
install-templ:
  @echo "Installing templ for code generation..."
  {{GO}} install github.com/a-h/templ/cmd/templ@latest
  @echo "templ installed successfully!"

# Remove binary and clean cached files
[group('clean')]
clean:
  {{ if os() == "windows" { "Remove-Item -Force {{BINARY}} -ErrorAction SilentlyContinue" } else { "rm -f {{BINARY}}" } }}
  {{GO}} clean

# Format Go source files
[group('quality')]
fmt:
  golangci-lint fmt ./...

# Run go vet
[group('quality')]
vet:
  {{GO}} vet ./...

# Run vet and golangci-lint
[group('quality')]
lint:
  golangci-lint run

[group('quality')]
check: fmt vet lint test build

# Install golangci-lint v2 for linting
[group('tools')]
install-lint:
  #!/usr/bin/env sh
  set -eu
  echo "Checking golangci-lint..."
  if ! command -v golangci-lint >/dev/null 2>&1 || ! golangci-lint --version 2>/dev/null | grep -qE 'version[[:space:]]v?2'; then
    echo "golangci-lint not found or version is older than v2 -- installing latest (v2+)..."
    curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $(go env GOPATH)/bin latest
  fi
  echo "golangci-lint is installed and up to date!"

# Install all development tools
[group('tools')]
install-tools: install-air install-lint install-templ
  @echo "All development tools installed successfully!"

# Run tests
[group('test')]
test:
  {{GO}} test -v ./...

# Show this help message
[group('help')]
help:
  @just --list
