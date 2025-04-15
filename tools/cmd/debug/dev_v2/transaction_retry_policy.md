# Transaction Healing Process

## Overview

The Accumulate Network implements a continuous healing process that runs alongside the blockchain to ensure data consistency across all partitions. This document clarifies the design, implementation, and expected behavior of this system.

## Healing Process Flow

The heal-all process follows a specific flow designed to ensure all gaps are eventually healed while respecting network resources. The process runs continuously alongside the blockchain, processing one transaction at a time per partition pair to avoid overwhelming the network.

### Core Healing Flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         MAIN HEALING LOOP                                │
└─────────────────────────────────────────────────────────────────────────┘
   │
   ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ FOR EACH PARTITION PAIR:                                                 │
│   ┌─────────────────────────────────────────────────────────────────────┐
│   │ 1. CHECK FOR ANCHOR GAPS                                            │
│   │    ├── If gap found:                                                │
│   │    │   ├── Get the first anchor that needs healing                  │
│   │    │   ├── Check if we have an existing transaction for this entry  │
│   │    │   │   ├── NO: Create a new healing transaction and submit it   │
│   │    │   │   └── YES: Check submission count                          │
│   │    │   │       ├── < 10: Submit again, increment count              │
│   │    │   │       └── ≥ 10: Create new synthetic tx and submit it      │
│   │    │   └── Move to next partition pair                              │
│   │    └── If no gap, proceed to synthetic check                        │
│   │                                                                     │
│   │ 2. CHECK FOR SYNTHETIC TRANSACTION GAPS                             │
│   │    ├── If gap found:                                                │
│   │    │   ├── Get the first synthetic tx that needs healing            │
│   │    │   ├── Check if we have an existing transaction for this entry  │
│   │    │   │   ├── NO: Create a new healing transaction and submit it   │
│   │    │   │   └── YES: Check submission count                          │
│   │    │   │       ├── < 10: Submit again, increment count              │
│   │    │   │       └── ≥ 10: Create new synthetic tx and submit it      │
│   │    │   └── Move to next partition pair                              │
│   │    └── If no gap, move to next partition pair                       │
│   └─────────────────────────────────────────────────────────────────────┘
└─────────────────────────────────────────────────────────────────────────┘
   │
   ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ PAUSE FOR CONTROLLED PACING                                              │
│   ├── If healing occurred: Pause for shorter duration (e.g., 5 seconds)  │
│   └── If no healing needed: Pause for longer duration (e.g., 30 seconds) │
└─────────────────────────────────────────────────────────────────────────┘
   │
   └─────────────────────────────────────────────────┐
                                                     │
                                                     ▼
                                                   REPEAT
```

### Key Principles

1. **Persistence**: Every entry must be healed - there are no permanent failures
2. **Network Respect**: Only one submission attempt per transaction per healing cycle
3. **Fresh Approach**: After 10 failed attempts, create a synthetic transaction as an alternative approach
4. **Controlled Pacing**: Appropriate pauses between healing cycles to limit network load
5. **Prioritization**: Process the lowest sequence numbers first to ensure logical healing order

```
┌───────────────────────────────────────────────────────────────┐
│                      Healing Process                           │
└───────────────────────────────────────────────────────────────┘
    │
    ▼
┌───────────────────────────────────────────────────────────────┐
│                      Healing Cycles (10 max)                   │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │                     Healing Cycle 1                      │  │
│  │  ┌─────────────────┐         ┌─────────────────┐        │  │
│  │  │  Submission 1   │         │  Submission 2   │        │  │
│  │  │  (Primary Node) │─failed─▶│  (Backup Node)  │        │  │
│  │  └─────────────────┘         └─────────────────┘        │  │
│  └─────────────────────────────────────────────────────────┘  │
│                              │                                 │
│                           failed                               │
│                              │                                 │
│                              ▼                                 │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │                     Healing Cycle 2                      │  │
│  │  ┌─────────────────┐         ┌─────────────────┐        │  │
│  │  │  Submission 1   │         │  Submission 2   │        │  │
│  │  │  (Primary Node) │─failed─▶│  (Backup Node)  │        │  │
│  │  └─────────────────┘         └─────────────────┘        │  │
│  └─────────────────────────────────────────────────────────┘  │
│                              │                                 │
│                              ▼                                 │
│                            . . .                               │
│                              ▼                                 │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │                     Healing Cycle 10                     │  │
│  │  ┌─────────────────┐         ┌─────────────────┐        │  │
│  │  │  Submission 1   │         │  Submission 2   │        │  │
│  │  │  (Primary Node) │─failed─▶│  (Backup Node)  │        │  │
│  │  └─────────────────┘         └─────────────────┘        │  │
│  └─────────────────────────────────────────────────────────┘  │
└───────────────────────────────────────────────────────────────┘
                              │
                           failed
                              │
                              ▼
