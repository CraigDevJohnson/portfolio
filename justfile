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
    rm -f {{BINARY}}
    {{GO}} clean
    -{{ if os() == "windows" { "Remove-Item -Force {{BINARY}} -ErrorAction SilentlyContinue" } else { "rm -f {{BINARY}}" } }}
# Format Go source files
[group('quality')]
fmt:
    {{GO}} fmt ./...

# Run go vet
[group('quality')]
vet:
    {{GO}} vet ./...

# Run vet and golangci-lint
[group('quality')]
lint: vet
    golangci-lint run

# Install golangci-lint for linting
[group('tools')]
install-golangci-lint:
    @echo "Installing golangci-lint for linting..."
    {{GO}} install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
    @echo "golangci-lint installed successfully!"

# Install all development tools
[group('tools')]
install-tools: install-air install-golangci-lint install-templ
    @echo "All development tools installed successfully!"

# Run tests
[group('test')]
test:
    {{GO}} test -v ./...

# Show this help message
[group('help')]
help:
    @just --list
