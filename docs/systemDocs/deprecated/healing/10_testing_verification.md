# Testing and Verification

## Introduction

Testing and verification are essential aspects of the healing processes in Accumulate. They ensure that the healing processes work correctly, maintain data integrity, and can handle various edge cases. This document details the testing and verification mechanisms used in the healing processes.

## Test Types

### Unit Tests

The healing processes include unit tests to verify the correctness of individual functions:

```go
// From internal/core/healing/synthetic_test.go
func TestHealSynthetic(t *testing.T) {
    // Set up test environment
    ctx := context.Background()
    
    // Create a mock client
    client := &mockClient{}
    
    // Create test arguments
    args := HealSyntheticArgs{
        Client: client,
        Source: "source",
        Dest: "destination",
        Number: 1,
    }
    
    // Create test sequence info
    si := SequencedInfo{
        Source: "source",
        Destination: "destination",
        Number: 1,
    }
    
    // Create a healer
    healer := &Healer{}
    
    // Call the function being tested
    err := healer.HealSynthetic(ctx, args, si)
    
    // Verify the result
    require.NoError(t, err)
    
    // Verify the client was called correctly
    require.Equal(t, 1, client.callCount)
    require.Equal(t, "source", client.lastSource)
    require.Equal(t, "destination", client.lastDestination)
    require.Equal(t, uint64(1), client.lastNumber)
}
```

These unit tests verify that individual functions work correctly in isolation.

### Integration Tests

The healing processes include integration tests to verify that the healing processes work correctly in a more realistic environment:

```go
// From internal/core/healing/integration_test.go
func TestHealingIntegration(t *testing.T) {
    // Set up test environment
    ctx := context.Background()
    
    // Create a test network
    network := testnetwork.New(t, 3)
    
    // Create a client
    client := network.Client()
    
    // Create a synthetic transaction
    txn := &protocol.Transaction{
        Header: &protocol.TransactionHeader{
            Principal: protocol.AccountUrl("test"),
        },
        Body: &protocol.CreateIdentity{
            Url: protocol.AccountUrl("test"),
        },
    }
    
    // Submit the transaction to the source partition
    sourcePartition := network.Partition("source")
    sourcePartition.Submit(txn)
    
    // Verify the transaction is in the source partition
    require.True(t, sourcePartition.HasTransaction(txn.ID()))
    
    // Verify the transaction is not in the destination partition
    destinationPartition := network.Partition("destination")
    require.False(t, destinationPartition.HasTransaction(txn.ID()))
    
    // Run the healing process
    healer := &Healer{}
    err := healer.HealSynthetic(ctx, HealSyntheticArgs{
        Client: client,
        Source: "source",
        Dest: "destination",
        Number: 1,
    }, SequencedInfo{
        Source: "source",
        Destination: "destination",
        Number: 1,
    })
    require.NoError(t, err)
    
    // Verify the transaction is now in the destination partition
    require.True(t, destinationPartition.HasTransaction(txn.ID()))
}
```

These integration tests verify that the healing processes work correctly in a more realistic environment with multiple partitions.

### Reference Tests

The healing processes include reference tests that serve as the definitive implementation of the healing logic:

```go
// From internal/core/healing/anchor_synth_report_test.go
func TestAnchorSynthReport(t *testing.T) {
    // Set up test environment
    ctx := context.Background()
    
    // Create a test network
    network := testnetwork.New(t, 3)
    
    // Create a client
    client := network.Client()
    
    // Create an anchor
    anchor := &protocol.BlockAnchor{
        Anchor: &protocol.AnchorMetadata{
            Source: protocol.PartitionUrl("source"),
            Destination: protocol.PartitionUrl("destination"),
            Number: 1,
        },
    }
    
    // Submit the anchor to the source partition
    sourcePartition := network.Partition("source")
    sourcePartition.Submit(anchor)
    
    // Verify the anchor is in the source partition
    require.True(t, sourcePartition.HasAnchor(anchor.ID()))
    
    // Verify the anchor is not in the destination partition
    destinationPartition := network.Partition("destination")
    require.False(t, destinationPartition.HasAnchor(anchor.ID()))
    
    // Run the healing process
    err := HealAnchor(ctx, HealAnchorArgs{
        Client: client,
        Source: "source",
        Dest: "destination",
        Number: 1,
    }, SequencedInfo{
        Source: "source",
        Destination: "destination",
        Number: 1,
    })
    require.NoError(t, err)
    
    // Verify the anchor is now in the destination partition
    require.True(t, destinationPartition.HasAnchor(anchor.ID()))
}
```

