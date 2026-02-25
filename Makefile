.PHONY: help build test test-unit test-e2e test-coverage clean install fmt lint

help:
	@echo "Available commands:"
	@echo "  make build              - Build the nvair binary"
	@echo "  make install            - Install nvair binary to GOPATH/bin"
	@echo "  make test               - Run all tests (unit + integration)"
	@echo "  make test-unit          - Run unit tests with coverage (-short -tags excluded)"
	@echo "  make test-e2e           - Run E2E tests only (-tags=e2e)"
	@echo "  make test-coverage      - Generate HTML coverage report (coverage.html)"
	@echo "  make fmt                - Format code with gofmt"
	@echo "  make lint               - Run linter (golangci-lint)"
	@echo "  make clean              - Remove build artifacts"

build:
	@echo "Building nvair..."
	go build -o bin/nvair ./cmd/nvair

install:
	@echo "Installing nvair..."
	go install ./cmd/nvair

test:
	@echo "Running all tests..."
	go test -v ./...

test-unit:
	@echo "Running unit tests with coverage..."
	go test ./... -short -coverprofile=coverage.out
	@echo ""
	@echo "Coverage summary:"
	go tool cover -func=coverage.out | tail -1

test-e2e: build
	@echo "Running E2E tests..."
	go test -v ./e2e -tags=e2e

test-coverage:
	@echo "Generating coverage report (unit tests only)..."
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

fmt:
	@echo "Formatting code..."
	go fmt ./...

lint:
	@echo "Running linter..."
	golangci-lint run

clean:
	@echo "Cleaning up..."
	rm -f bin/nvair
	rm -f coverage.out coverage.html
	go clean

# Development helper targets
dev: fmt build test
	@echo "Development build complete"

run: build
	./bin/nvair help

# CI/CD target - runs all checks
ci: fmt lint test-coverage
	@echo "All CI checks passed!"
