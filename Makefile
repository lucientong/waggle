.PHONY: all build test lint cover fmt vet clean tidy

# Default target
all: fmt vet test build

# Build the CLI binary
build:
	go build -o bin/waggle ./cmd/waggle/...

# Run all tests
test:
	go test ./...

# Run tests with short flag (skip slow tests)
test-short:
	go test -short ./...

# Run tests with verbose output
test-v:
	go test -v ./...

# Run tests with race detector
test-race:
	go test -race ./...

# Generate coverage report
cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report written to coverage.html"

# Show coverage in terminal
cover-func:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

# Format source code
fmt:
	gofmt -w .

# Run go vet
vet:
	go vet ./...

# Run staticcheck linter (requires: go install honnef.co/go/tools/cmd/staticcheck@latest)
lint:
	staticcheck ./...

# Tidy go.mod and go.sum
tidy:
	go mod tidy

# Clean build artifacts
clean:
	rm -rf bin/ coverage.out coverage.html

# Install CLI to $GOPATH/bin
install:
	go install ./cmd/waggle/...

# Run example: code review
example-code-review:
	go run ./examples/code_review/...

# Run example: research assistant
example-research:
	go run ./examples/research/...

# Run example: customer support
example-customer-support:
	go run ./examples/customer_support/...