These reference tests serve as the definitive implementation of the healing logic and are used as a reference for other implementations.

## Test Infrastructure

### Mock Clients

The healing processes use mock clients to simulate network interactions in tests:

```go
// From internal/core/healing/synthetic_test.go
type mockClient struct {
    callCount int
    lastSource string
    lastDestination string
    lastNumber uint64
}

func (c *mockClient) ForAddress(addr multiaddr.Multiaddr) message.AddressedClient {
    return c
}

func (c *mockClient) QueryMessage(ctx context.Context, id *url.TxID, opts *api.QueryOptions) (*api.MessageRecord, error) {
    c.callCount++
    return &api.MessageRecord{
        Status: &protocol.TransactionStatus{
            Code: protocol.StatusDelivered,
        },
    }, nil
}

// Additional mock methods...
```

These mock clients allow tests to control the behavior of network interactions and verify that the healing processes interact with the network correctly.

### Test Networks

The healing processes use test networks to simulate a more realistic environment:

```go
// From internal/core/healing/integration_test.go
func TestHealingIntegration(t *testing.T) {
    // Set up test environment
    ctx := context.Background()
    
    // Create a test network
    network := testnetwork.New(t, 3)
    
    // Create a client
    client := network.Client()
    
    // Additional test code...
}
```

These test networks simulate multiple partitions and allow tests to verify that the healing processes work correctly in a more realistic environment.

### Test Utilities

The healing processes include test utilities to simplify common testing tasks:

```go
// From internal/core/healing/testutil.go
func createTestTransaction() *protocol.Transaction {
    return &protocol.Transaction{
        Header: &protocol.TransactionHeader{
            Principal: protocol.AccountUrl("test"),
        },
        Body: &protocol.CreateIdentity{
            Url: protocol.AccountUrl("test"),
        },
    }
}

func createTestAnchor() *protocol.BlockAnchor {
    return &protocol.BlockAnchor{
        Anchor: &protocol.AnchorMetadata{
            Source: protocol.PartitionUrl("source"),
            Destination: protocol.PartitionUrl("destination"),
            Number: 1,
        },
    }
}

// Additional test utilities...
```

These test utilities simplify common testing tasks and ensure consistency across tests.

## Verification Mechanisms

### Receipt Verification

The healing processes include receipt verification to ensure that receipts are valid:

```go
// From internal/core/healing/synthetic.go
// Verify the receipt
err = receipt.Verify()
if err != nil {
    return nil, errors.UnknownError.WithFormat(
        "verify receipt: %w", err)
}
```

This verification ensures that the receipt is cryptographically valid and provides a proper proof of transaction inclusion.

### Signature Verification

The healing processes include signature verification to ensure that signatures are valid:

```go
// From internal/core/healing/anchors.go
// Filter out bad signatures
if !sig.Verify(nil, seq) {
    slog.ErrorContext(ctx, "Node gave us an invalid signature", "id", info)
    continue
}
```

This verification ensures that only valid signatures are included in the healed transaction or anchor.

### Transaction Verification

The healing processes include transaction verification to ensure that transactions are valid:

```go
// From internal/core/healing/synthetic.go
// Verify the transaction
err = txn.Verify()
if err != nil {
    return errors.UnknownError.WithFormat(
        "verify transaction: %w", err)
}
```

This verification ensures that the transaction is valid and can be processed by the network.

## Data Integrity Verification

### Reference Implementation

The healing processes follow a reference implementation to ensure data integrity:

```go
// From internal/core/healing/anchor_synth_report_test.go
func TestAnchorSynthReport(t *testing.T) {
    // Test code...
}
```

This reference implementation serves as the definitive implementation of the healing logic and is used as a reference for other implementations.

### No Data Fabrication

The healing processes follow strict rules to ensure that they do not fabricate or fake any data:

```go
// From internal/core/healing/synthetic.go
// Query the synthetic transaction
r, err := ResolveSequenced[messaging.Message](ctx, args.Client, args.NetInfo, si.Source, si.Destination, si.Number, false)
if err != nil {
    return err // Return the error, don't fabricate data
}
```

