---
title: Accumulate Simulator Developer Guide
description: Comprehensive guide for setting up and using the Accumulate network simulator
tags: [accumulate, testing, simulator, development, network]
created: 2025-05-17
version: 1.0
---

# Accumulate Simulator Developer Guide

## 1. Introduction

The Accumulate simulator is a powerful tool for developers to test and validate their code in a controlled environment. It provides a simulated Accumulate network that behaves similarly to a production network but with deterministic execution and simplified consensus. This guide covers how to set up, configure, and use the simulator effectively, as well as understanding its limitations.

## 2. Overview of the Simulator

### 2.1 Purpose and Capabilities

The Accumulate simulator allows developers to:

- Test transaction processing in a controlled environment
- Validate protocol changes without deploying to a testnet
- Debug complex interactions between network components
- Simulate various network configurations and topologies
- Reproduce and diagnose issues in a deterministic way
- Run automated tests against a simulated network

### 2.2 Architecture

The simulator implements a simplified version of the Accumulate network with:

- Multiple BVNs and DNs in configurable topologies
- Simulated consensus mechanism
- In-memory or persistent database options
- API endpoints that match production behavior
- Transaction validation and execution
- Synthetic time control for deterministic testing

## 3. Setting Up the Simulator

### 3.1 Installation

The simulator is included in the Accumulate repository. To build it:

```bash
# From the root of the Accumulate repository
make simulator
```

This will create a `simulator` binary in your build directory.

### 3.2 Basic Usage

To run the simulator with default settings:

```bash
./simulator
```

This will start a simulated network with:
- 3 BVNs with 3 validators each
- 1 DN with 3 validators
- In-memory database (data will be lost when the simulator stops)
- Default genesis time and parameters

### 3.3 Command-Line Options

The simulator supports various command-line options:

```bash
# Start with a specific network configuration
./simulator --bvns=2 --validators=4

# Use persistent storage
./simulator --database=/path/to/data

# Control simulation speed
./simulator --step=100ms

# Set executor version
./simulator --globals='{"executorVersion": "v2vandenberg"}'

# Run with specific network topology
./simulator --network=custom --topology=/path/to/topology.json
```

Common options include:

| Option | Description | Default |
|--------|-------------|---------|
| `--bvns`, `-b` | Number of BVNs | 3 |
| `--validators`, `-v` | Number of validators per partition | 3 |
| `--step`, `-s` | Time step for simulation | "on-wait" |
| `--database` | Path for persistent database | "" (in-memory) |
| `--globals` | JSON string of global parameters | {} |
| `--network` | Network type (simple, local, custom) | "simple" |
| `--listen` | Address to listen on for API requests | ":8080" |

## 4. Using the Simulator for Development

### 4.1 Connecting to the Simulator

Once running, the simulator exposes API endpoints that match the production Accumulate network:

```bash
# Query the status of the network
curl http://localhost:8080/v2 -d '{"jsonrpc":"2.0","id":1,"method":"describe","params":{}}'

# Submit a transaction
curl http://localhost:8080/v2 -d '{"jsonrpc":"2.0","id":1,"method":"submit","params":{"transaction":"..."}}'
```

You can use the standard Accumulate CLI by pointing it to the simulator:

```bash
accumulate --url http://localhost:8080/v2 account list
```

### 4.2 Programmatic Usage

The simulator can be used programmatically in Go tests:

```go
import (
    "testing"
    "gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

func TestMyFeature(t *testing.T) {
    // Create a simulated network
    sim := simulator.New(
        simulator.SimpleNetwork(t.Name(), 3, 3),
        simulator.Genesis(time.Now()),
    )
    
    // Start the simulator
    err := sim.Start(context.Background())
    require.NoError(t, err)
    defer sim.Stop()
    
    // Use the simulator
    client := sim.GetClient("BVN0")
    
    // Test your code...
}
```

### 4.3 Time Control

The simulator provides control over time progression, which is useful for testing time-dependent behavior:

```go
// Step the simulator forward by one block
sim.Step()

// Step the simulator until a specific condition is met
sim.StepUntil(func() bool {
    // Return true when the condition is met
    return someCondition
})
```

When using the `--step=on-wait` option (default), the simulator will automatically step forward when an API call is waiting for a transaction to be processed.

## 5. Advanced Configuration

### 5.1 Custom Network Topologies

You can define custom network topologies for more complex testing scenarios:

```go
// Programmatic configuration
network := simulator.NewNetworkFromTemplate(simulator.NetworkTemplate{
    DNCount:       1,
    BVNCount:      4,
    ValidatorCount: 5,
    FollowerCount:  2,
})
```

