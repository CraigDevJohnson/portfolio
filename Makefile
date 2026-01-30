# Portfolio Server Makefile

BINARY := portfolio-server
GO := go
GOFLAGS :=
PORT ?= 8080

.PHONY: all build run clean fmt lint vet test help

all: build

build:
	$(GO) build $(GOFLAGS) -o $(BINARY) .

run: build
	./$(BINARY)

dev:
	@command -v air >/dev/null 2>&1 || { echo "air not installed: go install github.com/air-verse/air@latest"; exit 1; }
	air

clean:
	rm -f $(BINARY)
	$(GO) clean

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

lint: vet
	@command -v staticcheck >/dev/null 2>&1 || { echo "staticcheck not installed: go install honnef.co/go/tools/cmd/staticcheck@latest"; exit 1; }
	staticcheck ./...

test:
	$(GO) test -v ./...

help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  all      Build the binary (default)"
	@echo "  build    Compile the server binary"
	@echo "  run      Build and run the server"
	@echo "  dev      Run with air for hot-reload"
	@echo "  clean    Remove binary and cached files"
	@echo "  fmt      Format Go source files"
	@echo "  vet      Run go vet"
	@echo "  lint     Run vet and staticcheck"
	@echo "  test     Run tests"
	@echo "  help     Show this help message"
