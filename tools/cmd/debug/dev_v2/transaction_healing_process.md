# Transaction Healing Process

## Overview

The Accumulate Network implements a continuous healing process that runs alongside the blockchain to ensure data consistency across all partitions. This document clarifies the design, implementation, and expected behavior of this system.

### Purpose and Importance

The primary purpose of the healing process is to keep transactions flowing through the blockchain. Interactions between networks must continue via anchors and synthetic transactions to maintain the flow of tokens, data, and signatures. Since all networks together form the actual blockchain, any halt in communication isolates accounts and authorities, potentially fragmenting the network.

### Relationship to Core Blockchain

The healing process runs outside of the blockchain as an independent process. While future versions may integrate healing more intelligently into the actual peers, the current implementation operates as a separate system that monitors and repairs cross-partition communication.

### Failure Consequences

Failure of the healing process can lead to the blockchain stalling. It is critical to keep transactions between partitions flowing to maintain blockchain operations. Without proper healing, gaps in transaction sequences would accumulate, eventually preventing proper validation and consensus across the network.

### Operational Context

Healing operates continuously, constantly monitoring for gaps and addressing them as they're detected. This approach serves as a necessary stopgap measure until healing methods can be integrated directly into the nodes themselves.

### Scope

The healing process covers all partitions within the Accumulate Network. The Directory Network, which is run by all nodes, has the least communication issues but is also the most critical component. Even minor disruptions in Directory Network communication can have significant impacts on the overall system.

### Historical Context

Transaction healing has been a persistent challenge in the Accumulate Network. This implementation represents the latest approach to addressing these challenges, with a focus on reliability, persistence, and network-friendly operation.

### Error Handling Philosophy

The core philosophy of the healing process is that every transaction must eventually be healed - there are no permanent failures. If the automated healing process cannot resolve issues, developers must take direct actions to fix the network. This approach ensures the network's integrity is maintained even in challenging conditions.

### Monitoring and Reporting

The healing process generates comprehensive reports and metrics that contribute to overall network monitoring, providing visibility into cross-partition communication health and identifying potential systemic issues before they impact network operations.

#### Reporting Categories

1. **Transaction Healing Metrics**:
   - Gaps detected per partition pair
   - Healing attempts (successful and failed)
   - Average healing time per transaction
   - Number of transactions requiring multiple attempts
   - Number of transactions requiring synthetic alternatives

2. **Node Performance Metrics**:
   - Success/failure rates per node
   - Response times per node
   - Node ranking changes over time
   - Exploration vs. exploitation statistics

3. **Network Health Indicators**:
   - Partition connectivity matrix
   - Cross-partition communication latency
   - Recurring problem areas or patterns
   - URL construction and resolution statistics

4. **Transaction Reuse Approach**:
   - Transaction reuse statistics for healing gaps
   - Submission attempt counts
   - Routing error resolution metrics
   - Memory usage and optimization metrics
   
   > **Note**: The system does not implement a general query caching system. The only "caching" mechanism is the transaction tracking system that maintains healing transactions for resubmission on routing errors or across healing cycles.

#### Report Generation

Reports are generated at multiple intervals to provide both real-time and historical perspectives:

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        REPORTING SCHEDULE                                │
└─────────────────────────────────────────────────────────────────────────┘
   │
   ├── Real-time logs: Continuous detailed logging of all healing activities
   │
   ├── Hourly summaries: Aggregated statistics for the past hour
   │
   ├── Daily reports: Comprehensive analysis of healing patterns
   │
   └── Weekly trend analysis: Long-term patterns and system recommendations
```

#### Integration with Monitoring Systems

The healing process exposes metrics through multiple channels to integrate with existing monitoring infrastructure:

1. **Structured Logs**: All healing activities are logged with structured metadata for easy parsing and analysis

2. **Metrics Endpoints**: Key metrics are exposed via HTTP endpoints for scraping by monitoring systems

3. **Alert Triggers**: Configurable thresholds for automatic alerting when healing patterns indicate potential issues

4. **Visualization Data**: Pre-formatted data for dashboard visualization of network health

#### Actionable Insights

The reporting system is designed to provide actionable insights rather than just raw data:

1. **Problem Identification**: Automatically identify recurring patterns that indicate systemic issues

2. **Root Cause Analysis**: Correlate healing failures with network conditions to suggest potential root causes

3. **Performance Optimization**: Recommend configuration changes based on observed healing patterns

4. **Capacity Planning**: Project future healing needs based on network growth and transaction patterns

These comprehensive reporting capabilities ensure that the healing process not only maintains network consistency but also contributes valuable diagnostic information to the overall health monitoring of the Accumulate Network.

## Implementation Action Plan

Based on a comparison between this design document and the current implementation, the following action plan has been developed to address gaps and ensure alignment with the design principles.

### 1. Standardize URL Construction

- **Current Issue**: Inconsistent URL construction between files (raw partition URLs vs. anchor pool URLs)
- **Solution**: 
  - Consistently use raw partition URLs as specified in the design (`acc://bvn-Apollo.acme`)
  - Implement URL normalization to handle legacy formats
  - Update all chain queries to use the standardized URL format

### 2. Consolidate Transaction Tracking

- **Current Issue**: Incomplete transaction tracker with undefined methods and inconsistent usage
- **Solution**:
  - Implement a complete `TransactionTracker` with all required methods
  - Ensure proper lifecycle management of transactions
  - Add missing methods like `GetStats`, `FindTransactionForGap`, and `HandlePendingConflictingRoutes`
  - Standardize transaction status management

### 3. Implement Proper Node Selection

- **Current Issue**: Multiple competing node management approaches with redundant code
- **Solution**:
  - Consolidate on the `StableNodeManager` approach
  - Implement the time-weighted ranking system as described in the design
  - Add proper exploration vs. exploitation balance
  - Ensure node performance metrics are properly tracked

### 4. Fix Control Flow

- **Current Issue**: Broken control flow with undefined variables and unreachable code
- **Solution**:
  - Ensure one gap is processed per partition pair per cycle
  - Properly initialize and update all status variables
  - Fix the main healing loop to follow the design flow
  - Implement proper cycle management with appropriate pauses

### 5. Implement Comprehensive Error Handling

- **Current Issue**: Inconsistent error handling without proper recovery strategies
- **Solution**:
  - Categorize errors and implement appropriate recovery strategies
  - Ensure "no permanent failures" philosophy is maintained
  - Add proper logging of errors with context
  - Implement routing conflict detection and resolution

### 6. Consolidate Reporting

- **Current Issue**: Fragmented reporting with undefined report variables
- **Solution**:
  - Implement a consistent reporting structure
  - Ensure all metrics are properly tracked and displayed
  - Add support for the reporting schedule described in the design
  - Implement the visualization capabilities

### 7. Clean Up Code Structure

- **Current Issue**: Duplicate code across multiple files with slight variations
- **Solution**:
  - Reduce duplication by consolidating common functionality
  - Ensure consistent naming and function signatures
  - Remove redundant implementations of the same functionality
  - Organize code into logical modules

### 8. Address Syntax and Compilation Errors

- **Current Issue**: Multiple syntax errors and undefined imports
- **Solution**:
  - Fix all syntax errors and ensure code compiles cleanly
  - Resolve redeclaration issues across files
  - Add missing imports
  - Remove debug code with syntax errors (e.g., improper panic statements)

### Implementation Phases

1. **Phase 1: Fix Critical Issues**
   - Address syntax errors and compilation issues
   - Fix undefined variables and methods
   - Ensure basic functionality works

2. **Phase 2: Align with Design**
   - Standardize URL construction
   - Implement proper transaction tracking
   - Fix control flow

3. **Phase 3: Enhance Functionality**
   - Implement node selection strategy
   - Add comprehensive error handling
   - Consolidate reporting

4. **Phase 4: Optimize and Clean Up**
   - Reduce code duplication
   - Optimize performance
   - Add comprehensive testing

This action plan provides a structured approach to refactoring the transaction healing code to align with the design document while addressing the current implementation issues.

## What Needs to Be Healed

The healing process focuses on two specific types of chains that must be synchronized between partitions:

### 1. Anchor Sequence Chains

- **Purpose**: Track the sequence of anchor transactions between partitions
- **URL Format**: `acc://{partition-id}.acme/anchors/{target-partition}`
- **Example**: `acc://bvn-apollo.acme/anchors/bvn-europa`
- **Healing Need**: Ensure all anchor transactions are properly delivered between partitions

### 2. Synthetic Sequence Chains

- **Purpose**: Track the sequence of synthetic transactions between partitions
- **URL Format**: `acc://{partition-id}.acme/synthetic/synthetic-sequence-{0,1,index}`
- **Example**: `acc://bvn-apollo.acme/synthetic/synthetic-sequence-0`
- **Healing Need**: Ensure all synthetic transactions are properly delivered between partitions

### What Is NOT Healed

- **Root Chains**: These are partition-specific (`acc://{partition-id}.acme/ledger/root`) and are NOT synced between networks
- **Internal Partition State**: Only cross-partition transaction sequences need healing

### URL Construction Considerations

There are two methods for constructing URLs in the healing process:

1. **Raw Partition URLs** (used in sequence.go): `acc://bvn-Apollo.acme`
2. **Anchor Pool URLs** (used in heal_anchor.go): `acc://dn.acme/anchors/Apollo`

In Version 2, the raw partition URLs approach is preferred for consistency.

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
│ INITIALIZE:                                                              │
│   ├── Load network topology and partition information                    │
│   ├── Initialize transaction tracker                                     │
│   └── Set up node manager with time-weighted ranking system              │
└─────────────────────────────────────────────────────────────────────────┘
   │
   ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ FOR EACH PARTITION PAIR:                                                 │