### 5.2 Database Options

The simulator supports different database backends:

```go
// In-memory database (default)
simulator.MemoryDatabase()

// Badger database (persistent)
simulator.BadgerDatabaseFromDirectory("/path/to/data", errorHandler)

// Overlay database (for testing database migrations)
simulator.OverlayDatabase(baseOpener, overlayOpener)
```

### 5.3 Genesis Configuration

You can customize the genesis state of the network:

```go
// Create custom accounts at genesis
alice := &protocol.LiteIdentity{
    Url: protocol.AccountUrl("alice"),
    // ...
}

sim := simulator.New(
    simulator.SimpleNetwork(t.Name(), 1, 1),
    simulator.Genesis(genesisTime).With(alice),
)
```

## 6. Testing Strategies

### 6.1 Unit Testing with the Simulator

The simulator is particularly useful for unit testing components that interact with the Accumulate network:

```go
func TestTransaction(t *testing.T) {
    // Set up simulator
    sim := setupSimulator(t)
    
    // Create and submit a transaction
    tx := createTestTransaction()
    txid, err := sim.SubmitTx(tx)
    require.NoError(t, err)
    
    // Wait for the transaction to be processed
    receipt, err := sim.WaitForTx(txid)
    require.NoError(t, err)
    
    // Verify the result
    assert.Equal(t, protocol.TxStatusSuccess, receipt.Status)
}
```

### 6.2 Integration Testing

For integration testing, you can use the simulator to test interactions between multiple components:

```go
func TestIntegration(t *testing.T) {
    // Set up simulator with multiple BVNs
    sim := simulator.New(
        simulator.SimpleNetwork(t.Name(), 3, 3),
        simulator.Genesis(time.Now()),
    )
    
    // Start services that interact with the network
    service1 := startService(sim.GetClient("BVN0"))
    service2 := startService(sim.GetClient("BVN1"))
    
    // Test interactions between services
    // ...
}
```

### 6.3 Debugging Tips

When debugging issues with the simulator:

1. Use the `--debug` flag to enable detailed logging
2. Examine the state of accounts after each transaction
3. Use `sim.DumpState()` to get a snapshot of the network state
4. Step through the simulation manually to isolate issues
5. Use the recorder to capture and replay transaction sequences

## 7. Simulator Limitations

### 7.1 Consensus Differences

The simulator uses a simplified consensus model that differs from the production network:

- **Deterministic ordering**: Transactions are processed in a deterministic order, which may hide race conditions
- **Simplified validation**: Some validation steps are simplified for performance
- **No network delays**: The simulator doesn't model network latency and message delays
- **Perfect synchronization**: All validators are perfectly synchronized, unlike real networks

### 7.2 Performance Characteristics

The simulator's performance characteristics differ from production:

- **Resource usage**: The simulator runs all partitions in a single process, which can be memory-intensive
- **Transaction throughput**: The simulator may process transactions faster or slower than production
- **Block times**: Block production is controlled by the step interval, not real consensus timing

### 7.3 Feature Limitations

Some features have limitations in the simulator:

- **P2P networking**: The P2P layer is simulated and doesn't use actual network connections
- **Signature validation**: Some signature validation may be simplified
- **External services**: Integration with external services may require mocking
- **Healing processes**: Cross-partition healing may behave differently
- **Snapshot/restore**: Snapshot and restore functionality may have limitations

## 8. Best Practices

### 8.1 Effective Simulator Usage

To get the most out of the simulator:

1. **Start simple**: Begin with minimal network configurations
2. **Isolate tests**: Each test should use its own simulator instance
3. **Control time**: Use explicit time control for deterministic testing
4. **Clean up**: Always stop the simulator after tests complete
5. **Use persistent storage** for debugging complex issues
6. **Verify in testnet**: Confirm critical behavior in a testnet before production

### 8.2 Common Pitfalls

Watch out for these common issues:

1. **Resource exhaustion**: Running too many simulator instances simultaneously
2. **Determinism assumptions**: Assuming behavior will match production exactly
3. **Time-dependent bugs**: Issues that only appear with specific timing
4. **Database corruption**: Not properly cleaning up between test runs
5. **Configuration mismatches**: Using simulator configs that don't match production

## 9. Examples

### 9.1 Basic Test Example

