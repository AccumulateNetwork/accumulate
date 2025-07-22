# Simulator Testing Guide

## Overview

The Accumulate simulator provides a controlled environment for testing network behavior, consensus mechanisms, and multi-partition interactions without requiring a full network deployment.

## Quick Start

```bash
# Run simulator tests
go test ./test/simulator/...

# Specific simulator test
go test -run TestSimulator ./test/simulator

# With verbose output
go test -v ./test/simulator/...
```

## Simulator Architecture

### Core Components

```
Simulator Framework:
├── Network Simulation (test/simulator/network.go)
├── Partition Management (test/simulator/partition.go)
├── Block Production (test/simulator/block.go)
├── Transaction Processing (test/simulator/transaction.go)
└── State Management (test/simulator/state.go)
```

### Key Files

- `test/simulator/simulator.go` - Main simulator implementation
- `test/simulator/network.go` - Network topology simulation
- `test/simulator/partition.go` - Partition management
- `test/simulator/transaction.go` - Transaction simulation
- `test/simulator/block.go` - Block production simulation

## Basic Usage

### Creating a Simulator

```go
func TestBasicSimulator(t *testing.T) {
    // Create simulator with 3 validators
    sim := simulator.New(t, 3)
    
    // Initialize from genesis
    sim.InitFromGenesis()
    
    // Create test accounts
    alice := sim.Partition("BVN0").Account("alice")
    bob := sim.Partition("BVN1").Account("bob")
    
    // Fund accounts
    sim.FundAccount(alice, 1000000)
    
    // Submit transaction
    txn := &SendTokens{
        From:   alice.URL(),
        To:     bob.URL(),
        Amount: 100,
    }
    
    result := sim.Submit(txn)
    require.NoError(t, result.Error)
    
    // Execute blocks
    sim.ExecuteBlocks(10)
    
    // Verify results
    balance := sim.QueryBalance(bob.URL())
    assert.Equal(t, 100, balance)
}
```

### Multi-Partition Setup

```go
func TestMultiPartition(t *testing.T) {
    // Create simulator with multiple partitions
    sim := simulator.New(t, 5).WithPartitions(3)
    sim.InitFromGenesis()
    
    // Access different partitions
    bvn0 := sim.Partition("BVN0")
    bvn1 := sim.Partition("BVN1")
    bvn2 := sim.Partition("BVN2")
    
    // Create accounts on different partitions
    alice := bvn0.Account("alice")
    bob := bvn1.Account("bob")
    charlie := bvn2.Account("charlie")
    
    // Test cross-partition transactions
    sim.FundAccount(alice, 1000000)
    
    // BVN0 -> BVN1
    sim.Submit(&SendTokens{
        From: alice.URL(),
        To:   bob.URL(),
        Amount: 100,
    })
    
    // BVN1 -> BVN2
    sim.Submit(&SendTokens{
        From: bob.URL(),
        To:   charlie.URL(),
        Amount: 50,
    })
    
    sim.ExecuteBlocks(20)
    
    // Verify cross-partition state
    assert.Equal(t, 50, sim.QueryBalance(charlie.URL()))
}
```

## Advanced Simulator Features

### Custom Network Topology

```go
func TestCustomTopology(t *testing.T) {
    config := simulator.Config{
        Validators: 7,
        Partitions: []string{"BVN0", "BVN1", "BVN2"},
        NetworkDelay: 100 * time.Millisecond,
        BlockTime: 1 * time.Second,
    }
    
    sim := simulator.NewWithConfig(t, config)
    sim.InitFromGenesis()
    
    // Test with custom configuration
}
```

### Fault Injection

```go
func TestNetworkPartition(t *testing.T) {
    sim := simulator.New(t, 5)
    sim.InitFromGenesis()
    
    // Create network partition
    sim.PartitionNetwork("BVN0", "BVN1")
    
    // Test behavior during partition
    alice := sim.Partition("BVN0").Account("alice")
    bob := sim.Partition("BVN1").Account("bob")
    
    sim.FundAccount(alice, 1000000)
    
    // This should fail or be delayed
    result := sim.Submit(&SendTokens{
        From: alice.URL(),
        To:   bob.URL(),
        Amount: 100,
    })
    
    // Heal partition
    sim.HealNetwork()
    
    // Now it should succeed
    sim.ExecuteBlocks(10)
}
```

### Performance Testing

```go
func TestSimulatorPerformance(t *testing.T) {
    sim := simulator.New(t, 3)
    sim.InitFromGenesis()
    
    alice := sim.Partition("BVN0").Account("alice")
    bob := sim.Partition("BVN1").Account("bob")
    
    sim.FundAccount(alice, 1000000)
    
    // High-throughput test
    const numTxns = 1000
    start := time.Now()
    
    for i := 0; i < numTxns; i++ {
        sim.Submit(&SendTokens{
            From: alice.URL(),
            To:   bob.URL(),
            Amount: 1,
        })
    }
    
    sim.ExecuteBlocks(100)
    duration := time.Since(start)
    
    tps := float64(numTxns) / duration.Seconds()
    t.Logf("Simulator TPS: %.2f", tps)
    
    // Verify final state
    balance := sim.QueryBalance(bob.URL())
    assert.Equal(t, numTxns, balance)
}
```