│   ┌─────────────────────────────────────────────────────────────────────┐
│   │ 1. CHECK FOR ANCHOR GAPS                                            │
│   │    ├── Query source and destination ledgers                         │
│   │    ├── If gap found:                                                │
│   │    │   ├── Get the first anchor that needs healing                  │
│   │    │   ├── Check transaction tracker for existing transaction        │
│   │    │   │   ├── NO EXISTING TX: Create new anchor transaction        │
│   │    │   │   │   ├── Create envelope with proper signatures           │
│   │    │   │   │   ├── Add to transaction tracker                       │
│   │    │   │   │   └── Submit via best node (selected by node manager)  │
│   │    │   │   │                                                         │
│   │    │   │   └── EXISTING TX: Check submission count                  │
│   │    │   │       ├── < 10: Resubmit transaction                       │
│   │    │   │       │   ├── Increment submission count                   │
│   │    │   │       │   ├── Update last attempt time                     │
│   │    │   │       │   └── Submit via best node (selected by node manager) │
│   │    │   │       │                                                     │
│   │    │   │       └── ≥ 10: Create new synthetic transaction           │
│   │    │   │           ├── Mark old transaction as failed               │
│   │    │   │           ├── Create new synthetic envelope                │
│   │    │   │           ├── Add to transaction tracker                   │
│   │    │   │           └── Submit via best node                         │
│   │    │   │                                                             │
│   │    │   └── Process only one gap per partition pair per cycle        │
│   │    └── If no gap, proceed to synthetic check                        │
│   │                                                                     │
│   │ 2. CHECK FOR SYNTHETIC TRANSACTION GAPS                             │
│   │    ├── Query source and destination ledgers                         │
│   │    ├── If gap found:                                                │
│   │    │   ├── Get the first synthetic tx that needs healing            │
│   │    │   ├── Check transaction tracker for existing transaction        │
│   │    │   │   ├── NO EXISTING TX: Create new synthetic transaction     │
│   │    │   │   │   ├── Create envelope with proper data                 │
│   │    │   │   │   ├── Add to transaction tracker                       │
│   │    │   │   │   └── Submit via best node (selected by node manager)  │
│   │    │   │   │                                                         │
│   │    │   │   └── EXISTING TX: Check submission count                  │
│   │    │   │       ├── < 10: Resubmit transaction                       │
│   │    │   │       │   ├── Increment submission count                   │
│   │    │   │       │   ├── Update last attempt time                     │
│   │    │   │       │   └── Submit via best node (selected by node manager) │
│   │    │   │       │                                                     │
│   │    │   │       └── ≥ 10: Create alternative synthetic transaction   │
│   │    │   │           ├── Mark old transaction as failed               │
│   │    │   │           ├── Create new synthetic envelope                │
│   │    │   │           ├── Add to transaction tracker                   │
│   │    │   │           └── Submit via best node                         │
│   │    │   │                                                             │
│   │    │   └── Process only one gap per partition pair per cycle        │
│   │    └── If no gap, move to next partition pair                       │
│   └─────────────────────────────────────────────────────────────────────┘
└─────────────────────────────────────────────────────────────────────────┘
   │
   ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ SUBMISSION HANDLING:                                                     │
│   ├── For each submitted transaction:                                    │
│   │   ├── If successful:                                                 │
│   │   │   ├── Mark transaction as successful in tracker                  │
│   │   │   ├── Report success to node manager                            │
│   │   │   └── Log success with transaction details                       │
│   │   │                                                                  │
│   │   └── If failed:                                                     │
│   │       ├── Keep transaction in pending state                          │
│   │       ├── Report failure to node manager                            │
│   │       └── Log failure with error details                             │
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

### Key Design Principles

1. **Persistence**: Every entry must be healed - there are no permanent failures
   - After 10 failed attempts, the system creates an alternative approach rather than giving up
   - This ensures that even difficult-to-heal gaps will eventually be addressed

2. **Network Respect**: Controlled submission rate to avoid overwhelming the network
   - Only one submission attempt per transaction per healing cycle
   - Only one gap processed per partition pair per cycle
   - Appropriate pauses between healing cycles based on activity level

3. **Intelligent Node Selection**: Time-weighted ranking system for optimal node selection
   - Nodes are selected based on their historical performance with recency bias
   - Exploration vs. exploitation balance ensures discovery of new good nodes
   - Failed nodes are penalized but can recover their ranking over time

4. **Comprehensive Tracking**: All transactions are tracked throughout their lifecycle
   - Transaction state is maintained across healing cycles
   - Submission counts are tracked to determine when to try alternative approaches
   - Success and failure metrics are collected for analysis and monitoring

5. **Prioritization**: Process the lowest sequence numbers first
   - Ensures logical healing order (earlier gaps are filled before later ones)
   - Prevents cascading gaps that could complicate the healing process

## URL Standardization

Consistent URL construction is critical for the healing process. The system standardizes on the raw partition URL format used in sequence.go to ensure reliable transaction routing and chain lookups.

### URL Construction Patterns

#### Standardized Approach

All URL construction in the healing process now follows the pattern established in sequence.go, using raw partition URLs:

```go
// Raw partition URL format (PREFERRED)
srcUrl := protocol.PartitionUrl(src.ID)  // e.g., acc://bvn-Apollo.acme
```

This approach is used consistently for all chain types:

| Chain Type | Standardized URL Pattern | Example |
|------------|--------------------------|--------|
| Root Chain | `acc://{partition-id}.acme/ledger/root` | `acc://bvn-apollo.acme/ledger/root` |
| Synthetic Sequence | `acc://{partition-id}.acme/synthetic/synthetic-sequence-{0,1,index}` | `acc://bvn-apollo.acme/synthetic/synthetic-sequence-0` |
| Anchor Chain | `acc://{partition-id}.acme/anchors/{target-partition}` | `acc://bvn-apollo.acme/anchors/bvn-europa` |

#### Deprecated Approach

Previously, heal_anchor.go used a different URL construction pattern that appended the partition ID to the anchor pool URL:

```go
// Anchor pool URL format (DEPRECATED)
srcUrl := protocol.DnUrl().JoinPath(protocol.AnchorPool).JoinPath(src.ID)  // e.g., acc://dn.acme/anchors/Apollo
```

This inconsistency caused "element does not exist" errors when code using different URL formats attempted to access the same chains.

### URL Normalization

To ensure consistent URL handling, the healing process implements URL normalization:

```go
// Normalize URLs to ensure consistent routing
func normalizeUrl(u *url.URL) *url.URL {
    if u == nil {
        return nil
    }

    // If this is an anchor pool URL with a partition path, convert to partition URL
    if strings.EqualFold(u.Authority, protocol.Directory) && strings.HasPrefix(u.Path, "/anchors/") {
        parts := strings.Split(u.Path, "/")
        if len(parts) >= 3 {
            partitionID := parts[2]
            return protocol.PartitionUrl(partitionID)
        }
    }

    return u
}
```

This function is applied to all URLs in messages before submission:

```go
// Normalize URLs in all messages before submission
for _, msg := range m {
    normalizeUrlsInMessage(msg)
}
```

### Implementation Details

The standardization has been implemented in several key components:

1. **heal_anchor.go**: Updated to use raw partition URLs consistently
2. **Transaction Submission**: All URLs in messages are normalized before submission
3. **Caching System**: Cache keys are constructed using normalized URLs to ensure cache hits
4. **Error Handling**: Improved error messages to identify URL-related issues

### Common URL-Related Issues

1. **"Element does not exist" errors**
   - Often caused by URL format mismatches
   - Now prevented by consistent URL normalization

2. **Partition ID case sensitivity**
   - URLs should be case-insensitive, but some code may be case-sensitive
   - Standardization ensures consistent casing

3. **Chain lookup failures**
   - Previously occurred when code looked for chains at different URL paths
   - Resolved by standardizing on a single URL construction pattern

### Troubleshooting URL Issues

If URL-related issues occur despite standardization:

1. **Verify URL construction**: Ensure all code is using `protocol.PartitionUrl(partitionID)`
2. **Check normalization**: Confirm that URL normalization is being applied to all messages
3. **Inspect cache keys**: Verify that cache keys are using normalized URLs
4. **Review error logs**: Look for specific URL patterns in error messages

## Transaction Tracking and Reuse

### Transaction Lifecycle

Each transaction in the healing process goes through the following lifecycle:

1. **Creation**: A new transaction is created when a gap is detected and no existing transaction is found for that gap

2. **Submission**: The transaction is submitted once per healing cycle

3. **Tracking**: The submission count is incremented with each submission

4. **Resolution**:
   - If the transaction is successfully submitted, it is marked as successful
   - If the transaction fails 10 times, a new synthetic transaction is created as an alternative approach

### Transaction Wrapper Implementation

The transaction wrapper is a key design element that manages the lifecycle of healing transactions without requiring persistent state:

```go
// HealingTransaction wraps a transaction with metadata for the healing process
type HealingTransaction struct {
    // Core transaction information
    SourcePartition string       // Source partition ID
    DestPartition   string       // Destination partition ID
    SequenceNumber  uint64       // Sequence number being healed
    Transaction     *protocol.Transaction // The actual transaction
    Envelope        *messaging.Envelope   // The signed envelope for submission
    
    // Tracking information
    SubmissionCount   int       // Number of submission attempts
    FirstAttemptTime  time.Time // Time of first submission attempt
    LastAttemptTime   time.Time // Time of last submission attempt
    CreationTime      time.Time // Time when the transaction was created
    Status            TxStatus  // Current status (pending, submitted, etc.)
    
    // Routing information
    DestinationUrl    *url.URL  // Normalized destination URL
    SelectedNode      string    // Node ID selected for submission
    
    // Status tracking
    Status          HealingTxStatus
    
    // Additional metadata
    TxID        string
    IsAnchor    bool   // Whether this is an anchor transaction
    IsSynthetic bool   // Whether this is a synthetic transaction
}
```

### Transaction Lifecycle

The transaction wrapper manages the complete lifecycle of a healing transaction without requiring persistent state:

1. **Creation**:
   ```go
   // When a gap is detected, create a new transaction wrapper
   tx := &HealingTransaction{
       SourcePartition: src.ID,
       DestPartition:   dst.ID,
       SequenceNumber:  gapNum,
       Transaction:     createTransaction(src, dst, gapNum),
       CreationTime:    time.Now(),
       Status:          TxStatusCreated,
       SubmissionCount: 0,
   }
   ```

2. **Tracking**:
   ```go
   // Add to transaction tracker for the duration of healing
   tracker.AddTransaction(tx)
   ```