These rules are critical for maintaining the integrity of the healing process and ensuring that it does not mask errors by fabricating data.

### On-Demand Transaction Fetching

The healing processes implement on-demand transaction fetching to avoid loading all transactions upfront:

```go
// From internal/core/healing/synthetic.go
// Try to get the transaction from the known transactions map
if txn, ok := args.TxnMap[txid.Hash()]; ok {
    return txn, nil
}

// If not in the map, fetch it from the network
// ...
```

This approach is particularly important for the anchor healing process, which may need to access a large number of transactions.

## Command-Line Testing

### Manual Testing

The healing processes can be tested manually using the command-line interface:

```bash
# Test synthetic healing
go run ./tools/cmd/debug heal-synth testnet

# Test anchor healing
go run ./tools/cmd/debug heal-anchor testnet
```

This manual testing allows operators to verify that the healing processes work correctly in a real environment.

### Automated Testing

The healing processes include automated tests that can be run from the command line:

```bash
# Run all tests
go test ./internal/core/healing/...

# Run specific tests
go test ./internal/core/healing -run TestHealSynthetic
```

These automated tests ensure that the healing processes work correctly and maintain data integrity.

## Continuous Integration

### CI Pipeline

The healing processes are included in the continuous integration pipeline to ensure that they continue to work correctly as the codebase evolves:

```yaml
# From .gitlab-ci.yml
test:
  stage: test
  script:
    - go test ./internal/core/healing/...
```

This CI pipeline ensures that the healing processes are tested regularly and that any issues are caught early.

### Test Coverage

The healing processes include test coverage metrics to ensure that the code is well-tested:

```bash
# Generate test coverage
go test ./internal/core/healing/... -coverprofile=coverage.out

# View test coverage
go tool cover -html=coverage.out
```

These test coverage metrics help identify areas of the code that need additional testing.

## Recent Changes

Recent changes to the testing and verification mechanisms include:

1. **Enhanced Reference Tests**: Improved reference tests to better define the expected behavior of the healing processes
2. **On-Demand Transaction Fetching**: Implemented on-demand transaction fetching to avoid loading all transactions upfront
3. **Improved Error Handling**: Enhanced error handling to better handle various error conditions

## Best Practices

When working with testing and verification in the healing processes, it's important to follow these best practices:

1. **Follow the Reference Implementation**: Always follow the reference implementation in `anchor_synth_report_test.go`
2. **Verify Receipts and Signatures**: Always verify receipts and signatures to ensure they are valid
3. **Do Not Fabricate Data**: Never fabricate or fake data that isn't available from the network
4. **Use On-Demand Transaction Fetching**: Use on-demand transaction fetching to avoid loading all transactions upfront
5. **Include Comprehensive Tests**: Include unit tests, integration tests, and reference tests to ensure comprehensive coverage

```go
// Example of following the reference implementation
// From internal/core/healing/synthetic.go
// Query the synthetic transaction
r, err := ResolveSequenced[messaging.Message](ctx, args.Client, args.NetInfo, si.Source, si.Destination, si.Number, false)
if err != nil {
    return err // Return the error, don't fabricate data
}
```

## Critical Rules

The healing processes follow several critical rules to ensure data integrity:

1. **Data Retrieved from the Protocol CANNOT be Faked**: Doing so masks errors and leads to huge wastes of time for those monitoring the Network
2. **Follow the Reference Test Code Exactly**: The only acceptable implementation is to follow exactly what's in the reference test code (`anchor_synth_report_test.go`)
3. **Implement Proper Fallback Mechanisms**: Implement fallback mechanisms as defined in the reference test code
4. **Add Detailed Logging**: Add detailed logging to track when transactions are fetched on-demand

These rules are critical for maintaining the integrity of the healing process and ensuring that it does not mask errors by fabricating data.

## Conclusion

Testing and verification are essential aspects of the healing processes in Accumulate. By using unit tests, integration tests, reference tests, and various verification mechanisms, the healing processes can ensure that they work correctly, maintain data integrity, and can handle various edge cases.

This document concludes our comprehensive documentation of the healing processes in Accumulate. We have covered the problem definition, APIs, synthetic transaction healing, anchor healing, API layers, database and caching infrastructure, receipt creation and signature gathering, reporting system, error handling, and testing and verification. This documentation provides a solid foundation for understanding and working with the healing processes in Accumulate.
