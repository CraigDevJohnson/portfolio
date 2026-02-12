# Portfolio Server Makefile

BINARY := portfolio-server
GO := go
GOFLAGS :=
PORT ?= 8080

.PHONY: all build run clean fmt lint vet test install-air install-golangci-lint install-tools help generate templ

all: build

generate: templ

templ:
	@TEMPL_BIN="$$($(GO) env GOPATH)/bin/templ"; \
	if [ ! -f "$$TEMPL_BIN" ]; then \
		echo "templ not installed. Run 'go install github.com/a-h/templ/cmd/templ@latest' to install it."; \
		exit 1; \
	fi; \
	"$$TEMPL_BIN" generate

build: generate
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
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not installed. Run 'make install-golangci-lint' to install it automatically."; exit 1; }
	golangci-lint run

install-golangci-lint:
	@echo "Installing golangci-lint for linting..."
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "golangci-lint installed successfully!"

install-tools: install-air install-golangci-lint
	@echo "All development tools installed successfully!"

test:
	$(GO) test -v ./...

help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  all                   Build the binary (default)"
	@echo "  build                 Compile the server binary"
	@echo "  run                   Build and run the server"
	@echo "  dev                   Run with air for hot-reload"
	@echo "  clean                 Remove binary and cached files"
	@echo "  fmt                   Format Go source files"
	@echo "  vet                   Run go vet"
	@echo "  lint                  Run vet and golangci-lint"
	@echo "  test                  Run tests"
	@echo "  install-air           Install air for hot-reload development"
	@echo "  install-golangci-lint Install golangci-lint for linting"
	@echo "  install-tools         Install all development tools"
	@echo "  help                  Show this help message"
