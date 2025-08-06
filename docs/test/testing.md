# Accumulate Network Testing Guide

## Table of Contents

1. [Overview](#overview)
2. [Quick Start](#quick-start)
3. [Environment Setup](#environment-setup)
4. [Test Categories](#test-categories)
5. [Running Tests](#running-tests)
6. [Makefile Targets](#makefile-targets)
7. [Test Configuration](#test-configuration)
8. [Debugging Tests](#debugging-tests)
9. [Performance Testing](#performance-testing)
10. [Continuous Integration](#continuous-integration)
11. [Troubleshooting](#troubleshooting)
12. [Best Practices](#best-practices)

## Overview

The Accumulate Network test suite provides comprehensive validation across all system components, from individual functions to complete network scenarios. This guide covers everything needed to run, debug, and maintain the test suite effectively.

### Test Philosophy

- **Comprehensive Coverage**: Unit, integration, end-to-end, and performance tests
- **Realistic Scenarios**: Tests mirror real-world usage patterns
- **Network Simulation**: Complete blockchain network simulation for complex scenarios
- **Performance Validation**: Load testing and benchmark suites
- **Regression Prevention**: Extensive regression test coverage

## Quick Start

### Prerequisites Check
```bash
# Verify Go version (1.21+ required)
go version

# Verify in project root
pwd  # Should end with /accumulate

# Install dependencies
go mod download
```

### Run Your First Tests
```bash
# Quick validation (30 seconds)
make test-unit

# Full validation (15 minutes)
make test-e2e

# Complete test suite (45 minutes)
make test-all
```

## Environment Setup

### System Requirements

**Minimum Requirements:**
- Go 1.21 or later
- 8GB RAM
- 10GB free disk space
- Linux/macOS (Windows with WSL)

**Recommended for Full Suite:**
- 16GB RAM
- 20GB free disk space
- SSD storage for better performance

### Development Environment

```bash
# Clone repository
git clone <repository-url>
cd accumulate

# Verify environment
go mod verify
go mod tidy

# Test basic functionality
go test -short ./pkg/url/...
```

### IDE Setup (VS Code)

The repository includes comprehensive VS Code configurations:

```bash
# Open in VS Code
code .

# Available launch configurations:
# - Debug Unit Tests
# - Debug E2E Tests
# - Debug Load Tests
# - Debug Network Simulation
# - Profile Performance Tests
```

## Test Categories

### 1. Unit Tests

**Purpose**: Test individual components in isolation  
**Location**: Throughout codebase (`*_test.go` files)  
**Runtime**: Fast (< 1 minute)  
**Coverage**: Functions, methods, data structures

```bash
# Run all unit tests
go test ./internal/... ./pkg/...

# Run with coverage
go test -cover ./internal/... ./pkg/...

# Run specific package
go test ./internal/api/v3/...
```

**When to Run:**
- During development
- Before commits
- In CI/CD pipelines
- For quick validation

### 2. Integration Tests

**Purpose**: Test component interactions  
**Location**: Mixed with unit tests  
**Runtime**: Moderate (2-5 minutes)  
**Coverage**: Service integration, API endpoints

```bash
# Run integration tests
go test -tags=integration ./...

# Run with longer timeout
go test -timeout=10m ./internal/api/...
```

### 3. End-to-End (E2E) Tests

**Purpose**: Test complete workflows  
**Location**: `test/e2e/`  
**Runtime**: Moderate to long (5-30 minutes)  
**Coverage**: Full transaction lifecycles, network operations

```bash
# Run all E2E tests
go test ./test/e2e/...

# Run specific test category
go test ./test/e2e/txn_*_test.go

# Run with verbose output
go test -v ./test/e2e/api_test.go
```

**Test Categories:**
- **Transaction Tests**: All transaction types and scenarios
- **Signature Tests**: Multi-signature, delegation, authority
- **Network Tests**: Cross-partition, anchoring, consensus
- **System Tests**: Genesis, limits, state consistency
- **Regression Tests**: Critical bug prevention

### 4. Performance Tests

**Purpose**: Validate system performance and capacity  
**Location**: `test/cmd/load/`, benchmark tests  
**Runtime**: Variable (1-60 minutes)  
**Coverage**: Throughput, latency, resource usage

```bash
# Run load tests
go run ./test/cmd/load/main.go

# Run benchmarks
go test -bench=. ./...

# Profile performance
go test -bench=BenchmarkNotFound -cpuprofile=cpu.prof ./pkg/database/values
```

### 5. Simulator Tests

**Purpose**: Test complex network scenarios  
**Location**: `test/simulator/`  
**Runtime**: Variable (1-10 minutes)  
**Coverage**: Network behavior, consensus, multi-partition

```bash
# Run simulator tests
go test ./test/simulator/...

# Run with custom network size
go test -run TestCustomNetwork ./test/simulator/...
```

## Running Tests

### Standard Go Testing

```bash
# Basic test execution
go test ./...                    # All tests
go test ./test/e2e/...          # E2E tests only
go test ./internal/api/...       # API tests only

# With options
go test -v ./test/e2e/...       # Verbose output
go test -short ./...            # Skip slow tests
go test -race ./...             # Race detection
go test -timeout=30m ./...      # Custom timeout

# Specific tests
go test -run TestSendTokens ./test/e2e/...
go test -run "TestSendTokens|TestAddCredits" ./test/e2e/...
```

### Test Selection Patterns

```bash
# By functionality
go test -run TestSend ./test/e2e/...           # All send-related tests
go test -run TestSignature ./test/e2e/...      # All signature tests
go test -run TestNetwork ./test/e2e/...        # All network tests

# By issue number
go test -run "AC-3069" ./test/e2e/...          # Specific issue tests

# By test file
go test ./test/e2e/txn_send_tokens_test.go     # Single file
go test ./test/e2e/sig_*_test.go               # Signature tests
```

## Makefile Targets

### Recommended Targets

```bash
# Quick validation
make test-unit          # Unit tests only (~30s)
make test-short         # Unit tests with -short flag

# Integration testing
make test-integration   # Integration tests (~5m)
make test-e2e          # End-to-end tests (~15m)

# Performance testing
make test-performance   # Load and benchmark tests
make test-benchmark     # Benchmark tests only
make test-load         # Load testing utility

# Comprehensive testing
make test-all          # All test categories (~45m)
make test-coverage     # Generate coverage report

# Maintenance
make test-clean        # Clean test artifacts
make test-deps         # Install test dependencies
```

### Custom Makefile Implementation

Add these targets to your Makefile:

```makefile
# Test targets
.PHONY: test-unit test-e2e test-performance test-all test-coverage

test-unit:
	go test -short ./internal/... ./pkg/...

test-e2e:
	go test -timeout=30m ./test/e2e/...

test-performance:
	go run ./test/cmd/load/main.go -transactions=1000 -duration=60
	go test -bench=. -benchmem ./...

test-all: test-unit test-e2e test-performance

test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

test-clean:
	rm -f coverage.out coverage.html
	rm -rf test/data/tmp/
	go clean -testcache
```

## Test Configuration

### Simulator Configuration

```go
// Basic 3-node network
sim := simulator.New(t, 3)
sim.InitFromGenesis()

// Multi-partition network
sim := simulator.New(t, 3).WithPartitions(2)
sim.InitFromGenesis()

// Custom configuration
sim := simulator.New(t, 3)
sim.SetOptions(simulator.Options{
    BvnCount:       2,
    ValidatorCount: 3,
    NetworkName:    "test-network",
})
```

### Load Test Configuration

```bash
# Basic load test
go run ./test/cmd/load/main.go \
  -server=http://localhost:8080 \
  -transactions=100 \
  -duration=30

# High-throughput test
go run ./test/cmd/load/main.go \
  -server=http://localhost:8080 \
  -transactions=10000 \
  -duration=300 \
  -max-goroutines=100

# Custom parameters
export SERVER_URL="http://localhost:8080"
export TRANSACTIONS=1000
export DURATION=60
go run ./test/cmd/load/main.go
```

### Environment Variables

```bash
# Test configuration
export ACC_TEST_TIMEOUT=30m        # Test timeout
export ACC_TEST_PARALLEL=4         # Parallel execution
export ACC_TEST_VERBOSE=true       # Verbose output

# Network configuration
export ACC_TEST_NETWORK=testnet    # Network name
export ACC_TEST_API_URL=http://localhost:8080

# Performance testing
export ACC_LOAD_TRANSACTIONS=1000  # Load test transactions
export ACC_LOAD_DURATION=60        # Load test duration
export ACC_LOAD_GOROUTINES=25      # Concurrent goroutines
```

## Debugging Tests

### VS Code Debugging

1. **Set Breakpoints**: Click in gutter next to line numbers
2. **Select Configuration**: Choose from `.vscode/launch.json`
3. **Start Debugging**: Press F5 or use Debug menu

**Available Configurations:**
- **Debug Unit Tests**: Debug specific unit test
- **Debug E2E Tests**: Debug end-to-end scenarios
- **Debug Load Tests**: Debug performance tests
- **Debug Network Simulation**: Debug simulator tests

### Command Line Debugging

```bash
# Verbose output
go test -v ./test/e2e/...

# Race condition detection
go test -race ./...

# Memory profiling
go test -memprofile=mem.prof ./...
go tool pprof mem.prof

# CPU profiling
go test -cpuprofile=cpu.prof ./...
go tool pprof cpu.prof

# Trace execution
go test -trace=trace.out ./...
go tool trace trace.out
```

### Debug Logging

```go
// Enable debug logging in tests
import "gitlab.com/accumulatenetwork/accumulate/internal/logging"

func TestExample(t *testing.T) {
    logger := logging.NewTestLogger(t, "debug", false)
    // Use logger in test
}
```

### Common Debug Scenarios

```bash
# Test hangs or times out
go test -timeout=1m -v ./test/e2e/specific_test.go

# Memory issues
go test -memprofile=mem.prof -run TestSpecific ./...

# Race conditions
go test -race -run TestConcurrent ./...

# Network issues
go test -v -run TestNetwork ./test/e2e/...
```

## Performance Testing

### Load Testing

```bash
# Basic load test
go run ./test/cmd/load/main.go

# Custom parameters
go run ./test/cmd/load/main.go \
  -server=http://localhost:8080 \
  -transactions=5000 \
  -duration=120 \
  -max-goroutines=50
```

### Benchmark Testing

```bash
# Run all benchmarks
go test -bench=. ./...

# Specific benchmarks
go test -bench=BenchmarkNotFound ./pkg/database/values

# With profiling
go test -bench=. -cpuprofile=cpu.prof -memprofile=mem.prof ./...

# Compare benchmarks
go test -bench=. ./... > old.bench
# Make changes
go test -bench=. ./... > new.bench
benchcmp old.bench new.bench
```

### Performance Analysis

```bash
# CPU profiling
go tool pprof cpu.prof
(pprof) top10
(pprof) web

# Memory profiling
go tool pprof mem.prof
(pprof) top10
(pprof) list FunctionName

# Trace analysis
go tool trace trace.out
```

## Continuous Integration

### GitHub Actions Example

```yaml
name: Tests
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v3
    - uses: actions/setup-go@v3
      with:
        go-version: '1.21'
    
    - name: Unit Tests
      run: make test-unit
    
    - name: E2E Tests
      run: make test-e2e
      timeout-minutes: 30
    
    - name: Performance Tests
      run: make test-performance
      if: github.event_name == 'push'
```

### GitLab CI Example

```yaml
stages:
  - test

unit-tests:
  stage: test
  script:
    - make test-unit
  
e2e-tests:
  stage: test
  script:
    - make test-e2e
  timeout: 30m
  
performance-tests:
  stage: test
  script:
    - make test-performance
  only:
    - main
```

## Troubleshooting

### Common Issues

#### 1. Port Conflicts
```bash
# Check for port usage
lsof -i :8080
netstat -tulpn | grep :8080

# Kill conflicting processes
pkill -f accumulated
```

#### 2. Database Lock Issues
```bash
# Clean test databases
rm -rf test/data/tmp/
go clean -testcache

# Run tests sequentially
go test -p 1 ./test/e2e/...
```

#### 3. Memory Issues
```bash
# Increase memory limits
export GOMAXPROCS=4
export GOMEMLIMIT=8GB

# Run with memory profiling
go test -memprofile=mem.prof ./...
```

#### 4. Timeout Issues
```bash
# Increase timeout
go test -timeout=60m ./test/e2e/...

# Run specific slow test
go test -timeout=10m -run TestSlowFunction ./...
```

### Debug Commands

```bash
# Verbose test output
go test -v ./test/e2e/...

# Show test coverage
go test -cover ./...

# List available tests
go test -list . ./test/e2e/...

# Show test binary info
go test -c ./test/e2e/
./e2e.test -test.list
```

### Log Analysis

```bash
# Enable debug logging
export ACC_LOG_LEVEL=debug

# Filter logs
go test -v ./... 2>&1 | grep ERROR

# Save logs
go test -v ./... > test.log 2>&1
```

## Best Practices

### Writing Tests

1. **Use Descriptive Names**: `TestSendTokens_DuplicateRecipients`
2. **Test One Thing**: Each test should validate one specific behavior
3. **Use Table Tests**: For multiple input scenarios
4. **Clean Up Resources**: Use `defer` for cleanup
5. **Use Test Helpers**: Extract common setup code

### Test Organization

```go
func TestSendTokens(t *testing.T) {
    tests := []struct {
        name     string
        amount   int64
        expected error
    }{
        {"valid amount", 1000, nil},
        {"zero amount", 0, ErrInvalidAmount},
        {"negative amount", -100, ErrInvalidAmount},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

### Performance Testing

1. **Baseline First**: Establish performance baselines
2. **Consistent Environment**: Use same hardware/OS for comparisons
3. **Multiple Runs**: Average results across multiple runs
4. **Profile Regularly**: Use profiling to identify bottlenecks
5. **Monitor Regressions**: Track performance over time

### Maintenance

1. **Update Regularly**: Keep tests current with code changes
2. **Remove Dead Tests**: Delete obsolete or redundant tests
3. **Review Coverage**: Maintain good test coverage
4. **Document Changes**: Update test documentation
5. **Monitor CI**: Fix failing tests promptly

## Advanced Topics

### Custom Test Harness

```go
// Create custom test environment
func setupTestEnvironment(t *testing.T) *TestEnv {
    env := &TestEnv{
        Simulator: simulator.New(t, 3),
        Logger:    logging.NewTestLogger(t, "info", false),
    }
    env.Simulator.InitFromGenesis()
    return env
}
```

### Network Simulation

```go
// Multi-partition network testing
func TestCrossPartitionTransfer(t *testing.T) {
    sim := simulator.New(t, 3).WithPartitions(2)
    sim.InitFromGenesis()
    
    // Test cross-partition operations
}
```

### Load Testing Patterns

```go
// Concurrent load testing
func TestConcurrentLoad(t *testing.T) {
    const numGoroutines = 10
    const transactionsPerGoroutine = 100
    
    var wg sync.WaitGroup
    for i := 0; i < numGoroutines; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            // Execute transactions
        }()
    }
    wg.Wait()
}
```

---

## See Also

- [test-content.md](test-content.md) - Complete test suite catalog
- [unit-tests.md](unit-tests.md) - Unit testing specifics
- [e2e-tests.md](e2e-tests.md) - End-to-end testing guide
- [performance-tests.md](performance-tests.md) - Performance testing details
- [debugging.md](debugging.md) - Advanced debugging techniques

*This documentation is maintained alongside the test suite. Please update when adding new test types or procedures.*
