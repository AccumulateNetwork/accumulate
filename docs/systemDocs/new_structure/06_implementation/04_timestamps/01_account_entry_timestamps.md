# Timestamps in Accumulate

## Metadata
- **Document Type**: Technical Implementation
- **Version**: 1.0
- **Last Updated**: 2025-05-17
- **Related Components**: Block Processing, Transaction Execution, Consensus

## 1. Introduction

Timestamps play a critical role in the Accumulate blockchain, serving multiple purposes including transaction ordering, state validation, and data integrity verification. This document details how timestamps are created, managed, and used throughout the system, with a particular focus on account entries.

## 2. Timestamp Sources in Accumulate

Accumulate uses several types of timestamps throughout its architecture:

### 2.1 Block Timestamps

Block timestamps are the primary source of time in the Accumulate system. They are determined by the consensus mechanism (CometBFT) during block creation.

```go
// Block parameters passed to Begin
type BlockParams struct {
    Height    uint64
    Time      time.Time
    AppHash   []byte
    LastBlock [32]byte
}
```

Key characteristics of block timestamps:
- Determined by the consensus mechanism
- Monotonically increasing (each block has a later timestamp than the previous)
- Used as the authoritative time reference for all transactions in the block
- Represented as UTC time

### 2.2 Transaction Timestamps

Transaction timestamps are used in two contexts:

1. **Signature Timestamps**: When a transaction is signed
2. **Execution Timestamps**: When a transaction is executed and added to an account

```go
// Signature timestamp is part of the signature
type Signature struct {
    // ...
    Timestamp uint64
    // ...
}
```

## 3. Account Entry Timestamp Creation

When a transaction is processed and results in an account entry, the timestamp is derived from the block time, not the transaction's signature time. This is a crucial distinction.

### 3.1 Execution Flow

The process for creating account entry timestamps follows this flow:

1. A transaction is submitted to the network with a signature timestamp
2. The transaction is included in a block with a specific block time
3. During execution, the block time is used as the timestamp for all account entries
4. The account entry is stored with this block-derived timestamp

```go
// Simplified execution context
type TransactionContext struct {
    // ...
    BlockTime time.Time
    // ...
}

// During transaction execution, the BlockTime is used for all state changes
func (x *TransactionContext) processTransaction(tx *protocol.Transaction) {
    // ...
    // All account entries created during this transaction
    // will use x.BlockTime as their timestamp
    // ...
}
```

### 3.2 Chain Entry Records

When transactions are added to chains (a fundamental data structure in Accumulate), they create chain entries that include timestamps:

```go
// Chain entry with timestamp
type ChainEntry struct {
    // ...
    BlockTime time.Time
    // ...
}
```

The timestamp for these entries is always the block time, ensuring consistency across the system.

## 4. Timestamp Validation

Accumulate implements several validation mechanisms for timestamps:

### 4.1 Signature Timestamp Validation

Signature timestamps are validated to ensure they are:
- Not too far in the past (prevents replay attacks)
- Not too far in the future (prevents time manipulation)

```go
// Simplified validation logic
func validateSignatureTimestamp(timestamp uint64, blockTime time.Time) error {
    // Convert to time.Time
    signatureTime := time.Unix(0, int64(timestamp))
    
    // Check if too old
    if blockTime.Sub(signatureTime) > maxTimestampAge {
        return errors.New("signature timestamp too old")
    }
    
    // Check if too far in the future
    if signatureTime.Sub(blockTime) > maxTimestampFuture {
        return errors.New("signature timestamp too far in the future")
    }
    
    return nil
}
```

### 4.2 Block Timestamp Validation

Block timestamps are validated by the consensus mechanism to ensure they are:
- Later than the previous block's timestamp
- Not too far in the future compared to the validators' local time

This validation is primarily handled by CometBFT, with additional checks in Accumulate's execution layer.

## 5. Major Block Timestamps

Accumulate uses a concept of "major blocks" for cross-chain coordination. Major block timestamps follow a predetermined schedule:

```go
// Major block scheduling
nextBlockTime := b.Executor.globals.Active.MajorBlockSchedule().Next(anchor.MajorBlockTime)
if blockTimeUTC.IsZero() || blockTimeUTC.Before(nextBlockTime) {
    // Not time for a major block yet
    return false
}
```

Major block timestamps are critical for:
- Coordinating state across multiple chains
- Determining when anchors should be created
- Synchronizing the network's overall state

## 6. Timestamp Precision and Format

Accumulate uses different timestamp formats depending on the context:

### 6.1 Internal Representation

- Block times: `time.Time` (Go's time representation)
- Signature timestamps: `uint64` (nanoseconds since Unix epoch)

### 6.2 Storage Format

When persisted to the database, timestamps are stored in a standardized format:
- Binary: 8-byte integer (nanoseconds since Unix epoch)
- JSON: ISO 8601 format (e.g., "2025-05-17T10:30:00Z")

## 7. Timestamp Querying

Timestamps can be queried through the API:

```go
// Query for transaction timestamp
func (s *TimestampService) GetTimestamp(ctx context.Context, id *url.TxID) (*MessageData, error) {
    // ...
    // Retrieves the timestamp of a transaction
    // ...
}
```

This allows clients to verify when transactions were processed and included in the blockchain.

## 8. Special Cases and Considerations

### 8.1 Genesis Block

The genesis block has a special timestamp handling:
- Set during network initialization
- Used as the starting point for the network's timeline
- All initial accounts use this timestamp

### 8.2 Synthetic Transactions

Synthetic transactions (system-generated transactions) use the block time of the block in which they are generated:

```go
// Creating a synthetic transaction
syntheticTx := &protocol.Transaction{
    Header: &protocol.TransactionHeader{
        Principal: destination,
    },
    Body: body,
}

// The timestamp will be set to the current block time during execution
```

### 8.3 Transaction Expiration

Transactions can specify an expiration time, after which they will not be processed:

```go
// Transaction expiration based on block time
if tx.Header.Expiration > 0 && uint64(blockTime.Unix()) > tx.Header.Expiration {
    return errors.New("transaction expired")
}
```

## 9. Timestamp Consistency

Maintaining timestamp consistency is critical for the integrity of the Accumulate blockchain:

1. **Block Time Monotonicity**: Each block's timestamp must be later than the previous block
2. **Cross-Chain Consistency**: When anchoring between chains, timestamps must be consistent
3. **Validator Time Synchronization**: Validators should maintain synchronized clocks (using NTP or similar)

## 10. Best Practices for Developers

When working with timestamps in Accumulate:

1. **Always Use Block Time**: For any state changes, always use the block time, not the transaction signature time
2. **UTC Standardization**: Always work with UTC times to avoid timezone issues
3. **Time Comparisons**: When comparing times, be aware of precision issues and use appropriate methods
4. **Expiration Setting**: Set reasonable expiration times for transactions based on expected network conditions

## 11. Conclusion

Timestamps in Accumulate serve as a critical component for maintaining the integrity and ordering of the blockchain. By using block times as the authoritative source for account entry timestamps, Accumulate ensures consistency across its distributed system while still allowing for flexible transaction submission with signature timestamps.

The distinction between signature timestamps (when a transaction was created) and block timestamps (when a transaction was executed) is fundamental to understanding how time works in the Accumulate blockchain.

## Related Documents

- [Block Processing](../02_block_processing.md)
- [Transaction Execution](../03_transaction_execution.md)
- [Consensus Integration](../../04_network/05_consensus/01_overview.md)
