# Unit Testing Guide

## Table of Contents

1. [Overview](#overview)
2. [Quick Start](#quick-start)
3. [Test Structure](#test-structure)
4. [Running Unit Tests](#running-unit-tests)
5. [Writing Unit Tests](#writing-unit-tests)
6. [Test Patterns](#test-patterns)
7. [Mocking and Stubs](#mocking-and-stubs)
8. [Coverage Analysis](#coverage-analysis)
9. [Performance Testing](#performance-testing)
10. [Best Practices](#best-practices)
11. [Common Issues](#common-issues)

## Overview

Unit tests form the foundation of the Accumulate Network test suite, providing fast, isolated validation of individual components. These tests focus on testing single functions, methods, or small units of code in isolation from external dependencies.

### Key Characteristics

- **Fast Execution**: Typically run in milliseconds
- **Isolated**: No external dependencies (databases, networks, files)
- **Focused**: Test one specific behavior or function
- **Deterministic**: Same input always produces same output
- **Independent**: Tests don't depend on each other

### Test Distribution

```
Total Unit Tests: ~2,500+
├── API Package Tests: ~400
├── Protocol Tests: ~600
├── Database Tests: ~300
├── Crypto Tests: ~200
├── URL/Address Tests: ~150
├── Core Logic Tests: ~500
└── Utility Tests: ~350
```

## Quick Start

### Run All Unit Tests
```bash
# Fast execution (< 1 minute)
go test -short ./internal/... ./pkg/...

# With coverage
go test -short -cover ./internal/... ./pkg/...

# Parallel execution
go test -short -parallel 8 ./internal/... ./pkg/...
```

### Run Specific Packages
```bash
# API tests
go test ./internal/api/v3/...

# Protocol tests
go test ./protocol/...

# Database tests
go test ./internal/database/...

# Crypto tests
go test ./pkg/crypto/...
```

## Test Structure

### Package Organization

Unit tests are co-located with source code:

```
internal/
├── api/
│   ├── v3/
│   │   ├── query.go
│   │   ├── query_test.go      # Unit tests for query.go
│   │   ├── submit.go
│   │   └── submit_test.go     # Unit tests for submit.go
│   └── ...
├── database/
│   ├── database.go
│   ├── database_test.go       # Database unit tests
│   └── ...
└── ...
```

### Test File Naming

- **Source file**: `example.go`
- **Test file**: `example_test.go`
- **Benchmark file**: `example_bench_test.go` (optional)

### Test Function Naming

```go
// Basic test
func TestFunctionName(t *testing.T) { ... }

// Table-driven test
func TestFunctionName_Scenarios(t *testing.T) { ... }

// Specific scenario
func TestFunctionName_SpecificCondition(t *testing.T) { ... }

// Benchmark
func BenchmarkFunctionName(b *testing.B) { ... }
```

## Running Unit Tests

### Basic Execution

```bash
# All unit tests
go test ./...

# Specific package
go test ./internal/api/v3

# Specific test file
go test ./internal/api/v3/query_test.go

# Specific test function
go test -run TestQueryAccount ./internal/api/v3
```

### Test Options

```bash
# Short tests only (skip slow tests)
go test -short ./...

# Verbose output
go test -v ./internal/api/v3

# Parallel execution
go test -parallel 4 ./...

# Race condition detection
go test -race ./...

# Coverage analysis
go test -cover ./...

# Timeout control
go test -timeout 30s ./...
```

### Advanced Patterns

```bash
# Test specific scenarios
go test -run "TestSendTokens.*Valid" ./...

# Multiple test patterns
go test -run "TestSend|TestReceive" ./...

# Exclude specific tests
go test -run "^((?!TestSlow).)*$" ./...

# Run tests multiple times
go test -count=5 ./internal/api/v3
```

## Writing Unit Tests

### Basic Test Structure

```go
package api

import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestQueryAccount(t *testing.T) {
    // Arrange
    account := &Account{
        URL:     "acc://test.acme/account",
        Balance: 1000,
    }
    
    // Act
    result, err := QueryAccount(account.URL)
    
    // Assert
    require.NoError(t, err)
    assert.Equal(t, account.Balance, result.Balance)
    assert.Equal(t, account.URL, result.URL)
}
```

### Table-Driven Tests

```go
func TestValidateURL(t *testing.T) {
    tests := []struct {
        name    string
        url     string
        want    bool
        wantErr bool
    }{
        {
            name: "valid account URL",
            url:  "acc://test.acme/account",
            want: true,
        },
        {
            name:    "invalid protocol",
            url:     "http://test.acme/account",
            want:    false,
            wantErr: true,
        },
        {
            name:    "empty URL",
            url:     "",
            want:    false,
            wantErr: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := ValidateURL(tt.url)
            
            if tt.wantErr {
                assert.Error(t, err)
                return
            }
            
            require.NoError(t, err)
            assert.Equal(t, tt.want, got)
        })
    }
}
```

### Subtests

```go
func TestTransactionValidation(t *testing.T) {
    t.Run("SendTokens", func(t *testing.T) {
        // Test send tokens validation
    })
    
    t.Run("AddCredits", func(t *testing.T) {
        // Test add credits validation
    })
    
    t.Run("CreateAccount", func(t *testing.T) {
        // Test create account validation
    })
}
```

## Test Patterns

### 1. Arrange-Act-Assert (AAA)

```go
func TestCalculateBalance(t *testing.T) {
    // Arrange
    account := &Account{Balance: 1000}
    transaction := &Transaction{Amount: 100}
    
    // Act
    newBalance := CalculateBalance(account, transaction)
    
    // Assert
    assert.Equal(t, 900, newBalance)
}
```

### 2. Given-When-Then

```go
func TestAccountCreation(t *testing.T) {
    // Given a valid account URL
    url := "acc://test.acme/account"
    
    // When creating an account
    account, err := CreateAccount(url)
    
    // Then the account should be created successfully
    require.NoError(t, err)
    assert.Equal(t, url, account.URL)
    assert.Equal(t, 0, account.Balance)
}
```

### 3. Test Fixtures

```go
func TestAccountOperations(t *testing.T) {
    // Setup test fixture
    account := createTestAccount(t)
    
    t.Run("Deposit", func(t *testing.T) {
        err := account.Deposit(100)
        require.NoError(t, err)
        assert.Equal(t, 100, account.Balance)
    })
    
    t.Run("Withdraw", func(t *testing.T) {
        account.Balance = 200
        err := account.Withdraw(50)
        require.NoError(t, err)
        assert.Equal(t, 150, account.Balance)
    })
}

func createTestAccount(t *testing.T) *Account {
    account := &Account{
        URL:     "acc://test.acme/account",
        Balance: 0,
    }
    return account
}
```

### 4. Error Testing

```go
func TestWithdraw_InsufficientFunds(t *testing.T) {
    account := &Account{Balance: 50}
    
    err := account.Withdraw(100)
    
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "insufficient funds")
    assert.Equal(t, 50, account.Balance) // Balance unchanged
}
```

## Mocking and Stubs

### Interface Mocking

```go
// Define interface
type DatabaseInterface interface {
    Get(key string) ([]byte, error)
    Put(key string, value []byte) error
}

// Mock implementation
type MockDatabase struct {
    data map[string][]byte
}

func (m *MockDatabase) Get(key string) ([]byte, error) {
    value, exists := m.data[key]
    if !exists {
        return nil, errors.New("key not found")
    }
    return value, nil
}

func (m *MockDatabase) Put(key string, value []byte) error {
    m.data[key] = value
    return nil
}

// Test using mock
func TestService_GetAccount(t *testing.T) {
    // Arrange
    mockDB := &MockDatabase{
        data: map[string][]byte{
            "account1": []byte(`{"balance": 1000}`),
        },
    }
    service := NewService(mockDB)
    
    // Act
    account, err := service.GetAccount("account1")
    
    // Assert
    require.NoError(t, err)
    assert.Equal(t, 1000, account.Balance)
}
```

### Using testify/mock

```go
import (
    "github.com/stretchr/testify/mock"
    "github.com/stretchr/testify/assert"
)

type MockDatabase struct {
    mock.Mock
}

func (m *MockDatabase) Get(key string) ([]byte, error) {
    args := m.Called(key)
    return args.Get(0).([]byte), args.Error(1)
}

func TestService_GetAccount_WithMock(t *testing.T) {
    // Arrange
    mockDB := new(MockDatabase)
    mockDB.On("Get", "account1").Return([]byte(`{"balance": 1000}`), nil)
    
    service := NewService(mockDB)
    
    // Act
    account, err := service.GetAccount("account1")
    
    // Assert
    require.NoError(t, err)
    assert.Equal(t, 1000, account.Balance)
    mockDB.AssertExpectations(t)
}
```

## Coverage Analysis

### Generate Coverage Reports

```bash
# Basic coverage
go test -cover ./...

# Detailed coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html

# Coverage by package
go test -cover ./internal/api/v3
go test -cover ./internal/database
go test -cover ./pkg/crypto
```

### Coverage Targets

```bash
# Set coverage threshold
go test -cover ./... | grep -E "(PASS|FAIL).*coverage:"

# Coverage analysis script
#!/bin/bash
THRESHOLD=80
COVERAGE=$(go test -cover ./... | grep "coverage:" | awk '{print $5}' | sed 's/%//' | sort -n | tail -1)
if (( $(echo "$COVERAGE < $THRESHOLD" | bc -l) )); then
    echo "Coverage $COVERAGE% is below threshold $THRESHOLD%"
    exit 1
fi
```

### Package-Specific Coverage

```bash
# API package coverage
go test -cover ./internal/api/v3/...

# Database package coverage
go test -cover ./internal/database/...

# Protocol package coverage
go test -cover ./protocol/...
```

## Performance Testing

### Benchmark Tests

```go
func BenchmarkValidateURL(b *testing.B) {
    url := "acc://test.acme/account"
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        ValidateURL(url)
    }
}

func BenchmarkParseTransaction(b *testing.B) {
    data := []byte(`{"type": "sendTokens", "amount": 1000}`)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        ParseTransaction(data)
    }
}
```

### Memory Benchmarks

```go
func BenchmarkCreateAccount(b *testing.B) {
    b.ReportAllocs()
    
    for i := 0; i < b.N; i++ {
        account := &Account{
            URL:     "acc://test.acme/account",
            Balance: 0,
        }
        _ = account
    }
}
```

### Running Benchmarks

```bash
# Run all benchmarks
go test -bench=. ./...

# Specific benchmark
go test -bench=BenchmarkValidateURL ./internal/api/v3

# With memory stats
go test -bench=. -benchmem ./...

# Multiple runs for stability
go test -bench=. -count=5 ./...
```

## Best Practices

### 1. Test Naming

```go
// Good: Descriptive and specific
func TestSendTokens_ValidAmount_UpdatesBalance(t *testing.T) { ... }
func TestValidateURL_EmptyString_ReturnsError(t *testing.T) { ... }

// Bad: Vague or generic
func TestSendTokens(t *testing.T) { ... }
func TestValidation(t *testing.T) { ... }
```

### 2. Test Independence

```go
// Good: Each test is independent
func TestAccountBalance(t *testing.T) {
    account := createTestAccount()
    account.Deposit(100)
    assert.Equal(t, 100, account.Balance)
}

// Bad: Tests depend on shared state
var globalAccount *Account

func TestDeposit(t *testing.T) {
    globalAccount.Deposit(100)
    assert.Equal(t, 100, globalAccount.Balance)
}
```

### 3. Clear Assertions

```go
// Good: Specific assertions
assert.Equal(t, 1000, account.Balance)
assert.Contains(t, err.Error(), "insufficient funds")
assert.True(t, account.IsActive())

// Bad: Generic assertions
assert.NotNil(t, account)
assert.True(t, account.Balance > 0)
```

### 4. Test Data Management

```go
// Good: Use test helpers
func createTestAccount(balance int) *Account {
    return &Account{
        URL:     "acc://test.acme/account",
        Balance: balance,
    }
}

// Good: Use constants for test data
const (
    TestAccountURL = "acc://test.acme/account"
    TestAmount     = 1000
)
```

### 5. Error Testing

```go
// Good: Test both success and failure cases
func TestWithdraw(t *testing.T) {
    t.Run("Success", func(t *testing.T) {
        account := createTestAccount(1000)
        err := account.Withdraw(500)
        require.NoError(t, err)
        assert.Equal(t, 500, account.Balance)
    })
    
    t.Run("InsufficientFunds", func(t *testing.T) {
        account := createTestAccount(100)
        err := account.Withdraw(500)
        assert.Error(t, err)
        assert.Contains(t, err.Error(), "insufficient funds")
    })
}
```

## Common Issues

### 1. Flaky Tests

```go
// Problem: Time-dependent tests
func TestTimeout(t *testing.T) {
    start := time.Now()
    DoSomething()
    duration := time.Since(start)
    assert.True(t, duration < time.Second) // Flaky!
}

// Solution: Use deterministic testing
func TestTimeout(t *testing.T) {
    mockClock := &MockClock{}
    service := NewService(mockClock)
    
    result := service.DoSomething()
    assert.True(t, result.Completed)
}
```

### 2. Test Pollution

```go
// Problem: Global state modification
var globalConfig = &Config{}

func TestFeature(t *testing.T) {
    globalConfig.Enabled = true // Affects other tests!
    // Test implementation
}

// Solution: Isolate state
func TestFeature(t *testing.T) {
    config := &Config{Enabled: true}
    service := NewService(config)
    // Test implementation
}
```

### 3. Over-Mocking

```go
// Problem: Mocking everything
func TestComplexOperation(t *testing.T) {
    mockA := &MockA{}
    mockB := &MockB{}
    mockC := &MockC{}
    // Too many mocks make test brittle
}

// Solution: Test at appropriate level
func TestComplexOperation(t *testing.T) {
    // Use real objects where possible
    // Mock only external dependencies
}
```

### 4. Slow Tests

```bash
# Identify slow tests
go test -v ./... | grep -E "PASS.*[0-9]+\.[0-9]+s"

# Use -short flag for fast tests
go test -short ./...

# Profile test execution
go test -cpuprofile=test.prof ./...
```

## Integration with CI/CD

### GitHub Actions

```yaml
name: Unit Tests
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v3
    - uses: actions/setup-go@v3
      with:
        go-version: '1.21'
    
    - name: Run Unit Tests
      run: go test -short -cover ./internal/... ./pkg/...
    
    - name: Generate Coverage
      run: |
        go test -short -coverprofile=coverage.out ./internal/... ./pkg/...
        go tool cover -html=coverage.out -o coverage.html
    
    - name: Upload Coverage
      uses: actions/upload-artifact@v3
      with:
        name: coverage-report
        path: coverage.html
```

### Makefile Integration

```makefile
.PHONY: test-unit test-unit-coverage test-unit-verbose

test-unit:
	go test -short ./internal/... ./pkg/...

test-unit-coverage:
	go test -short -coverprofile=coverage.out ./internal/... ./pkg/...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

test-unit-verbose:
	go test -short -v ./internal/... ./pkg/...

test-unit-race:
	go test -short -race ./internal/... ./pkg/...
```

---

## See Also

- [testing.md](testing.md) - Complete testing guide
- [e2e-tests.md](e2e-tests.md) - End-to-end testing guide
- [performance-tests.md](performance-tests.md) - Performance testing details
- [debugging.md](debugging.md) - Test debugging techniques
- [test-content.md](test-content.md) - Complete test suite catalog

*This guide covers unit testing specifics. For broader testing concepts, see the main testing guide.*