┌───────────────────────────────────────────────────────────────┐
│                Transaction Abandoned & Logged                  │
└───────────────────────────────────────────────────────────────┘
```

### Tier 1: Per-Submission Retry Limit (2 attempts per cycle)

**Purpose**: Prevents overwhelming individual peers and triggering P2P backoff mechanisms

**Implementation Details**:
- Each transaction is attempted at most **twice** within a single healing cycle
- First attempt uses the primary node for the target partition
- Second attempt (only if first fails) uses at most one additional peer with the best success history
- If both attempts fail, the transaction is not retried again within the same cycle
- The transaction will be retried in subsequent healing cycles until success or the tier-2 limit is reached

**Code Implementation**:
```go
// In stableNodeSubmitLoop (simplified)
// Try primary node first
if primaryNode, hasPrimary := primaryNodes[targetPartition]; hasPrimary {
    submitted, err := attemptSubmission(msg, primaryNode)
    if submitted {
        // Success with primary node
        continue
    }
}

// If primary node submission failed, try at most ONE additional peer
if !submitted {
    // Find the peer with the highest success count
    bestPeer := findBestPeer(targetPartition)
    submitted, err := attemptSubmission(msg, bestPeer)
}

// No more attempts in this cycle if still not submitted
```

### Tier 2: Overall Healing Cycle Limit (10 cycles max)

**Purpose**: Prevents endlessly retrying transactions that consistently fail across multiple cycles

**Implementation Details**:
- A transaction can be attempted across a maximum of **10 healing cycles**
- The `AttemptCount` field in `HealingTransaction` tracks the number of cycles in which a transaction has been attempted
- After 10 cycles with no success, the transaction is removed from the tracker and logged as a permanent failure
- This prevents the system from getting stuck on problematic transactions

**Code Implementation**:
```go
// From heal_transaction_tracking.go
const maxSubmissionAttempts = 10

// In UpdateTransactionStatus
// If submitted successfully or too many attempts, remove from queue
if submitted || tx.AttemptCount >= maxSubmissionAttempts {
    // Remove this entry and update stats
    // ...
    
    // Log if giving up
    if !submitted && tx.AttemptCount >= maxSubmissionAttempts {
        slog.Warn("Giving up on transaction after max attempts",
            "source", tx.SourcePartition,
            "destination", tx.DestPartition,
            "sequence", tx.SequenceNumber,
            "attempts", tx.AttemptCount)
    }
}
```

## Transaction Lifecycle

### 1. Creation Phase

When a gap is detected in the sequence between partitions:

```go
// Create a new transaction
slog.InfoContext(ctx, "Creating new transaction", 
    "source", src.ID, "destination", dst.ID, "sequence", gap.Number)

// Use the existing healSingleAnchor function
h.healSingle(h, src, dst, gap.Number, nil)

// Create a transaction tracking entry
tx := CreateHealingTransaction(envelope, src.ID, dst.ID, gap.Number, isAnchor)
h.txTracker.AddTransaction(tx)
```

### 2. Submission Phase (Per Cycle)

For each healing cycle:

1. Check if we have an existing transaction for this gap
2. If found and `AttemptCount < 10`, reuse it
3. Otherwise, create a new transaction
4. Attempt submission up to 2 times (primary and backup nodes)
5. Update transaction status based on the result

```go
// Check if we have an existing transaction for this gap
existingTx := h.txTracker.FindTransactionForGap(src.ID, dst.ID, gap.Number)