3. **Submission**:
   ```go
   // Before submission, update tracking information
   tx.SubmissionCount++
   tx.LastAttemptTime = time.Now()
   tx.Status = TxStatusPending
   tx.SelectedNode = selectedNodeID
   
   // Submit the transaction
   submitTransaction(ctx, tx)
   ```

4. **Status Update**:
   ```go
   // After submission, update status based on result
   if success {
       tx.Status = TxStatusSubmitted
   } else {
       tx.Status = TxStatusFailed
   }
   ```

5. **Reuse or Replacement**:
   ```go
   // When encountering the same gap again
   existingTx := tracker.FindTransactionForGap(src.ID, dst.ID, gapNum)
   if existingTx != nil {
       if existingTx.SubmissionCount < MaxSubmissionAttempts {
           // Reuse the existing transaction
           existingTx.SubmissionCount++
           existingTx.LastAttemptTime = time.Now()
           submitTransaction(ctx, existingTx)
       } else {
           // Create a synthetic alternative after too many attempts
           createSyntheticAlternative(ctx, src, dst, gapNum)
       }
   }
   ```

6. **Cleanup**:
   ```go
   // Once a transaction is confirmed or replaced with a synthetic alternative
   tracker.RemoveTransaction(tx)
   ```

### Transaction Reuse Benefits

This wrapper-based approach provides several advantages:

- **No Persistent State Required**: All tracking information is contained in memory during the healing process
- **Comprehensive Tracking**: Maintains submission count, timestamps, and status without database overhead
- **Clean Lifecycle Management**: Clear transitions between states from creation to completion
- **Efficient Reuse**: Enables intelligent reuse decisions based on submission history
- **Memory Efficiency**: The one-transaction-per-partition-pair design limits memory usage

### Transaction Reuse Mechanism

The transaction reuse mechanism is a key optimization in the healing process. Instead of creating a new transaction for each detected gap in every cycle, the system tracks and reuses existing transactions until they either succeed or reach the maximum retry threshold.

```
┌─────────────────────────────────────────────────────────────────────────┐
│                     TRANSACTION REUSE FLOW                               │
└─────────────────────────────────────────────────────────────────────────┘
   │
   ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ GAP DETECTION                                                            │
│   ├── Query source and destination chains                                │
│   └── Identify sequence gaps                                             │
└─────────────────────────────────────────────────────────────────────────┘
   │
   ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ TRANSACTION LOOKUP                                                       │
│   │                                                                     │
│   ├── Check if transaction exists for this gap:                          │
│   │   txTracker.FindTransactionForGap(srcID, dstID, gapNumber)          │
│   │                                                                     │
│   ├── If EXISTS:                                                         │
│   │   │                                                                 │
│   │   ├── Check submission count                                         │
│   │   │                                                                 │
│   │   ├── If count < 10:                                                 │
│   │   │   ├── Increment submission count                                 │
│   │   │   ├── Update last attempt time                                   │
│   │   │   └── REUSE existing transaction                                 │
│   │   │                                                                 │
│   │   └── If count >= 10:                                                │
│   │       ├── Mark existing transaction as failed                        │
│   │       └── Create new synthetic alternative                           │
│   │                                                                     │
│   └── If NOT EXISTS:                                                     │
│       └── Create new transaction                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

#### Transaction Lookup and Identification

The transaction tracker maintains an efficient index of all transactions by their source partition, destination partition, and sequence number:

```go
// FindTransactionForGap looks up a transaction by its source, destination, and sequence number
func (t *TransactionTracker) FindTransactionForGap(srcPartition, dstPartition string, seqNum uint64) *HealingTransaction {
    // Create a unique key for this gap
    key := fmt.Sprintf("%s:%s:%d", srcPartition, dstPartition, seqNum)
    
    t.mutex.RLock()
    defer t.mutex.RUnlock()
    
    // Look up the transaction in our index
    return t.txMap[key]
}
```

#### Transaction Reuse Decision Logic

The decision to reuse a transaction is based on its submission count and status:

```go
// Process a gap by either reusing an existing transaction or creating a new one
func (h *healer) processGap(ctx context.Context, src, dst *protocol.PartitionInfo, gapNum uint64) (bool, error) {
    // Look for an existing transaction for this gap
    existingTx := h.txTracker.FindTransactionForGap(src.ID, dst.ID, gapNum)
    
    // If no existing transaction, create a new one
    if existingTx == nil {
        slog.InfoContext(ctx, "No existing transaction found, creating new one",
            "source", src.ID, "destination", dst.ID, "sequence", gapNum)
        
        return h.createNewTransaction(ctx, src, dst, gapNum)
    }
    
    // If existing transaction with fewer than 10 submissions, reuse it
    if existingTx.SubmissionCount < 10 {
        slog.InfoContext(ctx, "Reusing existing transaction",
            "source", src.ID, "destination", dst.ID, 
            "sequence", gapNum, "submissions", existingTx.SubmissionCount)
        
        // Increment submission count
        existingTx.SubmissionCount++
        existingTx.LastAttemptTime = time.Now()
        
        // Submit the transaction
        h.submit <- existingTx
        
        // Update transaction status
        h.txTracker.UpdateTransactionStatus(existingTx, TxStatusPending)
        return true, nil
    }
    
    // If existing transaction with 10 or more submissions, create synthetic
    slog.InfoContext(ctx, "Transaction reached maximum submission attempts, creating synthetic alternative",
        "source", src.ID, "destination", dst.ID, 
        "sequence", gapNum, "submissions", existingTx.SubmissionCount)
    
    // Mark the old transaction as failed
    h.txTracker.UpdateTransactionStatus(existingTx, TxStatusFailed)
    
    // Create a new synthetic transaction
    return h.createSyntheticAlternative(ctx, src, dst, gapNum)
}
```

#### Benefits of Transaction Reuse

1. **Network Efficiency**: Reduces the number of transaction objects created, minimizing memory usage

2. **Consistent Tracking**: Maintains a clear history of all submission attempts for a particular gap

3. **Optimized Retry Logic**: Enables sophisticated retry strategies based on submission count

4. **Adaptive Healing**: Allows the system to adapt its approach based on previous attempt outcomes

5. **Failure Analysis**: Provides comprehensive data for analyzing persistent failures

#### Transaction State Management

The transaction tracker maintains the state of all transactions across healing cycles:

```go
// Transaction states
const (
    TxStatusNew     HealingTxStatus = "new"      // Newly created transaction
    TxStatusPending HealingTxStatus = "pending"  // Transaction has been submitted but not confirmed
    TxStatusSuccess HealingTxStatus = "success"  // Transaction was successfully processed
    TxStatusFailed  HealingTxStatus = "failed"   // Transaction failed after maximum attempts
)

// UpdateTransactionStatus updates the status of a transaction and performs any necessary state transitions
func (t *TransactionTracker) UpdateTransactionStatus(tx *HealingTransaction, newStatus HealingTxStatus) {
    t.mutex.Lock()
    defer t.mutex.Unlock()
    
    oldStatus := tx.Status
    tx.Status = newStatus
    
    // Update metrics based on state transition
    switch {
    case oldStatus != TxStatusSuccess && newStatus == TxStatusSuccess:
        // Transaction succeeded
        t.stats.SuccessCount++
        t.stats.AvgAttempts = (t.stats.AvgAttempts*float64(t.stats.SuccessCount-1) + float64(tx.SubmissionCount)) / float64(t.stats.SuccessCount)
        
    case oldStatus != TxStatusFailed && newStatus == TxStatusFailed:
        // Transaction failed after maximum attempts
        t.stats.FailureCount++
        
        // Record if this is a synthetic transaction failure (indicates a more serious issue)
        if tx.IsSynthetic {
            t.stats.SyntheticFailureCount++
        }
    }
}
```

### Synthetic Transaction Creation

After 10 failed attempts with the original transaction, the system creates a synthetic alternative:

```go
// Create a synthetic transaction as an alternative approach
func (h *healer) createSyntheticAlternative(ctx context.Context, src, dst *protocol.PartitionInfo, seqNum uint64) (bool, error) {
    // Create a synthetic envelope
    envelope, err := h.createSyntheticEnvelope(src, dst, seqNum)
    if err != nil {
        slog.ErrorContext(ctx, "Failed to create synthetic envelope", 
            "source", src.ID, "destination", dst.ID, "sequence", seqNum, 
            "error", err)
        return false, err
    }
    
    // Create a new healing transaction for the synthetic tx
    tx := &HealingTransaction{
        Envelope:        envelope,
        SourcePartition: src.ID,
        DestPartition:   dst.ID,
        SequenceNumber:  seqNum,
        FirstAttemptTime: time.Now(),
        LastAttemptTime:  time.Now(),
        SubmissionCount:  1,
        Status:          TxStatusNew,
        IsSynthetic:     true,
    }
    
    // Add to transaction tracker
    h.txTracker.AddTransaction(tx)
    
    // Submit the new synthetic transaction
    h.submit <- tx
    return true, nil
}
```

This synthetic transaction approach ensures that even the most difficult gaps can eventually be healed, maintaining the "no permanent failures" philosophy of the healing process.

## Peer Grading and Node Selection

The healing process uses a sophisticated peer grading system to select the best nodes for transaction submissions, ensuring optimal performance and reliability.

### Time-Weighted Ranking System

The node selection system uses a time-weighted ranking approach that prioritizes recent performance while still accounting for historical data:

```go
// NodeScore represents a time-weighted score for a node
type NodeScore struct {
    Score         float64   // Combined weighted score (higher is better)
    LastUpdated   time.Time // When the score was last updated
    AttemptCount  int       // Total number of attempts
    ResponseTimes []time.Duration // Recent response times
}
```

### Key Advantages of Time-Weighted Ranking

1. **Recency Bias**: Recent successes/failures have higher weight than older ones
2. **Natural Decay**: Scores naturally decay over time if a node isn't used
3. **Recovery Opportunity**: Nodes can recover from past failures if they start performing well
4. **Adaptability**: The system quickly adapts to changing network conditions

### Node Scoring Process

```
┌─────────────────────────────────────────────────────────────────────────┐
│                     TIME-WEIGHTED SCORING PROCESS                        │
└─────────────────────────────────────────────────────────────────────────┘
   │
   ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ AFTER EACH TRANSACTION SUBMISSION:                                       │
