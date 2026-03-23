# Load Generator Testing Guide

## Overview

The load generator includes comprehensive unit and integration tests to validate functionality.

## Running Unit Tests

Unit tests can run without a network connection:

```bash
go test -v -short
```

These tests validate:
- Account creation and uniqueness
- Lite token address generation
- Client structure
- Default configuration
- Transaction load calculations
- Dataset logging

## Running Integration Tests

Integration tests require a running Accumulate devnet with faucet service enabled.

### Prerequisites

1. Start a test devnet:
```bash
# Create test configuration (3 BVNs, 4 validators each)
devnet config create test-integration --bvns 3 --validators 4

# Start the devnet
devnet start test-integration
```

2. Set environment variables:
```bash
export INTEGRATION_TEST=1
export ACC_API=http://127.0.0.1:26660/v2
```

3. Run integration tests:
```bash
go test -v -run TestLoadGeneratorIntegration
```

### Integration Test Coverage

The integration tests validate:
- Single account fauceting
- Multiple client initialization
- Minimal load test execution
- Transaction submission and confirmation
- Network connectivity

## Test Requirements

### Unit Tests
- No external dependencies
- Run in <1 second
- Cover core logic and data structures

### Integration Tests
- Running Accumulate devnet
- Faucet service enabled
- API accessible on configured port
- Complete in <30 seconds

## Troubleshooting

### Common Issues

**"Server not available"**
- Ensure devnet is running: `ps aux | grep accumulated`
- Check API port: `netstat -an | grep 26660`
- Verify network config: `devnet config list`

**"Faucet failed: no live peers"**
- Faucet service not running on devnet
- Restart devnet with proper configuration
- Check devnet logs for faucet service status

**"Divide by zero" panic**
- API connection failed during initialization
- Check ACC_API environment variable
- Verify network describe endpoint responds

## CI/CD Integration

For CI pipelines:

```bash
# Run unit tests only (fast)
go test -short -v ./...

# Run all tests with devnet
devnet start test-integration
export INTEGRATION_TEST=1
export ACC_API=http://127.0.0.1:26660/v2
go test -v ./...
devnet stop test-integration
```

## Test Data

Tests generate temporary data in:
- `t.TempDir()` for unit tests
- `load_tester/` directory for integration tests

All test data is automatically cleaned up after test completion.