if existingTx != nil && existingTx.Status == TxStatusPending && existingTx.AttemptCount < 10 {
    // Reuse the existing transaction
    slog.InfoContext(ctx, "Reusing existing transaction", 
        "source", src.ID, "destination", dst.ID, "sequence", gap.Number, 
        "attempts", existingTx.AttemptCount)
    
    // Submit the existing transaction (up to 2 attempts via submitLoop)
    if !pretend {
        h.submit <- existingTx.Envelope.Messages
    }
    
    // Update the transaction status
    h.txTracker.UpdateTransactionStatus(existingTx, true, nil)
}
```

### 3. Tracking Phase

The `TransactionTracker` maintains the state of all transactions:

```go
// TransactionTracker manages healing transactions between partitions
type TransactionTracker struct {
    // Map of partition pairs to ordered queues of transactions
    // Key format: "sourcePartition:destinationPartition"
    txMap map[string][]*HealingTransaction
    
    // Statistics for monitoring and reporting
    stats TransactionStats
    
    // Thread safety
    mutex sync.RWMutex
}

// HealingTransaction represents a transaction that needs to be sent from one partition to another
type HealingTransaction struct {
    // Core transaction data
    Envelope        *messaging.Envelope
    SourcePartition string
    DestPartition   string
    DestinationUrl  *url.URL
    SequenceNumber  uint64

    // Tracking information
    FirstAttemptTime time.Time
    LastAttemptTime  time.Time
    AttemptCount     int    // Tracks healing cycles, not individual submissions
    LastError        error

    // Status tracking
    Status          HealingTxStatus
    // ... other fields ...
}
```

### 4. Completion Phase

A transaction is removed from tracking when:

1. It is successfully submitted (status updated to `TxStatusSubmitted`)
2. It reaches the maximum number of healing cycles (`AttemptCount >= maxSubmissionAttempts`)
3. It is confirmed at the destination (status updated to `TxStatusConfirmed`)

## Current Implementation Behavior

In the current implementation:

1. The `AttemptCount` is incremented in `UpdateTransactionStatus` for each submission attempt:
   ```go
   // Update attempt information
   tx.AttemptCount++
   tx.LastAttemptTime = time.Now()
   tx.LastError = err
   ```

2. This means a transaction could have its `AttemptCount` incremented twice within a single healing cycle (once for primary node, once for backup node).

3. With a limit of `maxSubmissionAttempts = 10`, this effectively allows for approximately 5 healing cycles with 2 attempts each, rather than 10 full healing cycles.

## Implementation Recommendations

To align with the design intent of allowing 10 full healing cycles:

1. **Option 1**: Keep the current implementation but update `maxSubmissionAttempts` to 20 to allow for 10 full cycles with 2 attempts each.

2. **Option 2**: Modify the implementation to increment `AttemptCount` only once per healing cycle, regardless of how many submission attempts are made within that cycle.

## Troubleshooting Guide

### Common Issues

1. **Transactions being retried more than expected**
   - Check if `AttemptCount` is being incremented correctly
   - Verify the transaction is being properly removed after successful submission

2. **Transactions abandoned too early**
   - Check if `maxSubmissionAttempts` is set appropriately
   - Verify the transaction status is being updated correctly

3. **Transactions never being removed**
   - Ensure the removal logic in `UpdateTransactionStatus` is working
   - Check if transactions are being properly marked as submitted or failed

### Debugging Steps

1. **Log Analysis**:
   ```
   # Look for transaction creation
   "Creating new transaction" source=Apollo destination=Directory sequence=123
   
   # Look for transaction reuse
   "Reusing existing transaction" source=Apollo destination=Directory sequence=123 attempts=3
   
   # Look for submission results
   "Transaction submitted successfully" source=Apollo destination=Directory sequence=123 attempts=4
   
   # Look for abandoned transactions
   "Giving up on transaction after max attempts" source=Apollo destination=Directory sequence=123 attempts=10
   ```

2. **Transaction Tracker Statistics**:
   ```
   "Transaction tracker statistics" total=50 successful=30 failed=10 conflictingRoutes=5 avgAttempts=2.5 maxAttempts=10
   ```

## Summary

- **Per-Submission Limit**: Maximum 2 attempts per healing cycle (primary and backup nodes)
- **Overall Limit**: Maximum 10 healing cycles (tracked by `AttemptCount`)
- **Current Implementation**: Increments `AttemptCount` for each submission attempt
- **Design Intent**: `AttemptCount` should track healing cycles, not individual submission attempts
- **Recommendation**: Either adjust `maxSubmissionAttempts` or modify how `AttemptCount` is incremented
