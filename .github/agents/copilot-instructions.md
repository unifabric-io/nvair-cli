# nvair-cli Development Guidelines

Auto-generated from all feature plans. Last updated: 2026-01-12

## Active Technologies

- Go 1.22+ + Cobra (CLI framework), Resty/net/http (HTTP client), golang.org/x/crypto/ssh (SSH client), go-pretty/table (table formatting) (001-nvair-cli)

## Project Structure

```text
cmd/nvair/
pkg/
test/
```

## Commands

```bash
# Build
go build -o ./bin/nvair ./cmd/nvair

# Test
go test ./...

# Lint
golangci-lint run
```

## Code Style

Go 1.22+: Follow standard Go conventions (gofmt, effective Go)

## Recent Changes

- 001-nvair-cli: Added Go 1.22+ + Cobra (CLI framework), Resty/net/http (HTTP client), golang.org/x/crypto/ssh (SSH client), go-pretty/table (table formatting)

<!-- MANUAL ADDITIONS START -->
<!-- MANUAL ADDITIONS END -->
