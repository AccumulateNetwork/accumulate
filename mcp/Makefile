.PHONY: build test test-integration clean install

# Build the MCP server
build:
	go build -o mcp-server ./cmd/mcp-server

# Run unit tests
test:
	go test -v ./...

# Run integration tests (requires network access)
test-integration:
	go test -v -tags=integration ./...

# Run all tests with coverage
test-cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Clean build artifacts
clean:
	rm -f mcp-server
	rm -f coverage.out coverage.html

# Install to GOPATH/bin
install:
	go install ./cmd/mcp-server

# Run linter
lint:
	golangci-lint run ./...

# Format code
fmt:
	gofmt -s -w .

# Check if code is formatted
check-fmt:
	@test -z "$$(gofmt -l .)" || (echo "Code is not formatted. Run 'make fmt'" && exit 1)