│   ├── Calculate time decay factor:                                       │
│   │   └── decay = exp(-timeSinceLastUpdate/halfLife)                    │
│   │                                                                     │
│   ├── Apply decay to existing score:                                     │
│   │   └── score = score * decay                                          │
│   │                                                                     │
│   ├── Add new result with recency weight:                                │
│   │   ├── If success: score += 1.0 * recencyWeight                      │
│   │   └── If failure: score -= 1.0 * recencyWeight                      │
│   │                                                                     │
│   ├── Update response time metrics (if successful)                       │
│   │                                                                     │
│   └── Cap score to prevent extreme values                                │
└─────────────────────────────────────────────────────────────────────────┘
   │
   ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ EXPLORATION VS. EXPLOITATION BALANCE:                                    │
│   ├── With probability p: Select random untested/low-attempt node        │
│   └── With probability (1-p): Select highest scoring node                │
└─────────────────────────────────────────────────────────────────────────┘
```

### Node Selection Strategy

1. **Exploration vs. Exploitation Balance**:
   - **Exploitation**: Most of the time, select the highest-scoring node
   - **Exploration**: Occasionally try untested or low-attempt nodes to discover potential good performers
   - Exploration probability decreases as more good nodes are discovered

   ```go
   // Configuration parameters for tuning exploration vs. exploitation
   type ExplorationConfig struct {
       // Base exploration probability (0.0-1.0)
       BaseExplorationRate float64
       
       // Minimum exploration probability, never goes below this
       MinExplorationRate float64
       
       // How quickly exploration rate decreases as good nodes are found
       // Higher values = faster decrease
       ExplorationDecayFactor float64
       
       // Number of good nodes at which exploration rate reaches minimum
       GoodNodeThreshold int
   }
   ```

   The actual exploration probability is calculated dynamically:
   
   ```go
   // Calculate exploration probability based on number of known good nodes
   func calculateExplorationRate(config ExplorationConfig, goodNodeCount int) float64 {
       if goodNodeCount >= config.GoodNodeThreshold {
           return config.MinExplorationRate
       }
       
       // Linear decay from base rate to min rate
       decayRatio := float64(goodNodeCount) / float64(config.GoodNodeThreshold)
       return config.BaseExplorationRate - 
              (config.BaseExplorationRate - config.MinExplorationRate) * decayRatio
   }
   ```
   
   Default values that work well in practice:
   - `BaseExplorationRate`: 0.2 (20% exploration at start)
   - `MinExplorationRate`: 0.05 (5% minimum exploration)
   - `GoodNodeThreshold`: 5 (reaches minimum after finding 5 good nodes)
   - `ExplorationDecayFactor`: 1.0 (linear decay)

2. **Partition-Specific Rankings**:
   - Each partition maintains its own ranked list of nodes
   - Rankings are continuously updated based on time-weighted scores

3. **Adaptive Testing**:
   - All nodes are periodically tested to update their scores
   - Test frequency varies based on node score and attempt count:
     - High-scoring nodes: Tested less frequently to avoid unnecessary load
     - Low-scoring nodes: Tested occasionally to check for improvement
     - Untested nodes: Given priority for testing to establish initial scores

4. **Recovery Mechanism**:
   - Even nodes with past failures can recover their ranking if they start performing well
   - The time decay ensures that old failures have diminishing impact over time

This time-weighted ranking system ensures that the healing process consistently selects the most reliable nodes for transaction submissions while still discovering and adapting to changes in the network.

## Implementation Details

### Anchor Transaction Creation

When a gap is detected in the anchor sequence between two partitions, a new anchor transaction is created:

```go
// Check if we have an existing transaction for this anchor gap
existingTx := h.txTracker.FindTransactionForGap(src.ID, dst.ID, gap.Number)

// If no existing transaction, create a new anchor transaction
if existingTx == nil {
    // Create the anchor envelope
    envelope, err := h.createAnchorEnvelope(src, dst, gap.Number)
    if err != nil {
        slog.ErrorContext(ctx, "Failed to create anchor envelope", 
            "source", src.ID, "destination", dst.ID, "sequence", gap.Number, 
            "error", err)
        return false, err
    }
    
    // Create a new healing transaction for the anchor
    tx := CreateHealingTransaction(envelope, src.ID, dst.ID, gap.Number, true) // isAnchor=true
    h.txTracker.AddTransaction(tx)
    
    // Submit the new transaction
    h.submit <- tx
    return true, nil
}
```

### Synthetic Transaction Creation

When a gap is detected in the synthetic transaction sequence between two partitions, a new synthetic transaction is created:

```go
// Check if we have an existing transaction for this synthetic gap
existingTx := h.txTracker.FindTransactionForGap(src.ID, dst.ID, gap.Number)

// If no existing transaction, create a new synthetic transaction
if existingTx == nil {
    // Create the synthetic envelope
    envelope, err := h.createSyntheticEnvelope(src, dst, gap.Number)
    if err != nil {
        slog.ErrorContext(ctx, "Failed to create synthetic envelope", 
            "source", src.ID, "destination", dst.ID, "sequence", gap.Number, 
            "error", err)
        return false, err
    }
    
    // Create a new healing transaction for the synthetic tx
    tx := CreateHealingTransaction(envelope, src.ID, dst.ID, gap.Number, false) // isAnchor=false
    h.txTracker.AddTransaction(tx)
    
    // Submit the new transaction
    h.submit <- tx
    return true, nil
}
```

### Transaction Submission

Each transaction is submitted exactly once per healing cycle:

```go
// If existing transaction with fewer than 10 submissions, reuse it
if existingTx != nil && existingTx.SubmissionCount < 10 {
    // Increment submission count
    existingTx.SubmissionCount++
    existingTx.LastAttemptTime = time.Now()
    
    // Submit the transaction
    h.submit <- existingTx
    
    // Update transaction status
    h.txTracker.UpdateTransactionStatus(existingTx, TxStatusPending)
}
```

### Synthetic Transaction Creation

After 10 failed submissions, a new synthetic transaction is created:

```go
// If existing transaction with 10 or more submissions, create synthetic
if existingTx != nil && existingTx.SubmissionCount >= 10 {
    // Mark the old transaction as failed
    h.txTracker.UpdateTransactionStatus(existingTx, TxStatusFailed)
    
    // Create a new synthetic transaction
    tx := CreateSyntheticTransaction(src.ID, dst.ID, gap.Number)
    h.txTracker.AddTransaction(tx)
    
    // Submit the new synthetic transaction
    h.submit <- tx
}
```

### Node Selection

The SimpleNodeManager selects the best node for each submission:

```go
// Get the best node for this partition
nodeID, found := h.nodeManager.GetBestNode(tx.DestPartition)
if !found {
    return false, fmt.Errorf("no good nodes available for partition %s", tx.DestPartition)
}

// Create a client for this node
info := h.net.Peers[strings.ToLower(tx.DestPartition)][nodeID]

// Submit the transaction
subs, err := c.Submit(ctx, tx.Envelope, api.SubmitOptions{})

// Report success or failure to the node manager
if err != nil {
    h.nodeManager.ReportFailure(tx.DestPartition, nodeID)
    return false, err
} else {
    h.nodeManager.ReportSuccess(tx.DestPartition, nodeID)
    return true, nil
}
```

## Logging and Monitoring

The healing process includes comprehensive logging to track progress and identify issues:

```go
// Log transaction submission
slog.InfoContext(ctx, "Processing transaction submission",
    "source", tx.SourcePartition,
    "destination", tx.DestPartition,
    "sequence", tx.SequenceNumber,
    "submissions", tx.SubmissionCount)

// Log success or failure
if success {
    slog.InfoContext(ctx, "Transaction submission succeeded",
        "source", tx.SourcePartition,
        "destination", tx.DestPartition,
        "sequence", tx.SequenceNumber)
} else {
    slog.ErrorContext(ctx, "Transaction submission failed",
        "source", tx.SourcePartition,
        "destination", tx.DestPartition,
        "sequence", tx.SequenceNumber,
        "error", err)
}
```

## Error Recovery Mechanisms

The transaction healing process implements robust error recovery mechanisms to ensure that no transaction permanently fails and that the system can recover from various error conditions.

### Error Classification and Handling

```
┌─────────────────────────────────────────────────────────────────────────┐
│                      ERROR CLASSIFICATION SYSTEM                         │
└─────────────────────────────────────────────────────────────────────────┘
   │
   ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ 1. TRANSIENT ERRORS                                                      │
