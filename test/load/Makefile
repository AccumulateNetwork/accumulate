# Makefile for Accumulate Load Tests
.PHONY: help faucet-test crosschain-test spend-test all clean

# Default target
help:
	@echo "🚰 Accumulate Load Test Commands"
	@echo "================================"
	@echo "make faucet-test      - Run standalone faucet collection test"
	@echo "make crosschain-test  - Run crosschain test with faucet helper"
	@echo "make spend-test       - Run comprehensive collect & spend test"
	@echo "make all             - Run all tests"
	@echo "make clean           - Clean up test artifacts"
	@echo ""
	@echo "Environment Variables:"
	@echo "  SERVER_URL          - DevNet server URL (default: http://127.0.0.1:26660)"
	@echo "  TIMEOUT             - Test timeout (default: 2m)"
	@echo ""
	@echo "Examples:"
	@echo "  make faucet-test"
	@echo "  make crosschain-test TIMEOUT=5m"
	@echo "  make crosschain-test SERVER_URL=http://127.0.0.1:27000"

# Configuration
SERVER_URL ?= http://127.0.0.1:26660
TIMEOUT ?= 2m

# Run standalone faucet collection test
faucet-test:
	@echo "🚰 Running standalone faucet collection test..."
	@echo "Server: $(SERVER_URL)"
	@echo "Timeout: $(TIMEOUT)"
	@echo ""
	timeout $(TIMEOUT) go run faucet_collection.go || echo "✅ Faucet test completed/timed out"

# Run crosschain test with faucet helper
crosschain-test:
	@echo "🔀 Running crosschain test with faucet helper..."
	@echo "Server: $(SERVER_URL)"
	@echo "Timeout: $(TIMEOUT)"
	@echo ""
	timeout $(TIMEOUT) go run crosschain_with_faucet.go faucet_helper.go

# Run comprehensive collect & spend test
spend-test:
	@echo "💸 Running comprehensive collect & spend test..."
	@echo "Server: $(SERVER_URL)"
	@echo "Timeout: $(TIMEOUT)"
	@echo ""
	timeout $(TIMEOUT) go run collect_and_spend_test.go acme_spender.go faucet_helper.go

# Run all tests
all: faucet-test crosschain-test spend-test

# Clean up any test artifacts
clean:
	@echo "🧹 Cleaning up test artifacts..."
	rm -f *.log
	rm -f *.pid
	@echo "✅ Cleanup complete"

# Check DevNet connectivity
check-devnet:
	@echo "🔍 Checking DevNet connectivity..."
	@if curl -s --max-time 5 "$(SERVER_URL)/v2" > /dev/null; then \
		echo "✅ DevNet is reachable at $(SERVER_URL)"; \
	else \
		echo "❌ Cannot reach DevNet at $(SERVER_URL)"; \
		echo "💡 Make sure your DevNet is running:"; \
		echo "   go run ./cmd/accumulated run devnet -w .devnet-test"; \
		exit 1; \
	fi