```go
func TestBasicTransaction(t *testing.T) {
    // Create a simple network with 1 BVN and 1 validator
    sim := simulator.New(
        simulator.SimpleNetwork(t.Name(), 1, 1),
        simulator.Genesis(time.Now()),
    )
    
    // Start the simulator
    err := sim.Start(context.Background())
    require.NoError(t, err)
    defer sim.Stop()
    
    // Get a client for the BVN
    client := sim.GetClient("BVN0")
    
    // Create a lite account
    liteAccount, err := protocol.LiteTokenAccountFromKey(acctesting.GenerateKey(), protocol.ACME)
    require.NoError(t, err)
    
    // Fund the account
    faucet := sim.GetFaucet()
    err = faucet.FundAccount(liteAccount.Url(), protocol.NewAmount(100, 0))
    require.NoError(t, err)
    
    // Create a transaction
    tx := &protocol.Transaction{
        Header: &protocol.TransactionHeader{
            Principal: liteAccount.Url(),
        },
        Body: &protocol.SendTokens{
            To: []*protocol.TokenRecipient{
                {
                    Url:    protocol.AccountUrl("recipient"),
                    Amount: protocol.NewAmount(50, 0),
                },
            },
        },
    }
    
    // Sign and submit the transaction
    signer := acctesting.NewSigner(liteAccount)
    err = signer.Sign(tx)
    require.NoError(t, err)
    
    txid, err := client.Submit(tx)
    require.NoError(t, err)
    
    // Wait for the transaction to be processed
    receipt, err := client.WaitForTx(txid)
    require.NoError(t, err)
    
    // Verify the result
    assert.Equal(t, protocol.TxStatusSuccess, receipt.Status)
}
```

### 9.2 Advanced Configuration Example

```go
func TestAdvancedConfiguration(t *testing.T) {
    // Define a custom network topology
    network := simulator.NewNetworkFromTemplate(simulator.NetworkTemplate{
        DNCount:        1,
        BVNCount:       2,
        ValidatorCount: 3,
        FollowerCount:  1,
    })
    
    // Set up persistent storage
    dbDir := t.TempDir()
    dbOpener := simulator.BadgerDbOpener(dbDir, func(err error) {
        require.NoError(t, err)
    })
    
    // Create custom accounts for genesis
    alice := &protocol.LiteIdentity{
        Url: protocol.AccountUrl("alice"),
        // ...
    }
    
    // Configure global parameters
    globals := &protocol.NetworkGlobals{
        ExecutorVersion: "v2vandenberg",
        Limits: &protocol.NetworkLimits{
            // Custom limits
        },
    }
    
    // Create the simulator with all options
    sim := simulator.New(
        network,
        simulator.GenesisWith(time.Now(), globals).With(alice),
        simulator.WithDatabase(dbOpener),
        simulator.StepOnWait(),
    )
    
    // Start the simulator
    err := sim.Start(context.Background())
    require.NoError(t, err)
    defer sim.Stop()
    
    // Use the simulator
    // ...
}
```

## 10. Troubleshooting

### 10.1 Common Errors

| Error | Possible Cause | Solution |
|-------|----------------|----------|
| "Failed to start simulator" | Resource contention | Close other simulator instances |
| "Transaction validation failed" | Incorrect transaction format | Check transaction structure and signatures |
| "Database error" | Corrupted database | Use a fresh database directory |
| "Network configuration error" | Invalid topology | Check network configuration parameters |
| "Out of memory" | Too many partitions or validators | Reduce network size or increase memory limits |

### 10.2 Debugging Tools

The simulator provides several debugging tools:

```go
// Print the current state of the network
sim.DumpState()

// Enable detailed logging
sim.SetLogLevel(log.LevelDebug)

// Record a simulation for later replay
recorder := simulator.NewRecorder(sim)
recorder.Start()
// ... run your test ...
recording := recorder.Stop()
recorder.SaveToFile("simulation.rec")

// Replay a recorded simulation
replay := simulator.LoadFromFile("simulation.rec")
replay.Start()
```

## 11. Conclusion

The Accumulate simulator is a powerful tool for testing and validating your code against a simulated Accumulate network. While it has some limitations compared to a production network, it provides a controlled, deterministic environment for development and testing.

By understanding how to configure and use the simulator effectively, you can develop more robust applications and protocol changes with confidence before deploying to testnets or production.

## 12. Related Documentation

- [Testing Overview](./01_testing_overview.md)
- [Test Environments](./03_test_environments.md)
- [Automated Testing](./04_automated_testing.md)
- [Network Architecture](../../04_network/00_index.md)
- [API Documentation](../../05_apis/01_overview.md)
