# Accumulate Network Test Suite Documentation

## Overview

The Accumulate Network test suite is a comprehensive testing framework designed to ensure the reliability, performance, and security of the blockchain network. This documentation provides complete guidance for running, understanding, and maintaining the test suite.

## Quick Start

```bash
# Clone and setup
git clone <repository>
cd accumulate
go mod download

# Run all tests
make test

# Run specific test categories
make test-unit          # Fast unit tests
make test-e2e           # End-to-end integration tests
make test-performance   # Load and benchmark tests
```

## Documentation Structure

This documentation is organized into focused sections:

### Core Documentation
- [readme.md](readme.md) - This overview and navigation guide
- [index.md](index.md) - **Master index with keywords, cross-references, and AI guidance**
- [test-content.md](test-content.md) - Complete test suite catalog
- [testing.md](testing.md) - Comprehensive testing guide (400+ lines)

### Specialized Testing Guides
- [unit-tests.md](unit-tests.md) - Unit testing guide with examples and best practices
- [e2e-tests.md](e2e-tests.md) - End-to-end testing with simulator framework
- [performance-tests.md](performance-tests.md) - Performance testing, benchmarking, and optimization
- [simulator-tests.md](simulator-tests.md) - Network simulation and consensus testing

### Development and Maintenance Guides
- [debugging.md](debugging.md) - Test debugging techniques and troubleshooting
- [ci-cd.md](ci-cd.md) - CI/CD pipeline integration and automation
- [test-maintenance.md](test-maintenance.md) - Test suite maintenance and optimization
- [ai-guidance.md](ai-guidance.md) - **AI assistant patterns, templates, and decision trees**

## Test Categories Overview

### 1. Unit Tests
**Purpose**: Test individual components in isolation  
**Location**: Throughout the codebase (`*_test.go` files)  
**Runtime**: Fast (< 1 minute)  
**When to Run**: During development, before commits

```bash
go test ./internal/... ./pkg/...
```

### 2. End-to-End (E2E) Tests
**Purpose**: Test complete workflows and system integration  
**Location**: `test/e2e/`  
**Runtime**: Moderate (5-30 minutes)  
**When to Run**: Before merging, release validation

```bash
go test ./test/e2e/...
```

### 3. Performance Tests
**Purpose**: Validate system performance and capacity  
**Location**: `test/cmd/load/`, benchmark tests  
**Runtime**: Variable (1-60 minutes)  
**When to Run**: Performance validation, capacity planning

```bash
go run ./test/cmd/load/main.go
```

### 4. Simulator Tests
**Purpose**: Test complex network scenarios in controlled environment  
**Location**: `test/simulator/`  
**Runtime**: Variable (1-10 minutes)  
**When to Run**: Network behavior validation, consensus testing

```bash
go test ./test/simulator/...
```

## Prerequisites

### System Requirements
- **Go**: Version 1.21 or later
- **Memory**: Minimum 8GB RAM (16GB recommended for full test suite)
- **Disk**: 10GB free space for test data and artifacts
- **Network**: Internet connection for dependency downloads

### Development Tools
- **Make**: For build automation
- **Git**: For version control
- **Docker**: For containerized testing (optional)

### Environment Setup
```bash
# Verify Go installation
go version

# Install dependencies
go mod download

# Verify test environment
go test -short ./pkg/url/...
```

## Test Execution Patterns

### Standard Go Testing
```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./test/e2e/...

# Run specific test
go test -run TestSendTokens ./test/e2e/...

# Run with timeout
go test -timeout=30m ./test/e2e/...
```

### Make Targets (Recommended)
```bash
# Quick validation
make test-unit

# Full validation
make test-all

# Performance testing
make test-performance

# Generate coverage report
make test-coverage
```

### VS Code Integration
The repository includes comprehensive VS Code launch configurations:
- Debug individual tests
- Run test suites with debugging
- Profile performance tests
- Validate network configurations

See `.vscode/launch.json` for available configurations.

## Test Data and Fixtures

### Test Data Location
- **Static Data**: `test/testdata/`
- **Generated Data**: `test/data/`
- **Benchmark Data**: `test/data/pkg_database_values_BenchmarkNotFound/`

### Test Data Management
```bash
# Generate test data
go run ./test/cmd/gen-testdata/

# Clean test artifacts
make clean-test-data
```

## Common Test Scenarios

### Development Workflow
1. **Write Code**: Implement new features
2. **Unit Tests**: `make test-unit`
3. **Integration Tests**: `make test-e2e`
4. **Performance Check**: `make test-performance` (if relevant)
5. **Commit**: After all tests pass

### Release Validation
1. **Full Test Suite**: `make test-all`
2. **Performance Regression**: `make test-benchmark`
3. **Load Testing**: Extended performance validation
4. **Network Testing**: Multi-node scenario validation

### Debugging Workflow
1. **Reproduce Issue**: Run specific failing test
2. **Debug Mode**: Use VS Code launch configurations
3. **Logging**: Enable verbose output and debug logging
4. **Isolation**: Run minimal test case to isolate problem

## Performance Considerations

### Test Execution Time
- **Unit Tests**: ~30 seconds
- **E2E Tests**: ~15 minutes
- **Full Suite**: ~45 minutes
- **Load Tests**: Variable (5-60 minutes)

### Resource Usage
- **Memory**: 2-8GB during test execution
- **CPU**: High utilization during parallel test execution
- **Disk**: Temporary files and test databases

### Optimization Tips
```bash
# Run tests in parallel
go test -parallel=4 ./...

# Skip slow tests during development
go test -short ./...

# Run specific test categories
go test ./test/e2e/txn_*_test.go
```

## Test Result Interpretation

### Success Indicators
- All tests pass with `PASS` status
- No race conditions detected
- Performance metrics within acceptable ranges
- Coverage targets met

### Failure Analysis
- **Test Failures**: Check test output for specific assertion failures
- **Timeouts**: May indicate performance issues or deadlocks
- **Race Conditions**: Run with `-race` flag for detection
- **Resource Issues**: Monitor memory and CPU usage

### Coverage Analysis
```bash
# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

## Troubleshooting

### Common Issues
1. **Port Conflicts**: Tests may conflict with running services
2. **Database Locks**: Concurrent test execution issues
3. **Memory Exhaustion**: Large test suites consuming available memory
4. **Network Timeouts**: Slow network affecting network-dependent tests

### Debug Commands
```bash
# Verbose test output
go test -v ./test/e2e/...

# Race condition detection
go test -race ./...

# Memory profiling
go test -memprofile=mem.prof ./...

# CPU profiling
go test -cpuprofile=cpu.prof ./...
```

## Next Steps

1. **Read the Complete Guide**: [testing.md](testing.md)
2. **Choose Your Test Type**: See individual test type documentation
3. **Set Up Your Environment**: Follow the setup instructions
4. **Run Your First Tests**: Start with unit tests
5. **Explore Advanced Features**: Debugging, performance testing, CI/CD

## Getting Help

- **Documentation**: Read the specific test type guides
- **Code Examples**: Check existing test files for patterns
- **VS Code**: Use the provided launch configurations
- **Issues**: Check for known issues and solutions

---

*This documentation is maintained alongside the test suite. Please update it when adding new test types or changing test procedures.*
