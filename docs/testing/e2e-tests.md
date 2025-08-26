# End-to-End Testing Guide

## Table of Contents

1. [Overview](#overview)
2. [Quick Start](#quick-start)
3. [Test Architecture](#test-architecture)
4. [Test Categories](#test-categories)
5. [Running E2E Tests](#running-e2e-tests)
6. [Simulator Framework](#simulator-framework)
7. [Test Harness](#test-harness)
8. [Writing E2E Tests](#writing-e2e-tests)
9. [Common Patterns](#common-patterns)
10. [Debugging E2E Tests](#debugging-e2e-tests)
11. [Performance Considerations](#performance-considerations)
12. [Best Practices](#best-practices)

## Overview

End-to-end (E2E) tests validate complete workflows and system integration in the Accumulate Network. These tests simulate real-world scenarios using a complete blockchain network environment, ensuring all components work together correctly.

### Key Characteristics

- **Complete Workflows**: Test entire transaction lifecycles
- **Network Simulation**: Full blockchain network with multiple nodes
- **Real Components**: Uses actual protocol, consensus, and API layers
- **Integration Focus**: Validates component interactions
- **Realistic Scenarios**: Mirrors production usage patterns

### Test Statistics

```
Total E2E Test Files: 53
├── Transaction Tests: 13 (25%) - txn_*.go files
├── Signature Tests: 7 (13%) - sig_*.go files  
├── Network Tests: 7 (13%) - net_*.go files
├── Block/Execution Tests: 6 (11%) - block_test.go, exec_*.go, major_block_test.go, msg_*.go
├── System Tests: 8 (15%) - genesis, limits, credits, fuzz, replay, state, sys_block, api
├── Regression Tests: 2 (4%) - regression*.go files
├── Sequence Tests: 2 (4%) - sequence*.go files
├── Validator Tests: 1 (2%) - validators_test.go
├── Special Tests: 3 (6%) - _factom_addresses, _relaunch, util
└── Other Tests: 4 (7%) - remaining test files
```

## Quick Start

### Prerequisites

```bash
# Verify Go version
go version  # Should be 1.21+

# Verify in project root
pwd  # Should end with /accumulate

# Clean any previous test state
rm -rf test/data/tmp/
go clean -testcache
```

### Run Basic E2E Tests

```bash
# Quick validation (5 minutes)
go test -timeout=10m ./test/e2e/api_test.go

# Transaction tests (10 minutes)
go test -timeout=15m ./test/e2e/txn_*_test.go

# All E2E tests (30 minutes)
go test -timeout=45m ./test/e2e/...
```

### Run Specific Test Categories

```bash
# Send tokens tests
go test -run TestSendTokens ./test/e2e/...

# Signature tests
go test -run TestSignature ./test/e2e/...

# Network tests
go test -run TestNetwork ./test/e2e/...
```

## Test Architecture

### Directory Structure

```
test/
├── e2e/                           # End-to-end tests
│   ├── api_test.go               # API endpoint tests
│   ├── txn_send_tokens_test.go   # Send tokens transaction tests
│   ├── txn_add_credits_test.go   # Add credits transaction tests
│   ├── sig_*_test.go             # Signature-related tests
│   ├── net_*_test.go             # Network behavior tests
│   ├── sys_*_test.go             # System-level tests
│   └── regression_*_test.go      # Regression tests
├── simulator/                     # Network simulation framework
│   ├── simulator.go              # Core simulator
│   ├── consensus.go              # Consensus simulation
│   ├── partition.go              # Partition management
│   └── api.go                    # API simulation
├── harness/                       # Test harness utilities
│   ├── harness.go                # Test harness core
│   ├── conditions.go             # Test conditions
│   ├── queries.go                # Query utilities
│   └── failures.go               # Failure handling
└── testing/                       # Testing utilities
    ├── node.go                   # Test node implementation
    ├── state.go                  # State management
    └── batch.go                  # Batch operations
```

### Component Relationships

```
E2E Test
    ↓
Test Harness ←→ Simulator Framework
    ↓               ↓
Query Utils     Network Nodes
    ↓               ↓
API Layer      Consensus Layer
    ↓               ↓
Protocol       Database Layer
```

## Test Categories

### 1. Transaction Tests (`txn_*_test.go`)

**Purpose**: Validate complete transaction workflows  
**Coverage**: All transaction types and scenarios

```bash
# All transaction tests
go test ./test/e2e/txn_*_test.go

# Specific transaction types
go test -run TestSendTokens ./test/e2e/txn_send_tokens_test.go
go test -run TestAddCredits ./test/e2e/txn_add_credits_test.go
go test -run TestCreateAccount ./test/e2e/txn_create_account_test.go
```

**Key Test Files:**
- `txn_send_tokens_test.go` - Token transfer scenarios
- `txn_add_credits_test.go` - Credit addition workflows
- `txn_create_account_test.go` - Account creation tests
- `txn_update_account_test.go` - Account modification tests
- `txn_write_data_test.go` - Data writing operations

### 2. Signature Tests (`sig_*_test.go`)

**Purpose**: Validate signature and authority mechanisms  
**Coverage**: Multi-signature, delegation, authority management

```bash
# All signature tests
go test ./test/e2e/sig_*_test.go

# Specific signature scenarios
go test -run TestMultiSig ./test/e2e/sig_multi_test.go
go test -run TestDelegation ./test/e2e/sig_delegation_test.go
```

**Key Test Files:**
- `sig_multi_test.go` - Multi-signature scenarios
- `sig_delegation_test.go` - Authority delegation
- `sig_threshold_test.go` - Threshold signatures
- `sig_recovery_test.go` - Key recovery mechanisms

### 3. Network Tests (`net_*_test.go`)

**Purpose**: Validate network-level behaviors  
**Coverage**: Cross-partition, anchoring, consensus

```bash
# All network tests
go test ./test/e2e/net_*_test.go

# Specific network scenarios
go test -run TestCrossPartition ./test/e2e/net_partition_test.go
go test -run TestAnchoring ./test/e2e/net_anchoring_test.go
```

**Key Test Files:**
- `net_partition_test.go` - Cross-partition operations
- `net_anchoring_test.go` - Anchoring mechanisms
- `net_consensus_test.go` - Consensus behavior
- `net_routing_test.go` - Transaction routing

### 4. System Tests (`sys_*_test.go`)

**Purpose**: Validate system-level functionality  
**Coverage**: Genesis, limits, state consistency

```bash
# All system tests
go test ./test/e2e/sys_*_test.go

# Specific system scenarios
go test -run TestGenesis ./test/e2e/sys_genesis_test.go
go test -run TestLimits ./test/e2e/sys_limits_test.go
```

### 5. Regression Tests (`regression_*_test.go`)

**Purpose**: Prevent known issues from reoccurring  
**Coverage**: Critical bug scenarios

```bash
# All regression tests
go test ./test/e2e/regression_*_test.go

# Specific issue tests
go test -run "AC-3069" ./test/e2e/regression_*_test.go
```

## Running E2E Tests

### Basic Execution

```bash
# All E2E tests (long running)
go test -timeout=45m ./test/e2e/...

# Specific test file
go test -timeout=10m ./test/e2e/api_test.go

# Specific test function
go test -timeout=5m -run TestSendTokens ./test/e2e/txn_send_tokens_test.go
```

### Test Selection

```bash
# Pattern matching
go test -run "TestSend.*Valid" ./test/e2e/...
go test -run "TestSignature.*Multi" ./test/e2e/...

# Multiple patterns
go test -run "TestSend|TestReceive" ./test/e2e/...

# Exclude patterns
go test -run "^((?!TestSlow).)*$" ./test/e2e/...
```

### Execution Options

```bash
# Verbose output
go test -v ./test/e2e/api_test.go

# Parallel execution (limited for E2E)
go test -parallel 2 ./test/e2e/...

# Race detection
go test -race ./test/e2e/api_test.go

# Memory profiling
go test -memprofile=mem.prof ./test/e2e/api_test.go

# CPU profiling
go test -cpuprofile=cpu.prof ./test/e2e/api_test.go
```

## Simulator Framework

### Basic Simulator Setup

```go
import (
    "testing"
    "gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

func TestBasicTransaction(t *testing.T) {
    // Create 3-node network
    sim := simulator.New(t, 3)
    sim.InitFromGenesis()
    
    // Test implementation
}
```

### Multi-Partition Network

```go
func TestCrossPartition(t *testing.T) {
    // Create network with 2 partitions
    sim := simulator.New(t, 3).WithPartitions(2)
    sim.InitFromGenesis()
    
    // Access different partitions
    alice := sim.Partition("BVN0").Account("alice")
    bob := sim.Partition("BVN1").Account("bob")
    
    // Test cross-partition transaction
}
```

### Custom Network Configuration

```go
func TestCustomNetwork(t *testing.T) {
    sim := simulator.New(t, 3)
    sim.SetOptions(simulator.Options{
        BvnCount:       2,
        ValidatorCount: 3,
        NetworkName:    "test-network",
    })
    sim.InitFromGenesis()
    
    // Test with custom configuration
}
```

### Simulator Operations

```go
func TestSimulatorOperations(t *testing.T) {
    sim := simulator.New(t, 3)
    sim.InitFromGenesis()
    
    // Submit transaction
    txn := &SendTokens{
        From:   alice.URL(),
        To:     bob.URL(),
        Amount: 1000,
    }
    sim.Submit(txn)
    
    // Execute blocks
    sim.ExecuteBlocks(5)
    
    // Query state
    account := sim.Query(alice.URL())
    
    // Verify results
    require.Equal(t, expectedBalance, account.Balance)
}
```

## Test Harness

### Harness Initialization

```go
import "gitlab.com/accumulatenetwork/accumulate/test/harness"

func TestWithHarness(t *testing.T) {
    // Create harness with simulator
    h := harness.New(t, simulator.New(t, 3))
    h.InitFromGenesis()
    
    // Use harness for testing
}
```

### Account Management

```go
func TestAccountOperations(t *testing.T) {
    h := harness.New(t, simulator.New(t, 3))
    h.InitFromGenesis()
    
    // Create test accounts
    alice := h.CreateAccount("alice")
    bob := h.CreateAccount("bob")
    
    // Fund accounts
    h.FundAccount(alice, 10000)
    h.FundAccount(bob, 5000)
    
    // Test operations
}
```

### Transaction Submission

```go
func TestTransactionSubmission(t *testing.T) {
    h := harness.New(t, simulator.New(t, 3))
    h.InitFromGenesis()
    
    alice := h.CreateAccount("alice")
    bob := h.CreateAccount("bob")
    
    // Submit transaction
    txn := &SendTokens{
        From:   alice.URL(),
        To:     bob.URL(),
        Amount: 1000,
    }
    
    result := h.Submit(txn)
    h.ExecuteBlocks(3)
    
    // Verify transaction
    require.NoError(t, result.Error)
    require.Equal(t, "delivered", result.Status)
}
```

### Condition Checking

```go
func TestConditions(t *testing.T) {
    h := harness.New(t, simulator.New(t, 3))
    h.InitFromGenesis()
    
    alice := h.CreateAccount("alice")
    
    // Wait for condition
    h.WaitForCondition(func() bool {
        account := h.Query(alice.URL())
        return account.Balance >= 1000
    }, 10*time.Second)
    
    // Assert condition
    h.AssertCondition(func() bool {
        account := h.Query(alice.URL())
        return account.IsActive()
    })
}
```

## Writing E2E Tests

### Basic E2E Test Structure

```go
func TestSendTokensBasic(t *testing.T) {
    // Setup
    sim := simulator.New(t, 3)
    sim.InitFromGenesis()
    
    // Create accounts
    alice := sim.Account("alice")
    bob := sim.Account("bob")
    
    // Fund alice
    sim.FundAccount(alice, 10000)
    
    // Create transaction
    txn := &SendTokens{
        From:   alice.URL(),
        To:     bob.URL(),
        Amount: 1000,
    }
    
    // Submit and execute
    result := sim.Submit(txn)
    sim.ExecuteBlocks(3)
    
    // Verify results
    require.NoError(t, result.Error)
    
    aliceAccount := sim.Query(alice.URL())
    bobAccount := sim.Query(bob.URL())
    
    assert.Equal(t, 9000, aliceAccount.Balance)
    assert.Equal(t, 1000, bobAccount.Balance)
}
```

### Complex Scenario Testing

```go
func TestMultiStepWorkflow(t *testing.T) {
    sim := simulator.New(t, 3)
    sim.InitFromGenesis()
    
    // Step 1: Create accounts
    alice := sim.Account("alice")
    bob := sim.Account("bob")
    charlie := sim.Account("charlie")
    
    // Step 2: Fund accounts
    sim.FundAccount(alice, 10000)
    sim.FundAccount(bob, 5000)
    
    // Step 3: Multiple transactions
    transactions := []*SendTokens{
        {From: alice.URL(), To: bob.URL(), Amount: 1000},
        {From: bob.URL(), To: charlie.URL(), Amount: 500},
        {From: charlie.URL(), To: alice.URL(), Amount: 200},
    }
    
    // Step 4: Submit all transactions
    for _, txn := range transactions {
        result := sim.Submit(txn)
        require.NoError(t, result.Error)
    }
    
    // Step 5: Execute blocks
    sim.ExecuteBlocks(5)
    
    // Step 6: Verify final state
    aliceAccount := sim.Query(alice.URL())
    bobAccount := sim.Query(bob.URL())
    charlieAccount := sim.Query(charlie.URL())
    
    assert.Equal(t, 9200, aliceAccount.Balance)  // 10000 - 1000 + 200
    assert.Equal(t, 5500, bobAccount.Balance)    // 5000 + 1000 - 500
    assert.Equal(t, 300, charlieAccount.Balance) // 0 + 500 - 200
}
```

### Error Scenario Testing

```go
func TestInsufficientFunds(t *testing.T) {
    sim := simulator.New(t, 3)
    sim.InitFromGenesis()
    
    alice := sim.Account("alice")
    bob := sim.Account("bob")
    
    // Fund alice with insufficient amount
    sim.FundAccount(alice, 500)
    
    // Attempt to send more than available
    txn := &SendTokens{
        From:   alice.URL(),
        To:     bob.URL(),
        Amount: 1000,
    }
    
    result := sim.Submit(txn)
    sim.ExecuteBlocks(3)
    
    // Verify transaction failed
    require.Error(t, result.Error)
    assert.Contains(t, result.Error.Error(), "insufficient funds")
    
    // Verify balances unchanged
    aliceAccount := sim.Query(alice.URL())
    bobAccount := sim.Query(bob.URL())
    
    assert.Equal(t, 500, aliceAccount.Balance)
    assert.Equal(t, 0, bobAccount.Balance)
}
```

## Common Patterns

### 1. Account Setup Pattern

```go
func setupTestAccounts(t *testing.T, sim *simulator.Simulator) (alice, bob, charlie *Account) {
    alice = sim.Account("alice")
    bob = sim.Account("bob")
    charlie = sim.Account("charlie")
    
    sim.FundAccount(alice, 10000)
    sim.FundAccount(bob, 5000)
    sim.FundAccount(charlie, 2000)
    
    return alice, bob, charlie
}

func TestWithSetup(t *testing.T) {
    sim := simulator.New(t, 3)
    sim.InitFromGenesis()
    
    alice, bob, charlie := setupTestAccounts(t, sim)
    
    // Test implementation
}
```

### 2. Transaction Batch Pattern

```go
func TestBatchTransactions(t *testing.T) {
    sim := simulator.New(t, 3)
    sim.InitFromGenesis()
    
    alice, bob, charlie := setupTestAccounts(t, sim)
    
    // Batch multiple transactions
    batch := []Transaction{
        &SendTokens{From: alice.URL(), To: bob.URL(), Amount: 1000},
        &SendTokens{From: bob.URL(), To: charlie.URL(), Amount: 500},
        &AddCredits{To: alice.URL(), Amount: 100},
    }
    
    // Submit batch
    results := make([]*TransactionResult, len(batch))
    for i, txn := range batch {
        results[i] = sim.Submit(txn)
    }
    
    // Execute blocks
    sim.ExecuteBlocks(5)
    
    // Verify all transactions
    for i, result := range results {
        require.NoError(t, result.Error, "Transaction %d failed", i)
    }
}
```

### 3. State Verification Pattern

```go
func verifyAccountState(t *testing.T, sim *simulator.Simulator, url string, expectedBalance int64) {
    account := sim.Query(url)
    require.NotNil(t, account, "Account not found: %s", url)
    assert.Equal(t, expectedBalance, account.Balance, "Balance mismatch for %s", url)
}

func TestWithVerification(t *testing.T) {
    sim := simulator.New(t, 3)
    sim.InitFromGenesis()
    
    alice, bob := setupTestAccounts(t, sim)
    
    // Perform operations
    txn := &SendTokens{From: alice.URL(), To: bob.URL(), Amount: 1000}
    result := sim.Submit(txn)
    sim.ExecuteBlocks(3)
    
    require.NoError(t, result.Error)
    
    // Verify state
    verifyAccountState(t, sim, alice.URL(), 9000)
    verifyAccountState(t, sim, bob.URL(), 6000)
}
```

## Debugging E2E Tests

### Verbose Logging

```bash
# Enable verbose output
go test -v ./test/e2e/api_test.go

# Enable debug logging
export ACC_LOG_LEVEL=debug
go test -v ./test/e2e/api_test.go
```

### VS Code Debugging

1. **Set Breakpoints**: In test files or source code
2. **Select Configuration**: "Debug E2E Tests"
3. **Configure Args**: Set specific test in launch.json
4. **Start Debugging**: Press F5

```json
{
    "name": "Debug E2E Test",
    "type": "go",
    "request": "launch",
    "mode": "test",
    "program": "${workspaceFolder}/test/e2e",
    "args": [
        "-test.run",
        "TestSendTokens",
        "-test.v"
    ]
}
```

### Test State Inspection

```go
func TestWithDebugging(t *testing.T) {
    sim := simulator.New(t, 3)
    sim.InitFromGenesis()
    
    alice := sim.Account("alice")
    
    // Debug: Print account state
    t.Logf("Alice account: %+v", alice)
    
    // Debug: Print network state
    t.Logf("Network state: %+v", sim.NetworkState())
    
    // Debug: Print transaction details
    txn := &SendTokens{From: alice.URL(), To: bob.URL(), Amount: 1000}
    t.Logf("Transaction: %+v", txn)
    
    result := sim.Submit(txn)
    t.Logf("Result: %+v", result)
}
```

### Common Debug Scenarios

```bash
# Test hangs
go test -timeout=5m -v ./test/e2e/specific_test.go

# Memory issues
go test -memprofile=mem.prof ./test/e2e/api_test.go
go tool pprof mem.prof

# Race conditions
go test -race ./test/e2e/api_test.go

# Network issues
export ACC_LOG_LEVEL=debug
go test -v -run TestNetwork ./test/e2e/...
```

## Performance Considerations

### Test Execution Time

```bash
# Monitor test duration
go test -v ./test/e2e/... | grep -E "PASS.*[0-9]+\.[0-9]+s"

# Identify slow tests
go test -v ./test/e2e/... 2>&1 | grep -E "PASS.*[1-9][0-9]+\.[0-9]+s"
```

### Resource Usage

```bash
# Monitor memory usage
go test -memprofile=mem.prof ./test/e2e/...
go tool pprof mem.prof

# Monitor CPU usage
go test -cpuprofile=cpu.prof ./test/e2e/...
go tool pprof cpu.prof
```

### Optimization Strategies

1. **Parallel Execution**: Limited for E2E tests
2. **Test Isolation**: Avoid shared state
3. **Resource Cleanup**: Clean up after tests
4. **Selective Testing**: Run specific test categories

```go
// Cleanup pattern
func TestWithCleanup(t *testing.T) {
    sim := simulator.New(t, 3)
    defer sim.Close() // Cleanup resources
    
    sim.InitFromGenesis()
    
    // Test implementation
}
```

## Best Practices

### 1. Test Organization

```go
// Good: Organized by functionality
func TestSendTokens_ValidAmount(t *testing.T) { ... }
func TestSendTokens_InsufficientFunds(t *testing.T) { ... }
func TestSendTokens_InvalidRecipient(t *testing.T) { ... }

// Good: Use subtests for related scenarios
func TestSendTokens(t *testing.T) {
    t.Run("ValidAmount", func(t *testing.T) { ... })
    t.Run("InsufficientFunds", func(t *testing.T) { ... })
    t.Run("InvalidRecipient", func(t *testing.T) { ... })
}
```

### 2. Network Configuration

```go
// Good: Use appropriate network size
func TestBasicTransaction(t *testing.T) {
    sim := simulator.New(t, 3) // Minimal network
    // Test implementation
}

func TestCrossPartition(t *testing.T) {
    sim := simulator.New(t, 3).WithPartitions(2) // Multi-partition
    // Test implementation
}
```

### 3. Error Handling

```go
// Good: Test both success and failure paths
func TestSendTokens(t *testing.T) {
    t.Run("Success", func(t *testing.T) {
        // Test successful transaction
    })
    
    t.Run("InsufficientFunds", func(t *testing.T) {
        // Test error scenario
    })
}
```

### 4. State Verification

```go
// Good: Verify complete state
func TestTransaction(t *testing.T) {
    // Submit transaction
    result := sim.Submit(txn)
    sim.ExecuteBlocks(3)
    
    // Verify transaction result
    require.NoError(t, result.Error)
    
    // Verify account states
    verifyAccountState(t, sim, alice.URL(), expectedBalance)
    verifyAccountState(t, sim, bob.URL(), expectedBalance)
    
    // Verify transaction history
    history := sim.QueryHistory(alice.URL())
    assert.Len(t, history, expectedCount)
}
```

### 5. Test Data Management

```go
// Good: Use test helpers for setup
func createTestNetwork(t *testing.T) *simulator.Simulator {
    sim := simulator.New(t, 3)
    sim.InitFromGenesis()
    return sim
}

func createFundedAccount(sim *simulator.Simulator, name string, balance int64) *Account {
    account := sim.Account(name)
    sim.FundAccount(account, balance)
    return account
}
```

---

## See Also

- [testing.md](testing.md) - Complete testing guide
- [unit-tests.md](unit-tests.md) - Unit testing guide
- [simulator-tests.md](simulator-tests.md) - Simulator testing details
- [performance-tests.md](performance-tests.md) - Performance testing guide
- [debugging.md](debugging.md) - Test debugging techniques

*This guide focuses on end-to-end testing. For other testing approaches, see the related documentation.*
