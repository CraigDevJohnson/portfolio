# Portfolio Server Makefile

BINARY := portfolio-server
GO := go
GOFLAGS :=
PORT ?= 8080

.PHONY: all build run clean fmt lint vet test install-air install-staticcheck install-tools help

all: build

build:
	$(GO) build $(GOFLAGS) -o $(BINARY) .

run: build
	./$(BINARY)

dev:
	@command -v air >/dev/null 2>&1 || { echo "air not installed. Run 'make install-air' to install it automatically."; exit 1; }
	air

install-air:
	@echo "Installing air for hot-reload development..."
	$(GO) install github.com/air-verse/air@latest
	@echo "air installed successfully!"

clean:
	rm -f $(BINARY)
	$(GO) clean

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

lint: vet
	@command -v staticcheck >/dev/null 2>&1 || { echo "staticcheck not installed. Run 'make install-staticcheck' to install it automatically."; exit 1; }
	staticcheck ./...

install-staticcheck:
	@echo "Installing staticcheck for linting..."
	$(GO) install honnef.co/go/tools/cmd/staticcheck@latest
	@echo "staticcheck installed successfully!"

install-tools: install-air install-staticcheck
	@echo "All development tools installed successfully!"

test:
	$(GO) test -v ./...

help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  all                Build the binary (default)"
	@echo "  build              Compile the server binary"
	@echo "  run                Build and run the server"
	@echo "  dev                Run with air for hot-reload"
	@echo "  clean              Remove binary and cached files"
	@echo "  fmt                Format Go source files"
	@echo "  vet                Run go vet"
	@echo "  lint               Run vet and staticcheck"
	@echo "  test               Run tests"
	@echo "  install-air        Install air for hot-reload development"
	@echo "  install-staticcheck Install staticcheck for linting"
	@echo "  install-tools      Install all development tools"
	@echo "  help               Show this help message"