## Test Patterns

### State Verification

```go
func TestStateConsistency(t *testing.T) {
    sim := simulator.New(t, 3)
    sim.InitFromGenesis()
    
    // Perform operations
    // ...
    
    // Verify state across partitions
    for _, partition := range sim.Partitions() {
        state := sim.GetPartitionState(partition)
        
        // Verify state consistency
        assert.NotNil(t, state)
        assert.True(t, state.IsValid())
    }
}
```

### Consensus Testing

```go
func TestConsensusFailure(t *testing.T) {
    sim := simulator.New(t, 7) // 7 validators
    sim.InitFromGenesis()
    
    // Stop 2 validators (still have majority)
    sim.StopValidator(0)
    sim.StopValidator(1)
    
    // Network should continue
    alice := sim.Partition("BVN0").Account("alice")
    sim.FundAccount(alice, 1000000)
    
    sim.ExecuteBlocks(10)
    
    // Stop 1 more validator (lose majority)
    sim.StopValidator(2)
    
    // Network should halt
    assert.False(t, sim.IsProducingBlocks())
    
    // Restart validator (regain majority)
    sim.StartValidator(2)
    
    // Network should resume
    sim.ExecuteBlocks(5)
    assert.True(t, sim.IsProducingBlocks())
}
```

## Debugging Simulator Tests

### Verbose Logging

```go
func TestWithLogging(t *testing.T) {
    sim := simulator.New(t, 3).WithLogging(true)
    sim.InitFromGenesis()
    
    // Operations will be logged
}
```

### State Inspection

```go
func TestStateInspection(t *testing.T) {
    sim := simulator.New(t, 3)
    sim.InitFromGenesis()
    
    // Inspect state at any point
    state := sim.GetState()
    t.Logf("Current state: %+v", state)
    
    // Inspect specific account
    account := sim.GetAccount("acc://test.acme/alice")
    t.Logf("Alice account: %+v", account)
    
    // Inspect transaction history
    history := sim.GetTransactionHistory("acc://test.acme/alice")
    t.Logf("Transaction history: %+v", history)
}
```

### Step-by-Step Execution

```go
func TestStepByStep(t *testing.T) {
    sim := simulator.New(t, 3)
    sim.InitFromGenesis()
    
    // Execute one block at a time
    for i := 0; i < 10; i++ {
        sim.ExecuteBlock()
        
        // Inspect state after each block
        height := sim.GetBlockHeight()
        t.Logf("Block %d executed", height)
        
        // Check specific conditions
        if height == 5 {
            // Verify intermediate state
        }
    }
}
```

## Best Practices

### 1. Test Organization

```go
// Good: Focused test
func TestCrossPartitionTransfer(t *testing.T) {
    sim := simulator.New(t, 3).WithPartitions(2)
    sim.InitFromGenesis()
    
    // Test specific cross-partition behavior
}

// Good: Comprehensive scenario
func TestComplexWorkflow(t *testing.T) {
    sim := simulator.New(t, 5)
    sim.InitFromGenesis()
    
    // Test complete user workflow
}
```

### 2. Resource Management

```go
func TestWithCleanup(t *testing.T) {
    sim := simulator.New(t, 3)
    defer sim.Close() // Always cleanup
    
    sim.InitFromGenesis()
    
    // Test implementation
}
```

### 3. Deterministic Testing

```go
func TestDeterministic(t *testing.T) {
    // Use fixed seed for reproducible results
    sim := simulator.New(t, 3).WithSeed(12345)
    sim.InitFromGenesis()
    
    // Test will be deterministic
}
```

## Common Issues

### 1. Timing Issues

```go
// Problem: Race condition
sim.Submit(txn)
balance := sim.QueryBalance(account) // May be stale

// Solution: Wait for execution
sim.Submit(txn)
sim.ExecuteBlocks(1)
balance := sim.QueryBalance(account) // Up to date
```

### 2. State Consistency

```go
// Problem: Checking state too early
sim.Submit(crossPartitionTxn)
// State may not be consistent yet

// Solution: Wait for cross-partition completion
sim.Submit(crossPartitionTxn)
sim.ExecuteBlocks(10) // Allow time for cross-partition messages
```

### 3. Resource Limits

```go
// Problem: Too many validators
sim := simulator.New(t, 100) // May be too resource intensive

// Solution: Use appropriate scale
sim := simulator.New(t, 7) // Sufficient for most tests
```

---

## See Also

- [testing.md](testing.md) - Complete testing guide
- [e2e-tests.md](e2e-tests.md) - End-to-end testing guide
- [performance-tests.md](performance-tests.md) - Performance testing guide
- [unit-tests.md](unit-tests.md) - Unit testing guide

*This guide covers simulator-based testing for network behavior and consensus mechanisms.*