│    ├── Network timeouts                                                  │
│    ├── Temporary node unavailability                                     │
│    ├── Rate limiting                                                     │
│    └── Recovery: Automatic retry with exponential backoff                │
│                                                                         │
│ 2. NODE-SPECIFIC ERRORS                                                  │
│    ├── Malformed responses                                               │
│    ├── Consistent timeouts from specific nodes                           │
│    ├── Protocol version mismatches                                       │
│    └── Recovery: Node blacklisting and alternative node selection        │
│                                                                         │
│ 3. TRANSACTION-SPECIFIC ERRORS                                           │
│    ├── Invalid transaction format                                        │
│    ├── Missing dependencies                                              │
│    ├── Sequence number conflicts                                         │
│    └── Recovery: Transaction reconstruction and synthetic alternatives    │
│                                                                         │
│ 4. PARTITION-LEVEL ERRORS                                                │
│    ├── Entire partition unreachable                                      │
│    ├── Consensus failures                                                │
│    ├── Chain state inconsistencies                                       │
│    └── Recovery: Partition-wide retry strategy with delayed healing      │
└─────────────────────────────────────────────────────────────────────────┘
```

### Retry Strategies

#### 1. Exponential Backoff for Transient Errors

```go
// Calculate backoff duration based on attempt count
func calculateBackoff(attemptCount int) time.Duration {
    // Base backoff of 1 second
    baseBackoff := 1 * time.Second
    
    // Maximum backoff of 5 minutes
    maxBackoff := 5 * time.Minute
    
    // Calculate exponential backoff with jitter
    backoff := baseBackoff * time.Duration(math.Pow(2, float64(attemptCount-1)))
    jitter := time.Duration(rand.Float64() * float64(backoff) * 0.2) // 20% jitter
    backoff = backoff + jitter
    
    // Cap at maximum backoff
    if backoff > maxBackoff {
        backoff = maxBackoff
    }
    
    return backoff
}
```

#### 2. Alternative Approach Generation

After multiple failures of the same transaction, the system creates alternative approaches:

```go
// After 10 failed attempts, create a synthetic alternative
if tx.SubmissionCount >= 10 && !tx.IsSynthetic {
    // Create synthetic transaction as an alternative approach
    syntheticTx := CreateSyntheticTransaction(tx.SourcePartition, tx.DestPartition, tx.SequenceNumber)
    syntheticTx.IsSynthetic = true
    
    // Add to transaction tracker and submit
    h.txTracker.AddTransaction(syntheticTx)
    h.submit <- syntheticTx
    
    slog.InfoContext(ctx, "Created synthetic alternative after multiple failures",
        "source", tx.SourcePartition,
        "destination", tx.DestPartition,
        "sequence", tx.SequenceNumber,
        "attempts", tx.SubmissionCount)
}
```

### Error Recovery Flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         ERROR RECOVERY FLOW                              │
└─────────────────────────────────────────────────────────────────────────┘
   │
   ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ TRANSACTION SUBMISSION FAILS                                             │
└─────────────────────────────────────────────────────────────────────────┘
   │
   ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ 1. ERROR ANALYSIS                                                        │
│    ├── Classify error type                                               │
│    ├── Log detailed error information                                    │
│    └── Update node and transaction metrics                               │
└─────────────────────────────────────────────────────────────────────────┘
   │
   ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ 2. NODE MANAGEMENT                                                       │
│    ├── Report failure to node manager                                    │
│    ├── Update node score with time-weighted penalty                      │
│    └── Potentially remove node from good nodes list                      │
└─────────────────────────────────────────────────────────────────────────┘
   │
   ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ 3. TRANSACTION TRACKING                                                  │
│    ├── Increment submission count                                        │
│    ├── Update last attempt time                                          │
│    └── Set transaction status to failed                                  │
└─────────────────────────────────────────────────────────────────────────┘
   │
   ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ 4. RECOVERY DECISION                                                     │
│    ├── If submission count < 10: Schedule for retry in next cycle        │
│    ├── If submission count >= 10: Create synthetic alternative           │
│    └── If partition-wide issue: Implement partition-specific strategy    │
└─────────────────────────────────────────────────────────────────────────┘
```

### Handling Specific Error Types

#### URL Resolution Errors

The system specifically handles URL-related errors through normalization:

```go
// Before submitting, normalize all URLs to prevent resolution errors
func prepareTransaction(tx *HealingTransaction) {
    // Apply URL normalization to all URLs in the transaction
    for _, msg := range tx.Envelope.Messages {
        normalizeUrlsInMessage(msg)
    }
    
    // Ensure destination URL is also normalized
    if tx.DestinationUrl != nil {
        tx.DestinationUrl = normalizeUrl(tx.DestinationUrl)
    }
}
```

#### Chain State Inconsistencies

When chain state inconsistencies are detected:

```go
// If chain states are inconsistent, perform chain verification
if err != nil && errors.Is(err, errors.Conflict) {
    // Log the inconsistency
    slog.WarnContext(ctx, "Chain state inconsistency detected",
        "source", tx.SourcePartition,
        "destination", tx.DestPartition,
        "sequence", tx.SequenceNumber)
    
    // Perform chain verification to identify the correct state
    correctState, verifyErr := verifyChainState(ctx, h, tx.SourcePartition, tx.DestPartition)
    if verifyErr != nil {
        slog.ErrorContext(ctx, "Failed to verify chain state",
            "error", verifyErr)
        return false, verifyErr
    }
    
    // Adjust healing strategy based on correct state
    adjustHealingStrategy(tx, correctState)
}
```

### Partition Recovery Strategies

For partition-wide issues, the system implements specialized recovery strategies:

1. **Consensus Verification**: When a partition appears unreachable, verify with multiple nodes to confirm it's a partition-wide issue rather than a node-specific problem

2. **Delayed Healing**: For confirmed partition-wide issues, implement a delayed healing schedule with increasing intervals between attempts

3. **Alternative Routing**: Attempt to route transactions through intermediate partitions when direct routes are consistently failing

4. **Administrator Alerts**: Generate alerts for persistent partition-wide issues that may require manual intervention

### No Permanent Failures Philosophy

The core philosophy of the error recovery system is that there are no permanent failures. Every transaction must eventually be healed, even if it requires multiple approaches, extended timeframes, or alternative routing paths.

```go
// Even after synthetic alternatives fail, continue attempting with increasing delays
if tx.SubmissionCount >= 20 {
    // Log persistent failure
    slog.WarnContext(ctx, "Transaction persistently failing, continuing with extended delays",
        "source", tx.SourcePartition,
        "destination", tx.DestPartition,
        "sequence", tx.SequenceNumber,
        "attempts", tx.SubmissionCount)
    
    // Calculate extended delay based on attempt count
    // This ensures we keep trying but don't overload the system
    extendedDelay := calculateExtendedDelay(tx.SubmissionCount)
    
    // Schedule for retry after extended delay
    time.AfterFunc(extendedDelay, func() {
        h.submit <- tx
    })
}
```

## Configuration Guide

The transaction healing process can be configured through several parameters to optimize its behavior for different network conditions and requirements. This section documents the currently implemented configuration parameters, their purpose, and recommended settings.

### Core Configuration Parameters

```go
// HealerConfig defines the core configuration for the healing process
type HealerConfig struct {
    // Maximum number of submission attempts before creating a synthetic alternative
    MaxSubmissionAttempts int
    
    // Pause duration between healing cycles
    CyclePauseDuration time.Duration
    
    // Maximum number of transactions to process per cycle
    MaxTransactionsPerCycle int
    
    // Maximum number of transactions to process per partition pair per cycle
    MaxTransactionsPerPartitionPair int
    
    // Whether to enable verbose logging
    VerboseLogging bool
}
```

#### Default Values and Recommendations

| Parameter | Default | Min | Max | Description |
|-----------|---------|-----|-----|-------------|
| `MaxSubmissionAttempts` | 10 | 5 | 20 | Number of attempts before creating synthetic alternative |
| `CyclePauseDuration` | 5s | 1s | 30s | Time to wait between healing cycles |
| `MaxTransactionsPerCycle` | 100 | 10 | 500 | Maximum transactions to process in one cycle |
| `MaxTransactionsPerPartitionPair` | 1 | 1 | 5 | Maximum transactions per partition pair per cycle |
| `VerboseLogging` | false | - | - | Enable detailed logging for debugging |

### Node Manager Configuration

```go
// NodeManagerConfig defines the configuration for the node manager
type NodeManagerConfig struct {
    // Time-weighted scoring parameters
    ScoreHalfLife time.Duration
    RecencyWeight float64
    
    // Exploration vs. exploitation parameters
    Exploration ExplorationConfig
    
    // Node testing parameters
    TestInterval time.Duration
}

// ExplorationConfig defines parameters for balancing exploration vs. exploitation
type ExplorationConfig struct {
    // Base exploration probability (0.0-1.0)
    BaseExplorationRate float64
    
    // Minimum exploration probability, never goes below this
    MinExplorationRate float64
    
    // Number of good nodes at which exploration rate reaches minimum
    GoodNodeThreshold int
}
```

#### Node Manager Default Values and Recommendations

| Parameter | Default | Min | Max | Description |
|-----------|---------|-----|-----|-------------|
| `ScoreHalfLife` | 1h | 10m | 24h | Time for score to decay by half |
| `RecencyWeight` | 0.3 | 0.1 | 0.5 | Weight given to recent results vs. historical |
| `BaseExplorationRate` | 0.2 | 0.1 | 0.3 | Initial exploration rate (20%) |
| `MinExplorationRate` | 0.05 | 0.01 | 0.1 | Minimum exploration rate (5%) |
| `GoodNodeThreshold` | 5 | 3 | 10 | Number of good nodes to reach min exploration |
| `TestInterval` | 1m | 30s | 5m | How often to test nodes |

### Configuration Examples

#### Small Network (< 10 Nodes)

```go
HealerConfig{
    MaxSubmissionAttempts: 8,
    CyclePauseDuration: 3 * time.Second,
    MaxTransactionsPerCycle: 50,
    MaxTransactionsPerPartitionPair: 1,
    NodeManager: NodeManagerConfig{
        ScoreHalfLife: 30 * time.Minute,
        RecencyWeight: 0.4,
        Exploration: ExplorationConfig{
            BaseExplorationRate: 0.3,
            MinExplorationRate: 0.1,
            GoodNodeThreshold: 3,
        },
        TestInterval: 30 * time.Second,
    },
}
```

#### Large Network (50+ Nodes)

```go
HealerConfig{
    MaxSubmissionAttempts: 12,
    CyclePauseDuration: 8 * time.Second,
    MaxTransactionsPerCycle: 200,
    MaxTransactionsPerPartitionPair: 2,
    NodeManager: NodeManagerConfig{
        ScoreHalfLife: 2 * time.Hour,
        RecencyWeight: 0.2,
        Exploration: ExplorationConfig{
            BaseExplorationRate: 0.15,
            MinExplorationRate: 0.03,
            GoodNodeThreshold: 8,
        },
        TestInterval: 2 * time.Minute,
    },
}
```

### Command Line Flags

The healing process can be configured using command line flags:

```bash
# Core configuration
--max-submission-attempts=10     # Maximum submission attempts before creating synthetic alternative
--cycle-pause=5s                 # Pause duration between healing cycles
--max-tx-per-cycle=100           # Maximum transactions to process per cycle
--max-tx-per-pair=1              # Maximum transactions per partition pair per cycle
--verbose                        # Enable verbose logging

# Node manager configuration
--score-half-life=1h             # Time for score to decay by half
--recency-weight=0.3             # Weight given to recent results vs. historical
--base-exploration-rate=0.2      # Initial exploration rate (20%)
--min-exploration-rate=0.05      # Minimum exploration rate (5%)
--good-node-threshold=5          # Number of good nodes to reach min exploration
--test-interval=1m               # How often to test nodes
```

