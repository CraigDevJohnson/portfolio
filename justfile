# Portfolio Server justfile

# Default values
BINARY := "portfolio-server"
GO := "go"
GOFLAGS := ""

# Set shell for Windows compatibility
# On Windows, ensure you have Git Bash, WSL, or another POSIX shell in PATH
set windows-shell := ["sh", "-cu"]

# Default recipe (runs when you just run 'just')
default: build

# Generate Templ components
[group('build')]
generate: templ

# Check and generate Templ components
[group('build')]
templ:
    @command -v templ >/dev/null 2>&1 || { echo "templ not installed. Run 'go install github.com/a-h/templ/cmd/templ@latest' to install it."; exit 1; }
    templ generate

# Build the binary
[group('build')]
build: generate
    {{GO}} build {{GOFLAGS}} -o {{BINARY}} .

# Build and run the server
[group('run')]
run: build
    ./{{BINARY}}

# Run with air for hot-reload development
[group('run')]
dev:
    @command -v air >/dev/null 2>&1 || { echo "air not installed. Run 'just install-air' to install it automatically."; exit 1; }
    air

# Install air for hot-reload development
[group('tools')]
install-air:
    @echo "Installing air for hot-reload development..."
    {{GO}} install github.com/air-verse/air@latest
    @echo "air installed successfully!"

# Remove binary and clean cached files
[group('clean')]
clean:
    rm -f {{BINARY}}
    {{GO}} clean

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
    @command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not installed. Run 'just install-golangci-lint' to install it automatically."; exit 1; }
    golangci-lint run

# Install golangci-lint for linting
[group('tools')]
install-golangci-lint:
    @echo "Installing golangci-lint for linting..."
    {{GO}} install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
    @echo "golangci-lint installed successfully!"

# Install all development tools
[group('tools')]
install-tools: install-air install-golangci-lint
    @echo "All development tools installed successfully!"

# Run tests
[group('test')]
test:
    {{GO}} test -v ./...

# Show this help message
[group('help')]
help:
    @just --list
