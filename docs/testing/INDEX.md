# Testing Documentation Index

[← Back to Main Index](../INDEX.md)

## Overview
Comprehensive testing documentation for Accumulate, including unit tests, integration tests, load tests, and DevNet testing.

## Quick Start
- [Testing README](readme.md) - General testing overview
- [Test Tiers](TEST_TIERS.md) - Test categorization and tiers
- [Running Tests](testing.md) - How to run tests

## Test Categories

### 🧪 Unit Testing
- [Unit Tests Guide](unit-tests.md) - Writing and running unit tests
- Related Code: [`test/`](../../test/) - Test implementations

### 🔄 Integration Testing
- [E2E Tests](e2e-tests.md) - End-to-end testing guide
- [E2E Test Examples](e2e2-example-test.md) - Example test implementations
- Related Code: [`test/e2e_v2/`](../../test/e2e_v2/) - E2E test suite

### 📊 Load Testing
- **[Load Testing Documentation](load/INDEX.md)** - Performance and load testing
  - [Gap Testing](load/GAP_TESTING_README.md) - Gap recovery testing
  - [Partition Failure Testing](load/PARTITION_FAILURE_DESIGN.md)
  - [Visual Testing Guide](load/HOW_TO_RUN_VISUAL_TESTS.md)

### 🌐 DevNet Testing
- **[DevNet Documentation](devnet/INDEX.md)** - Local development network
  - [DevNet Setup](devnet/devnet-setup.md) - Quick setup guide
  - [DevNet Configuration](devnet/DEVNET_CONFIGURATION.md) - Configuration options
  - [DevNet Design](devnet/DEVNET_DESIGN.md) - Architecture details
  - [DevNet Testing Guide](devnet/DEVNET_TESTING_GUIDE.md) - Testing workflows

### 🎯 Performance Testing
- [Performance Tests](performance-tests.md) - Performance benchmarking
- [Benchmark README](benchmark-readme.md) - Benchmark guidelines

### 🤖 Simulator Testing
- [Simulator Tests](simulator-tests.md) - Network simulation testing
- Related Tool: [`cmd/simulator/`](../../cmd/simulator/) - Simulator implementation

## Test Scripts

### DevNet Scripts
Located in [`scripts/devnet/`](../../scripts/devnet/):
- [`devnet_config.sh`](../../scripts/devnet/devnet_config.sh) - DevNet configuration
- [`devnet_manager.sh`](../../scripts/devnet/devnet_manager.sh) - DevNet management
- [`devnet_load_test.sh`](../../scripts/devnet/devnet_load_test.sh) - Load testing
- [`gap_recovery_demo.sh`](../../scripts/devnet/gap_recovery_demo.sh) - Gap recovery demo
- [`interactive_pause_test.sh`](../../scripts/devnet/interactive_pause_test.sh) - Interactive testing

### Test Runners
- [`run_full_test_suite.sh`](../../scripts/devnet/run_full_test_suite.sh) - Complete test suite
- [`quick_test.sh`](../../scripts/devnet/quick_test.sh) - Quick validation tests
- [`load_test_runner.sh`](../../scripts/devnet/load_test_runner.sh) - Load test orchestration

## Test Coverage & Reports

### Coverage Reports
- [Test Coverage Report](TEST_COVERAGE_REPORT.md) - Latest coverage metrics
- [Coverage Improvement Summary](COVERAGE_IMPROVEMENT_SUMMARY.md) - Coverage improvements
- [Test Improvements Summary](TEST_IMPROVEMENTS_SUMMARY.md) - Test enhancement tracking

### Test Analysis
- [Test Changes Review](TEST_CHANGES_REVIEW.md) - Recent test modifications
- [Pipeline Fix Summary](PIPELINE_FIX_SUMMARY.md) - CI/CD pipeline fixes

## CI/CD Integration
- [CI/CD Documentation](ci-cd.md) - Continuous integration setup
- GitLab Pipeline: [`.gitlab-ci.yml`](../../.gitlab-ci.yml)

## Debugging Tests
- [Debugging Guide](debugging.md) - Test debugging techniques
- [AI Guidance](ai-guidance.md) - AI-assisted testing

## Test Data
- [Test Data Index](test-data-index.md) - Test data organization
- [Test Content](test-content.md) - Test content guidelines
- Test Data Location: [`test/testdata/`](../../test/testdata/)

## Test Maintenance
- [Test Maintenance Guide](test-maintenance.md) - Keeping tests updated
- [API Server Testing](testing-api-server.md) - API-specific testing

## Running Tests

### Basic Commands
```bash
# Run all tests
go test ./...

# Run with coverage
go test -coverprofile=coverage.out ./...

# Run specific package tests
go test ./internal/core/execute/v2/crosschain/...

# Run with race detection
go test -race ./...

# Run benchmarks
go test -bench=. ./...
```

### DevNet Testing
```bash
# Start DevNet
./scripts/devnet/devnet_config.sh start 3 3 1

# Run load tests
./scripts/devnet/devnet_load_test.sh

# Interactive gap recovery testing
./scripts/devnet/interactive_pause_test.sh
```

### Coverage Analysis
```bash
# Generate coverage report
go test -coverprofile=coverage.out ./...

# View coverage in browser
go tool cover -html=coverage.out

# Get coverage percentage
go tool cover -func=coverage.out | grep total
```

## Test Organization

### Test Files
- `*_test.go` - Standard test files
- `*_bench_test.go` - Benchmark tests
- `example_test.go` - Example/documentation tests

### Test Helpers
- [`test/helpers/`](../../test/helpers/) - Shared test utilities
- [`test/testdata/`](../../test/testdata/) - Test fixtures

### Mocking
- Mock interfaces defined alongside implementations
- Example: [`internal/api/v2/mocks/`](../../internal/api/v2/mocks/)

## Best Practices

### 1. Test Naming
- Use descriptive test names: `TestConductor_GapRecovery_ResetsIndex`
- Group related tests: `TestConductor_*`

### 2. Test Structure
- Arrange-Act-Assert pattern
- Table-driven tests for multiple scenarios
- Subtests for logical grouping

### 3. Test Data
- Use fixtures for complex data
- Generate test data programmatically
- Clean up after tests

### 4. Performance
- Run expensive tests with `-short` flag check
- Use parallel tests where appropriate
- Benchmark critical paths

## Related Documentation

- [Development Process](../development/INDEX.md) - Development guidelines
- [Design Documentation](../design/INDEX.md) - System design
- [API Documentation](../api/INDEX.md) - API testing
- [Deployment](../deployment/INDEX.md) - Production testing