### Configuration Impact

#### Transaction Processing Rate

The transaction processing rate is primarily controlled by three parameters:

1. **CyclePauseDuration**: Shorter pauses increase the processing rate but may put more load on the network
2. **MaxTransactionsPerCycle**: Higher values increase throughput but may cause bursts of network activity
3. **MaxTransactionsPerPartitionPair**: Usually kept at 1 to ensure orderly processing, but can be increased for higher throughput

#### Node Selection Balance

The exploration vs. exploitation balance is controlled by:

1. **BaseExplorationRate**: Higher values (e.g., 0.3) increase exploration of new nodes
2. **MinExplorationRate**: Sets a floor on exploration to ensure new nodes are always discovered
3. **GoodNodeThreshold**: Determines how quickly the system transitions from exploration to exploitation

#### Time-Weighted Scoring

The time-weighted scoring system is controlled by:

1. **ScoreHalfLife**: Shorter values make the system more responsive to recent performance
2. **RecencyWeight**: Higher values prioritize recent results over historical performance

## Troubleshooting Guide

### Common Issues

1. **Transactions not being submitted**
   - Check if the node manager is finding appropriate nodes
   - Verify that the submission channel is being processed

2. **Synthetic transactions not being created**
   - Ensure the submission count is being properly incremented
   - Verify the logic for creating synthetic transactions after 10 attempts

3. **Network overload**
   - Check the pause durations between healing cycles
   - Ensure only one transaction per partition pair is being processed per cycle

### Debugging Steps

```
# Look for transaction creation
"No existing transaction found, creating new one" source=Apollo destination=Directory sequence=123

# Look for transaction resubmission
"Resubmitting existing transaction" source=Apollo destination=Directory sequence=123 submissions=5

# Look for synthetic transaction creation
"Transaction reached maximum submission attempts (10), creating new synthetic tx" source=Apollo destination=Directory sequence=123

# Look for node selection
"Using node for submission" partition=Apollo node=QmAbC... success_rate=85%
```

## Testing Strategy

This section outlines the comprehensive testing strategy for the transaction healing process. The testing approach is designed to verify the healing process's functionality, performance, and resilience under various conditions.

### Testing Objectives

1. **Functional Verification**: Ensure the healing process correctly identifies and heals gaps in anchor and synthetic transaction sequences
2. **URL Standardization**: Verify consistent URL construction across all components
3. **Node Selection**: Validate the time-weighted ranking system for node selection
4. **Transaction Reuse**: Confirm proper tracking and reuse of transactions
5. **Synthetic Alternatives**: Verify creation of synthetic alternatives after multiple failures
6. **Performance**: Measure and optimize the healing process's performance
7. **Resilience**: Test the system's ability to recover from various error conditions

### Test Categories

#### 1. Unit Tests

```go
func TestFindTransactionForGap(t *testing.T) {
    tracker := NewTransactionTracker()
    
    // Create and add a test transaction
    tx := &HealingTransaction{
        SourcePartition: "bvn-apollo",
        DestPartition:   "bvn-europa",
        SequenceNumber:  42,
    }
    tracker.AddTransaction(tx)
    
    // Test finding the transaction
    found := tracker.FindTransactionForGap("bvn-apollo", "bvn-europa", 42)
    assert.Equal(t, tx, found)
    
    // Test not finding a non-existent transaction
    notFound := tracker.FindTransactionForGap("bvn-apollo", "bvn-europa", 43)
    assert.Nil(t, notFound)
}

func TestNodeScoring(t *testing.T) {
    nm := NewNodeManager()
    
    // Report success for a node
    nm.ReportSuccess("bvn-apollo", "QmNodeID1", 100*time.Millisecond)
    
    // Get the best node
    bestNode, found := nm.GetBestNode("bvn-apollo")
    assert.True(t, found)
    assert.Equal(t, "QmNodeID1", bestNode)
    
    // Report failures and test score decay
    for i := 0; i < 5; i++ {
        nm.ReportFailure("bvn-apollo", "QmNodeID1")
    }
    
    // Report success for another node
    nm.ReportSuccess("bvn-apollo", "QmNodeID2", 150*time.Millisecond)
    
    // Best node should now be the second one
    bestNode, found = nm.GetBestNode("bvn-apollo")
    assert.True(t, found)
    assert.Equal(t, "QmNodeID2", bestNode)
}

func TestURLNormalization(t *testing.T) {
    // Test converting anchor pool URL to partition URL
    anchorPoolUrl, _ := url.Parse("acc://dn.acme/anchors/Apollo")
    normalized := normalizeUrl(anchorPoolUrl)
    
    expected, _ := url.Parse("acc://bvn-Apollo.acme")
    assert.Equal(t, expected.String(), normalized.String())
    
    // Test that partition URLs remain unchanged
    partitionUrl, _ := url.Parse("acc://bvn-Europa.acme")
    normalized = normalizeUrl(partitionUrl)
    assert.Equal(t, partitionUrl.String(), normalized.String())
}
```

#### 2. Integration Tests

```go
func TestHealingProcess(t *testing.T) {
    // Setup a mock network with controlled gaps
    mockNet := setupMockNetwork()
    
    // Create gaps in anchor sequences
    mockNet.CreateAnchorGap("bvn-apollo", "bvn-europa", 5)
    
    // Create gaps in synthetic sequences
    mockNet.CreateSyntheticGap("bvn-europa", "bvn-apollo", 3)
    
    // Run the healing process
    healer := NewHealer(mockNet)
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    go healer.Run(ctx)
    
    // Wait for healing to complete or timeout
    select {
    case <-ctx.Done():
        t.Fatal("Timeout waiting for healing to complete")
    case <-mockNet.AllGapsHealed():
        // Success
    }
    
    // Verify all gaps were healed
    assert.True(t, mockNet.VerifyNoGaps())
}

func TestURLStandardization(t *testing.T) {
    // Setup a mock network
    mockNet := setupMockNetwork()
    
    // Create a healer
    healer := NewHealer(mockNet)
    
    // Create a transaction with an anchor pool URL
    tx := &HealingTransaction{
        SourcePartition: "bvn-apollo",
        DestPartition:   "bvn-europa",
        SequenceNumber:  42,
        DestinationUrl:  parseUrl("acc://dn.acme/anchors/Europa"),
    }
    
    // Process the transaction
    healer.processTransaction(context.Background(), tx)
    
    // Verify URL was normalized
    assert.Equal(t, "acc://bvn-europa.acme", tx.DestinationUrl.String())
}
```

#### 3. Simulation Tests

```go
func TestNetworkSimulation(t *testing.T) {
    // Skip in short mode
    if testing.Short() {
        t.Skip("Skipping network simulation in short mode")
    }
    
    // Setup a simulated network with multiple partitions
    sim := NewNetworkSimulator(5) // 5 partitions
    
    // Configure random failures
    sim.SetNodeFailureRate(0.1) // 10% of node operations fail
    sim.SetPartitionFailureRate(0.05) // 5% chance of partition being unreachable
    
    // Run the simulation
    results := sim.RunHealingSimulation(1 * time.Hour)
    
    // Verify healing effectiveness
    assert.True(t, results.HealingSuccessRate > 0.95) // >95% success rate
    assert.True(t, results.AverageHealingTime < 30*time.Second) // <30s avg time
}
```

### Test Environment Setup

#### Mock Network

```go
// MockNetwork provides a controlled environment for testing the healing process
type MockNetwork struct {
    Partitions map[string]*MockPartition
    Gaps       map[string][]uint64 // key: "src:dst", value: gap sequence numbers
    gapsChan   chan struct{}
}

// MockPartition represents a partition in the mock network
type MockPartition struct {
    ID          string
    Nodes       map[string]*MockNode
    AnchorChain map[string]*MockChain // key: target partition
    SynthChain  map[string]*MockChain // key: target partition
}

// Setup a mock network for testing
func setupMockNetwork() *MockNetwork {
    net := &MockNetwork{
        Partitions: make(map[string]*MockPartition),
        Gaps:       make(map[string][]uint64),
        gapsChan:   make(chan struct{}),
    }
    
    // Create partitions
    partitions := []string{"bvn-apollo", "bvn-europa", "bvn-ganymede", "dn"}
    for _, id := range partitions {
        net.AddPartition(id)
    }
    
    // Setup connections between partitions
    for _, src := range partitions {
        for _, dst := range partitions {
            if src != dst {
                net.ConnectPartitions(src, dst)
            }
        }
    }
    
    return net
}
```

### Test Scenarios

#### 1. Normal Operation

- **Setup**: Create a network with all partitions and nodes functioning normally
- **Action**: Introduce gaps in anchor and synthetic sequences
- **Verification**: Confirm all gaps are healed within expected timeframes

#### 2. Node Failures

- **Setup**: Configure some nodes to consistently fail
- **Action**: Run the healing process
- **Verification**: Confirm the system identifies and avoids problematic nodes

#### 3. URL Standardization

- **Setup**: Create transactions with both URL formats
- **Action**: Process the transactions through the healing system
- **Verification**: Confirm all URLs are normalized to the standard format

#### 4. Transaction Reuse

- **Setup**: Create gaps that will require multiple submission attempts
- **Action**: Run the healing process with limited successful nodes
- **Verification**: Confirm transactions are properly reused and submission counts are tracked

#### 5. Synthetic Alternatives

- **Setup**: Configure some transactions to consistently fail
- **Action**: Run the healing process until submission count exceeds threshold
- **Verification**: Confirm synthetic alternatives are created and processed

### Performance Testing

#### 1. Throughput Testing

