# Portfolio Server Makefile

BINARY := portfolio-server
GO := go
GOFLAGS :=
PORT ?= 8080
TAILWIND := ./bin/tailwindcss
TAILWIND_VERSION := v4.1.18
TAILWIND_URL := https://github.com/tailwindlabs/tailwindcss/releases/download/$(TAILWIND_VERSION)/tailwindcss-linux-x64
TAILWIND_SHA256 := 737becf8d4ad1115ea98df69fa94026d402ca8feb91306a035b5b004167da8dd

.PHONY: all build run clean fmt lint vet test install-air install-golangci-lint install-tools help generate templ build-css watch-css install-tailwind

all: build

generate: templ

templ:
	@command -v templ >/dev/null 2>&1 || { echo "templ not installed. Run 'go install github.com/a-h/templ/cmd/templ@latest' to install it."; exit 1; }
	templ generate

install-tailwind:
	@if [ ! -f $(TAILWIND) ]; then \
		echo "Downloading Tailwind CSS $(TAILWIND_VERSION)..."; \
		mkdir -p bin; \
		if ! curl -fSL $(TAILWIND_URL) -o $(TAILWIND); then \
			echo "Error: Failed to download Tailwind CSS binary"; \
			rm -f $(TAILWIND); \
			exit 1; \
		fi; \
		echo "Verifying download integrity..."; \
		echo "$(TAILWIND_SHA256)  $(TAILWIND)" | sha256sum -c - || { \
			echo "Error: SHA256 verification failed. Downloaded file may be corrupt or tampered."; \
			rm -f $(TAILWIND); \
			exit 1; \
		}; \
		chmod +x $(TAILWIND); \
		echo "Tailwind CSS installed successfully at $(TAILWIND)"; \
	else \
		echo "Tailwind CSS already installed at $(TAILWIND)"; \
	fi

build-css: install-tailwind
	$(TAILWIND) -i ./static/css/tailwind.css -o ./static/css/output.css

watch-css: install-tailwind
	$(TAILWIND) -i ./static/css/tailwind.css -o ./static/css/output.css --watch

build: build-css generate
	$(GO) build $(GOFLAGS) -o $(BINARY) .

run: build
	./$(BINARY)

dev:
	@command -v air >/dev/null 2>&1 || { echo "air not installed. Run 'make install-air' to install it automatically."; exit 1; }
	@echo "Starting development server with hot reload..."
	@echo "Note: Run 'make watch-css' in a separate terminal for CSS hot reload"
	air

install-air:
	@echo "Installing air for hot-reload development..."
	$(GO) install github.com/air-verse/air@latest
	@echo "air installed successfully!"

clean:
	rm -f $(BINARY)
	rm -f static/css/output.css
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
	@echo "  build                 Compile CSS and server binary"
	@echo "  build-css             Build Tailwind CSS output"
	@echo "  watch-css             Watch and rebuild CSS on changes"
	@echo "  install-tailwind      Download standalone Tailwind CSS binary"
	@echo "  run                   Build and run the server"
	@echo "  dev                   Run with air for hot-reload (run 'make watch-css' separately)"
	@echo "  clean                 Remove binary and cached files"
	@echo "  fmt                   Format Go source files"
	@echo "  vet                   Run go vet"
	@echo "  lint                  Run vet and golangci-lint"
	@echo "  test                  Run tests"
	@echo "  install-air           Install air for hot-reload development"
	@echo "  install-golangci-lint Install golangci-lint for linting"
	@echo "  install-tools         Install all development tools"
	@echo "  help                  Show this help message"
