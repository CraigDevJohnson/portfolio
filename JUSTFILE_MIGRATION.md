# Justfile Migration Guide

This project has transitioned from using GNU Make (`make`) to `just` as its command runner.

## Why `just`?

- **Simpler syntax**: No need for `.PHONY` targets or Make's idiosyncrasies
- **Cross-platform**: Works consistently on Linux, macOS, and Windows
- **Better error messages**: Clear, informative error reporting
- **Command arguments**: Recipes can accept parameters
- **Modern tool**: Designed for command running, not building

## Installation

### Quick Install (Cargo)
```bash
cargo install just
```

### Other Installation Methods

See the [official installation guide](https://github.com/casey/just#installation) for other methods including:
- Homebrew: `brew install just`
- apt: `apt install just` (Debian 13+ / Ubuntu 24.04+)
- Chocolatey: `choco install just` (Windows)
- Scoop: `scoop install just` (Windows)

## Command Equivalents

All commands remain the same, just replace `make` with `just`:

| Old Command (Make)        | New Command (Just)        |
|---------------------------|---------------------------|
| `make`                    | `just`                    |
| `make build`              | `just build`              |
| `make run`                | `just run`                |
| `make dev`                | `just dev`                |
| `make generate`           | `just generate`           |
| `make clean`              | `just clean`              |
| `make fmt`                | `just fmt`                |
| `make vet`                | `just vet`                |
| `make lint`               | `just lint`               |
| `make test`               | `just test`               |
| `make install-air`        | `just install-air`        |
| `make install-golangci-lint` | `just install-golangci-lint` |
| `make install-tools`      | `just install-tools`      |
| `make help`               | `just --list`             |

## Key Differences

1. **Help Command**: Use `just --list` to see all available recipes (instead of `make help`)
2. **No `.PHONY` needed**: `just` recipes are always executed
3. **Grouped recipes**: Recipes are organized into logical groups (build, run, test, etc.)
4. **Better defaults**: Variables like `PORT` can use `env_var_or_default()`

## Features

The justfile provides the same functionality as the Makefile:

- ✅ Templ component generation
- ✅ Go building and running
- ✅ Hot-reload development with air
- ✅ Code formatting and linting
- ✅ Test execution
- ✅ Tool installation helpers

## Backwards Compatibility

The `Makefile` is still present for backwards compatibility. You can use either:
- `just <recipe>` - New recommended approach
- `make <target>` - Legacy approach (still supported)

Eventually, the Makefile may be removed in favor of the justfile.