```go
func TestHealingThroughput(t *testing.T) {
    // Skip in short mode
    if testing.Short() {
        t.Skip("Skipping throughput test in short mode")
    }
    
    // Create a large number of gaps
    mockNet := setupMockNetwork()
    for i := 1; i <= 1000; i++ {
        mockNet.CreateAnchorGap("bvn-apollo", "bvn-europa", uint64(i))
    }
    
    // Run the healing process with different configurations
    configs := []struct {
        name                        string
        maxTxPerCycle              int
        cyclePause                 time.Duration
        maxTxPerPartitionPair      int
    }{
        {"baseline", 100, 5 * time.Second, 1},
        {"high-throughput", 200, 2 * time.Second, 2},
        {"balanced", 150, 3 * time.Second, 1},
    }
    
    for _, cfg := range configs {
        t.Run(cfg.name, func(t *testing.T) {
            // Reset the network
            mockNet.Reset()
            
            // Configure and run the healer
            healer := NewHealer(mockNet)
            healer.Config.MaxTransactionsPerCycle = cfg.maxTxPerCycle
            healer.Config.CyclePauseDuration = cfg.cyclePause
            healer.Config.MaxTransactionsPerPartitionPair = cfg.maxTxPerPartitionPair
            
            // Measure healing time
            start := time.Now()
            ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
            defer cancel()
            go healer.Run(ctx)
            
            // Wait for healing to complete
            select {
            case <-ctx.Done():
                t.Fatal("Timeout waiting for healing to complete")
            case <-mockNet.AllGapsHealed():
                elapsed := time.Since(start)
                t.Logf("Configuration %s: Healed 1000 gaps in %v", cfg.name, elapsed)
            }
        })
    }
}
```

#### 2. Resource Utilization

```go
func TestResourceUtilization(t *testing.T) {
    // Skip in short mode
    if testing.Short() {
        t.Skip("Skipping resource test in short mode")
    }
    
    // Setup test environment
    mockNet := setupMockNetwork()
    healer := NewHealer(mockNet)
    
    // Create a moderate number of gaps
    for i := 1; i <= 500; i++ {
        mockNet.CreateAnchorGap("bvn-apollo", "bvn-europa", uint64(i))
    }
    
    // Start monitoring resources
    var memStats runtime.MemStats
    var maxMemory uint64
    ticker := time.NewTicker(100 * time.Millisecond)
    done := make(chan struct{})
    
    go func() {
        for {
            select {
            case <-ticker.C:
                runtime.ReadMemStats(&memStats)
                if memStats.Alloc > maxMemory {
                    maxMemory = memStats.Alloc
                }
            case <-done:
                ticker.Stop()
                return
            }
        }
    }()
    
    // Run the healing process
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
    defer cancel()
    go healer.Run(ctx)
    
    // Wait for healing to complete
    select {
    case <-ctx.Done():
        t.Fatal("Timeout waiting for healing to complete")
    case <-mockNet.AllGapsHealed():
        // Success
    }
    
    // Stop resource monitoring
    close(done)
    
    // Report resource usage
    t.Logf("Peak memory usage: %d MB", maxMemory/(1024*1024))
    t.Logf("Transactions processed: %d", healer.Stats.ProcessedCount)
    t.Logf("Memory per transaction: %d KB", (maxMemory/uint64(healer.Stats.ProcessedCount))/1024)
}
```

### Continuous Integration

The testing strategy should be integrated into the CI/CD pipeline with the following components:

1. **Unit Tests**: Run on every commit
2. **Integration Tests**: Run on every PR and nightly
3. **Simulation Tests**: Run weekly and before releases
4. **Performance Tests**: Run weekly and before releases

### Test Coverage Goals

- **Unit Test Coverage**: >90% for core components
- **Integration Test Coverage**: >80% for end-to-end flows
- **Scenario Coverage**: All identified scenarios must have corresponding tests

### URL Standardization Testing

Special attention should be given to testing URL standardization, as this has been identified as a critical issue:

```go
func TestURLStandardizationComprehensive(t *testing.T) {
    testCases := []struct {
        name     string
        input    string
        expected string
    }{
        {
            name:     "anchor pool URL to partition URL",
            input:    "acc://dn.acme/anchors/Apollo",
            expected: "acc://bvn-Apollo.acme",
        },
        {
            name:     "partition URL remains unchanged",
            input:    "acc://bvn-Europa.acme",
            expected: "acc://bvn-Europa.acme",
        },
        {
            name:     "case insensitivity in authority",
            input:    "acc://DN.acme/anchors/Apollo",
            expected: "acc://bvn-Apollo.acme",
        },
        {
            name:     "case insensitivity in path",
            input:    "acc://dn.acme/ANCHORS/Apollo",
            expected: "acc://bvn-Apollo.acme",
        },
        {
            name:     "additional path components",
            input:    "acc://dn.acme/anchors/Apollo/extra",
            expected: "acc://bvn-Apollo.acme/extra",
        },
    }
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            inputURL, err := url.Parse(tc.input)
            require.NoError(t, err)
            
            normalized := normalizeUrl(inputURL)
            assert.Equal(t, tc.expected, normalized.String())
        })
    }
}
```

## Design Review

This section provides a comprehensive review of the transaction healing process design, evaluating its strengths, identifying potential improvements, and assessing its alignment with system requirements.

### Architecture Evaluation

#### Core Components

1. **Transaction Tracker**
   - **Strengths**: Efficient key-based lookup for transactions, thread-safe implementation with mutex protection, comprehensive statistics tracking
   - **Considerations**: Memory usage is well-controlled by the one-transaction-per-partition-pair design, which inherently limits the total number of transactions in memory

2. **Node Manager**
   - **Strengths**: Time-weighted scoring provides adaptive node selection, exploration vs. exploitation balance promotes network resilience
   - **Considerations**: Score decay parameters may need tuning based on network size and characteristics

3. **URL Standardization**
   - **Strengths**: Consistent URL format prevents routing errors, normalization simplifies code paths
   - **Considerations**: Transition period may be needed when deploying to existing networks

4. **Transaction Reuse Mechanism**
   - **Strengths**: Reduces unnecessary transaction creation, optimizes network usage
   - **Considerations**: Submission count is tracked via a lightweight wrapper around the transaction during processing, no persistent state required

#### Design Patterns

1. **Observer Pattern**
   - Used for monitoring transaction status changes and updating statistics
   - Provides clean separation between transaction processing and status tracking

2. **Factory Pattern**
   - Used for creating appropriate transaction types (anchor vs. synthetic)
   - Encapsulates creation logic and simplifies client code

3. **Strategy Pattern**
   - Used for node selection algorithms
   - Allows for different selection strategies based on network conditions

### Architectural Decisions

#### URL Standardization

The decision to standardize on raw partition URLs (e.g., `acc://bvn-Apollo.acme`) rather than anchor pool URLs (e.g., `acc://dn.acme/anchors/Apollo`) is well-justified:

1. **Consistency**: Provides a single, consistent way to reference partitions
2. **Simplicity**: Reduces the need for URL translation in different code paths
3. **Error Reduction**: Eliminates a class of errors related to URL format mismatches

#### Transaction Reuse

The transaction reuse mechanism strikes a good balance between:

1. **Efficiency**: Avoids creating duplicate transactions for the same gap
2. **Simplicity**: Uses a lightweight wrapper to track submission count without persistent state
3. **Adaptability**: Creates synthetic alternatives after multiple failures



#### Node Selection

The time-weighted node scoring system is a strong design choice:

1. **Recency**: More recent performance has higher weight, adapting to changing network conditions
2. **Exploration**: Prevents permanently blacklisting nodes, allowing recovery
3. **Performance**: Prioritizes nodes with better historical performance

### Scalability Analysis

#### Memory Usage

- **Transaction Tracker**: O(p²) memory usage where p is the number of partitions, as the system maintains at most one transaction per partition pair at a time
- **Node Manager**: O(m * p) memory usage where m is the number of nodes and p is the number of partitions

#### Time Complexity

- **Transaction Lookup**: O(1) using map-based lookup
- **Node Selection**: O(log m) for selecting the best node from m candidates
- **Gap Detection**: O(log n) for binary search in sequence gaps

#### Network Load

- **Controlled by**: MaxTransactionsPerCycle, CyclePauseDuration, MaxTransactionsPerPartitionPair
- **Impact**: Linear relationship between configuration parameters and network load

### Security Considerations

1. **Transaction Validation**: All transactions are validated before submission
2. **Node Authentication**: Secure connections to nodes prevent MITM attacks
3. **Error Handling**: Robust error handling prevents cascading failures

### Resilience Mechanisms

1. **No Permanent Failures**: The system is designed to eventually heal all gaps
2. **Adaptive Node Selection**: Poor-performing nodes are deprioritized but not permanently excluded
3. **Synthetic Alternatives**: After multiple failures, synthetic transactions provide an alternative healing path

### Design Tradeoffs

#### Simplicity vs. Optimization

- **Current Design**: Favors simplicity and reliability over maximum optimization
- **Rationale**: In a distributed system, predictable behavior is often more valuable than marginal performance gains
- **Future Consideration**: Performance optimizations can be added incrementally as needed

#### Memory Usage vs. Computation

- **Current Design**: Efficiently balances memory usage and computational efficiency through the one-transaction-per-partition-pair limitation
- **Rationale**: This approach provides optimal memory usage while maintaining high computational efficiency
- **Future Consideration**: If partition count grows significantly, the O(p²) memory usage could be optimized further

### Alignment with Requirements

#### Functional Requirements

1. **Gap Detection**: ✅ Efficiently identifies gaps in transaction sequences
2. **Transaction Healing**: ✅ Successfully heals gaps through resubmission or synthetic alternatives
3. **URL Standardization**: ✅ Consistently uses standardized URL formats

#### Non-Functional Requirements

1. **Performance**: ✅ Configurable parameters allow tuning for different network sizes
2. **Reliability**: ✅ No permanent failures philosophy ensures eventual consistency
3. **Maintainability**: ✅ Clear separation of concerns and well-defined interfaces

### Recommendations for Future Enhancements

1. **Adaptive Configuration**
   - Automatically adjust configuration parameters based on network conditions
   - Example: Increase CyclePauseDuration during periods of high network congestion

2. **Enhanced Metrics and Monitoring**
   - Add detailed performance metrics for each component
   - Implement real-time monitoring dashboards

3. **Enhanced Transaction Prioritization**
   - Implement more sophisticated prioritization strategies for healing critical gaps first
   - Allow for dynamic adjustment of the one-transaction-per-partition-pair limitation based on network conditions

4. **Predictive Node Selection**
   - Use machine learning to predict node performance based on historical patterns
   - Incorporate network topology awareness into node selection

5. **Transaction Prioritization**
   - Implement priority queues for critical transactions
   - Allow configurable prioritization strategies

### Conclusion

The transaction healing process design demonstrates a robust, well-thought-out approach to ensuring blockchain consistency. The standardization of URL formats, time-weighted node selection, and transaction reuse mechanism are particularly strong aspects of the design.

The system successfully balances the competing concerns of performance, reliability, and maintainability. The "no permanent failures" philosophy ensures eventual consistency, while the configurable parameters allow adaptation to different network sizes and conditions.

Future enhancements should focus on adaptive configuration, enhanced monitoring, and scaling strategies for very large networks. These improvements can be implemented incrementally without disrupting the solid foundation of the current design.

## Comparative Design Analysis: heal_v1 vs. New Design

This section provides a detailed comparison between the heal_v1 implementation and the new design, with special focus on URL standardization and API conformance.

### URL Standardization Analysis

#### URL Construction in heal_v1

In heal_v1, there were two different approaches to URL construction:

1. **Anchor Pool URL Format** (used in heal_anchor.go):
   ```go
   // Example from heal_anchor.go
   srcUrl := protocol.DnUrl().JoinPath(protocol.AnchorPool).JoinPath(src.ID)
   // Results in: acc://dn.acme/anchors/Apollo
   ```

2. **Raw Partition URL Format** (used in sequence.go):
   ```go
   // Example from sequence.go
   srcUrl := protocol.PartitionUrl(src.ID)
   // Results in: acc://bvn-Apollo.acme
   ```

This inconsistency led to several issues:
- Queries failing with "element does not exist" errors when checking the wrong URL format
- Inconsistent transaction tracking due to URL format differences
- Difficulties in transaction reuse because of URL mismatches
- **Routing conflicts** leading to "cannot route message" errors during submission
- Caching inefficiencies due to the same partition being referenced with different URL formats

#### URL Construction in New Design

The new design standardizes on the **Raw Partition URL Format** approach:

```go
// Standardized approach in new design
srcUrl := protocol.PartitionUrl(src.ID)
// Results in: acc://bvn-Apollo.acme
```

This standardization provides several benefits:
- **Consistency**: Provides a single, consistent way to reference partitions
- **Simplicity**: Reduces the need for URL translation in different code paths
- **Error Reduction**: Eliminates a class of errors related to URL format mismatches
- **Caching Efficiency**: Improves cache hit rates by using consistent URL keys

#### API Implementation Analysis

Examining the protocol package implementation confirms that `PartitionUrl` is the preferred and more robust method:

```go
// From protocol/protocol.go
// PartitionUrl returns `acc://bvn-${partition}.acme` or `acc://dn.acme`.
func PartitionUrl(partition string) *url.URL {
	if strings.EqualFold(partition, Directory) {
		return DnUrl()
	}
	return &url.URL{Authority: bvnUrlPrefix + partition + TLD}
}
```

This function handles both BVN partitions and the Directory Network (DN) correctly, making it the most versatile approach.

#### URL Normalization Implementation

The URL normalization function ensures consistent URL formats throughout the system:

```go
// URL normalization function
func normalizeUrl(u *url.URL) *url.URL {
	if u == nil {
		return nil
	}

	// If this is an anchor pool URL with a partition path, convert to partition URL
	if strings.EqualFold(u.Authority, protocol.Directory) && strings.HasPrefix(u.Path, "/anchors/") {
		parts := strings.Split(u.Path, "/")
		if len(parts) >= 3 {
			partitionID := parts[2]
			return protocol.PartitionUrl(partitionID)
		}
	}

	return u
}
```

This function correctly converts anchor pool URLs to partition URLs, which is the standardized approach in the new design.

#### URL Normalization Integration

URL normalization is integrated at key points in the system:

1. **Transaction Creation**: URLs are normalized when creating transactions
   ```go
   tx := &protocol.Transaction{
       Header: &protocol.TransactionHeader{
           Principal: normalizeUrl(protocol.PartitionUrl(src.ID)),
       },
   }
   ```

2. **Query Operations**: URLs are normalized before making queries
   ```go
   normalizedUrl := normalizeUrl(url)
   result, err := client.QueryAccount(ctx, normalizedUrl)
   ```

3. **Caching System**: The caching system uses normalized URLs as keys
   ```go
   cacheKey := fmt.Sprintf("%s:%s", normalizeUrl(url).String(), queryType)
   ```

4. **Transaction Submission**: URLs are normalized before submission to prevent routing conflicts
   ```go
   // Normalize all URLs in the envelope before submission
   normalizeEnvelopeUrls(envelope)
   ```

### Feature Comparison

| Feature | heal_v1 | New Design | Improvement |
|---------|---------|------------|-------------|
| **URL Standardization** | Inconsistent (two approaches) | Standardized on PartitionUrl | ✅ Eliminates URL-related errors |
| **Node Selection** | Simple scoring | Time-weighted scoring | ✅ Better adaptation to network conditions |
| **Transaction Reuse** | Limited | Comprehensive tracking and reuse | ✅ Reduces network load |
| **Error Recovery** | Basic retry | Sophisticated with synthetic alternatives | ✅ More resilient |
| **Configuration** | Hardcoded | Flexible and environment-configurable | ✅ More adaptable |

### API Conformance Analysis

#### Protocol Package Integration

The new design integrates correctly with the protocol package API:

1. **PartitionUrl Usage**:
   - The new design uses `protocol.PartitionUrl(partitionID)` consistently
   - This matches the implementation in protocol/protocol.go

2. **URL Parsing**:
   - The new design uses `ParsePartitionUrl` for extracting partition information
   - This matches the implementation in protocol/protocol.go

3. **Chain Access**:
   - The new design uses the correct chain access patterns:
     ```go
     // For anchor chains
     dst := protocol.PartitionUrl(part.ID)
     anchor := getAccount[*protocol.AnchorLedger](h, dst.JoinPath(protocol.AnchorPool))
     
     // For synthetic ledgers
     synth := getAccount[*protocol.SyntheticLedger](h, dst.JoinPath(protocol.Synthetic))
     ```

#### Transaction Submission Conformance

The transaction submission process in the new design correctly follows the API patterns:

1. **Transaction Creation**:
   ```go
   // Creating a synthetic transaction
   tx := &protocol.Transaction{
       Header: &protocol.TransactionHeader{
           Principal: protocol.PartitionUrl(src.ID),
       },
       Body: &protocol.SyntheticDepositTokens{
           // ...
       },
   }
   ```

2. **Transaction Submission**:
   ```go
   // Submit using the correct client API
   result, err := client.Execute(ctx, &api.ExecuteRequest{
       Envelope: envelope,
   })
   ```

### Implementation Differences

| Component | heal_v1 Implementation | New Design Implementation | API Conformance |
|-----------|------------------------|---------------------------|----------------|
| **URL Construction** | Mixed approaches | Standardized on PartitionUrl | ✅ Fully conformant |
| **Chain Queries** | Direct API calls | Abstracted through helper functions | ✅ Fully conformant |
| **Transaction Creation** | Manual construction | Factory pattern with standardized URLs | ✅ Fully conformant |
| **Error Handling** | Basic retry logic | Comprehensive with node scoring | ✅ Fully conformant |

### Conclusion

The new design significantly improves upon heal_v1 by:

1. **Standardizing URL Construction**: Consistently using `protocol.PartitionUrl()` eliminates a major source of errors

2. **Conforming to Protocol APIs**: The new design correctly integrates with the protocol package APIs

3. **Enhancing Resilience**: The time-weighted node scoring and transaction reuse mechanisms provide better adaptation to network conditions

4. **Improving Configuration**: The flexible configuration system allows for better tuning of the healing process

The URL standardization approach in the new design exactly conforms to the API implementation in the protocol package, ensuring consistent and reliable operation across the system.

## Transaction Submission Policy

The transaction healing process implements a straightforward submission policy that balances persistence with network resource utilization.

### No Retry Mechanism

**Design Decision**: The current design intentionally does not include a retry mechanism within a single healing cycle

**Rationale**:
- Previous implementations with retry mechanisms caused problems with network backoff
- Retries within a single cycle can overwhelm individual peers
- A simpler approach with one submission per cycle has proven more reliable

### Overall Healing Cycle Limit (10 cycles max)

**Purpose**: Prevents endlessly attempting transactions that consistently fail across multiple cycles

**Implementation Details**:
- A transaction can be attempted across a maximum of **10 healing cycles**
- The `SubmissionCount` field in `HealingTransaction` tracks the number of submission attempts
- After 10 cycles with no success, the transaction is removed from the tracker and a synthetic alternative is created
- This prevents the system from getting stuck on problematic transactions

```go
// From heal_transaction_tracking.go
const maxSubmissionAttempts = 10

// In processGap
existingTx := h.txTracker.FindTransactionForGap(src.ID, dst.ID, gapNum)
if existingTx != nil {
    if existingTx.SubmissionCount < maxSubmissionAttempts {
        existingTx.SubmissionCount++
        existingTx.LastAttemptTime = time.Now()
        h.submit <- existingTx
        h.txTracker.UpdateTransactionStatus(existingTx, TxStatusPending)
        return true, nil
    }
    return h.createSyntheticAlternative(ctx, src, dst, gapNum)
}
```

## Summary

- **No Retry Mechanism**: Each transaction is submitted exactly once per healing cycle
- **Cycle Limit**: Maximum of 10 submission attempts across different healing cycles
- **Synthetic Alternatives**: After 10 failed cycles, create a synthetic transaction as an alternative approach
- **No Permanent Failures**: Every entry must eventually be healed
- **Network Friendly**: Controlled pacing with appropriate pauses between cycles
- **Prioritization**: Process the lowest sequence numbers first for logical healing order
