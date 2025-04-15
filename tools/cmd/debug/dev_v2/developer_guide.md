# Developer's Guide to Version 2 Implementation

This guide is designed for developers and AI assistants who need to understand, modify, or build upon the Version 2 code in the Accumulate Network. It provides a practical, code-focused approach to navigating the implementation.

This guide builds upon the foundation established in the [Version 1 Developer Guide](../heal_v1/developer_guide.md), focusing specifically on the changes and improvements made in Version 2. The design emphasizes a "no permanent failures" philosophy, ensuring that every transaction must eventually be healed.

## Documentation Structure

The Version 2 documentation is organized into the following components:

1. **Developer Guide** (this document): High-level overview of all components and features
2. **[Implementation Guide](./heal_all_implementation.md)**: Detailed code implementation with examples
3. **[Reporting Design](./heal_all_reporting.md)**: Specific design for the reporting system
4. **[Transaction Healing Process](./transaction_healing_process.md)**: Comprehensive design documentation for the transaction healing process

## Recent Enhancements

### Heal-All Command Improvements

The `heal-all` command has been enhanced with the following improvements:

1. **Process All Partition Pairs**: The command now processes all partition pairs in each cycle, healing one gap per pair if needed, rather than stopping after the first healing.

2. **Pre-initialization of Report**: The report is pre-initialized with all possible partition pairs, ensuring all pairs appear in the report even if they don't need healing. See the [Reporting Design](./heal_all_reporting.md#data-structure) for details.

3. **Comprehensive Reporting**: Added detailed reporting functionality to track and display the state of each partition pair for both anchoring and synthetic transactions. See the [Reporting Design](./heal_all_reporting.md) for the full specification.

4. **Controlled Pacing Refinement**: Improved the pacing mechanism to prevent DOS issues while ensuring efficient healing across the network. Instead of a single `healed` flag, we use `totalHealedThisCycle` to track all healing operations, allowing for more granular control over pause durations. See the [Implementation Guide](./heal_all_implementation.md#healing-process) for code details.

5. **Enhanced Error Handling**: Better error tracking for both anchor and synthetic transaction processing, with errors properly displayed in the report. See the [Reporting Design](./heal_all_reporting.md#data-structure) for the error tracking structure.

### Stable Node Submission Approach

A new stable node submission approach has been implemented to address issues with the P2P backoff mechanism:

1. **Primary Node Focus**: Each partition now maintains a primary node for submissions, reducing the number of different peers contacted.

2. **Minimal Fallback Strategy**: If the primary node fails, only ONE additional peer with the best success history is tried.

3. **Controlled Retry Timing**: Added delays between submission attempts to prevent triggering backoff mechanisms.

4. **Conflicting Routes Tracking**: Detailed statistics are collected on routing errors to help diagnose URL construction issues.

5. **Transaction Submission Policy**: Each transaction is submitted exactly once per healing cycle:
   - No retry mechanism within a single cycle
   - Failed transactions are not retried immediately but will be attempted again in the next healing cycle
   - Maximum of 10 submission attempts across different healing cycles
   - After 10 cycles with no success, a synthetic alternative is created
   - This prevents overwhelming the network while ensuring eventual consistency

6. **No Retry Mechanism**: The current design intentionally does not include a retry mechanism within a single healing cycle:
   - Previous implementations with retry mechanisms caused problems with network backoff
   - Retries within a single cycle can overwhelm individual peers
   - A simpler approach with one submission per cycle has proven more reliable
   - Each transaction is submitted exactly once per healing cycle
   - After 10 cycles with no success, a synthetic alternative is created
   - This approach ensures chain continuity and prevents the healing process from getting stuck

7. **Conservative Transaction Ordering**: The implementation takes a conservative approach to transaction ordering:
   - Strictly maintains sequence number ordering within each partition pair
   - Never relies on the network's ability to handle out-of-order transactions
   - Ensures no higher sequence number transaction is submitted until all lower ones succeed
   - Provides clear deadlock prevention through maximum retry limits
   - Enables independent progress for different partition pairs

For implementation details, see the [Implementation Guide](./heal_all_implementation.md#transaction-resubmission-limits).

## Quick Start for Developers

### Key Components and Their Purposes

| Component | Purpose | Key Features |
|-----------|---------|--------------|
| In-Memory Database | Replaces Bolt DB for performance | Thread-safe, ACID-compliant, non-persistent |
| URL Construction | Standardized approach for URLs | Uses `protocol.PartitionUrl(id)` pattern |
| Caching System | Reduces redundant network requests | Thread-safe, tracks problematic nodes |
| Fast Sync | Optimizes chain synchronization | Efficient handling of large data sets |
| Peer Management | Handles node discovery and connection | Improved reliability for cross-partition operations |
| Stable Node Submission | Prevents triggering P2P backoff | Primary node focus, minimal fallbacks, controlled timing |

### Common Debugging Scenarios

1. **"Element does not exist" errors**:
   - Check URL construction (now standardized on `protocol.PartitionUrl(id)`)
   - Verify the caching system is working correctly
   - Ensure you're querying the correct partition

2. **Performance issues**:
   - Check for lock contention in the in-memory database
   - Verify caching is being utilized properly
   - Examine memory usage patterns
   - Verify the transaction wrapper is properly cleaning up completed transactions

3. **Cross-partition transaction failures**:
   - Verify URL construction is consistent
   - Check peer connectivity and node health
   - Examine transaction routing logic
   - Verify that the transaction wrapper is correctly tracking submission attempts

4. **"Cannot route message" errors**:
   - Check for inconsistent URL formats in the same transaction
   - Verify the URL normalization is working correctly
   - Check the conflicting routes statistics in the logs
   - Ensure all URLs are constructed using `protocol.PartitionUrl(id)`

5. **P2P backoff mechanism issues**:
   - Verify that there is no retry mechanism in the implementation
   - Check that each transaction is submitted exactly once per healing cycle
   - Verify that submission attempts are properly spaced out between cycles
   - Verify that URL normalization is being applied before submission
   - Examine the routing table configuration
   - See [Routing Errors](../heal_v1/routing_errors.md) for detailed troubleshooting

6. **Transaction reuse issues**:
   - Verify the transaction tracker is correctly storing and retrieving transactions
   - Check that the submission count is being properly incremented
   - Ensure the transaction wrapper is correctly tracking status changes
   - Verify that synthetic alternatives are created after the maximum retry limit

## Code Generation Templates

### Transaction Wrapper Implementation

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
    
    // Additional metadata
    TxID        string
    IsAnchor    bool   // Whether this is an anchor transaction
    IsSynthetic bool   // Whether this is a synthetic transaction
}
```

### Transaction Lifecycle Management

```go
// Creating a new transaction wrapper
tx := &HealingTransaction{
    SourcePartition: src.ID,
    DestPartition:   dst.ID,
    SequenceNumber:  gapNum,
    Transaction:     createTransaction(src, dst, gapNum),
    CreationTime:    time.Now(),
    Status:          TxStatusCreated,
    SubmissionCount: 0,
}

// Adding to transaction tracker
tracker.AddTransaction(tx)

// Updating before submission
tx.SubmissionCount++
tx.LastAttemptTime = time.Now()
tx.Status = TxStatusPending

// Updating after submission
if success {
    tx.Status = TxStatusSubmitted
} else {
    tx.Status = TxStatusFailed
}

// Reusing an existing transaction
existingTx := tracker.FindTransactionForGap(src.ID, dst.ID, gapNum)
if existingTx != nil && existingTx.SubmissionCount < MaxSubmissionAttempts {
    existingTx.SubmissionCount++
    existingTx.LastAttemptTime = time.Now()
    submitTransaction(ctx, existingTx)
}

// Cleaning up a transaction
tracker.RemoveTransaction(tx)
```

### URL Construction (Standardized)

```go
// CORRECT: Standardized URL construction
srcUrl := protocol.PartitionUrl(src.ID)  // e.g., acc://bvn-Apollo.acme

// INCORRECT: Old approach (do not use)
// srcUrl := protocol.DnUrl().JoinPath(protocol.AnchorPool).JoinPath(src.ID)
```

### Caching Implementation

```go
// Cache implementation
type cache struct {
    cache map[cacheKey]interface{}
    mutex sync.RWMutex
}

type cacheKey struct {
    url string
    typ string
}

// Thread-safe cache access
func (c *cache) Get(url, typ string) (interface{}, bool) {
    c.mutex.RLock()
    defer c.mutex.RUnlock()
    value, ok := c.cache[cacheKey{url, typ}]
    return value, ok
}

func (c *cache) Set(url, typ string, value interface{}) {
    c.mutex.Lock()
    defer c.mutex.Unlock()
    c.cache[cacheKey{url, typ}] = value
}

// Example usage
if value, ok := cache.Get(url, "account"); ok {
    // Use cached value
    return value, nil
}

// Query and cache the result
result, err := client.QueryAccount(ctx, url)
if err != nil {
    return nil, err
}
cache.Set(url, "account", result)
return result, nil
```

### In-Memory Database Interaction

```go
// Initialize the in-memory database
db := memdb.New()

// Open a batch (transaction)
batch := db.Begin(false)  // false = read-only
defer batch.Discard()  // Always defer discard to prevent resource leaks

// Read operations
value, err := batch.Get(bucket, key)

// Write operations (in a writable transaction)
writeBatch := db.Begin(true)
defer writeBatch.Discard()
err := writeBatch.Put(bucket, key, value)
if err != nil {
    return err
}
err = writeBatch.Commit()
if err != nil {
    return err
}
```

## Reporting System Implementation

For the complete reporting system design and implementation details, see the [Reporting Design](./heal_all_reporting.md) document.

### TableWriter Usage Guidelines

All reporting in Version 2 should use the `github.com/olekukonko/tablewriter` package for consistent formatting:

```go
// Initialize a new table
table := tablewriter.NewWriter(os.Stdout)

// Configure the table
table.SetHeader([]string{"Column1", "Column2", "Column3"})
table.SetBorder(false)                            // No outer border
table.SetColumnSeparator(" | ")                   // Column separator
table.SetCenterSeparator("+")                     // Intersection character
table.SetHeaderAlignment(tablewriter.ALIGN_CENTER) // Center-align headers
table.SetAlignment(tablewriter.ALIGN_CENTER)       // Center-align content

// Set header colors
table.SetHeaderColor(
    tablewriter.Colors{tablewriter.Bold},
    tablewriter.Colors{tablewriter.Bold},
    tablewriter.Colors{tablewriter.Bold},
)

// Add rows with color coding
table.Rich([]string{
    "Value1",
    "Value2",
    "Status",
}, []tablewriter.Colors{
    {},  // No color for column 1
    {},  // No color for column 2
    tablewriter.Colors{tablewriter.FgGreenColor},  // Green for status
})

// Add a footer
table.SetFooter([]string{"Total", "Value", ""})
table.SetFooterAlignment(tablewriter.ALIGN_CENTER)
table.SetFooterColor(
    tablewriter.Colors{tablewriter.Bold},
    tablewriter.Colors{tablewriter.Bold},
    tablewriter.Colors{},
)

// Render the table
table.Render()
```

### Color Coding Standards

Use consistent color coding across all reports:

| Status | Color | Code |
|--------|-------|------|
| OK | Green | `tablewriter.Colors{tablewriter.FgGreenColor}` |
| Warning | Yellow | `tablewriter.Colors{tablewriter.FgYellowColor}` |
| Info | Cyan | `tablewriter.Colors{tablewriter.FgCyanColor}` |
| Error | Red | `tablewriter.Colors{tablewriter.FgRedColor, tablewriter.Bold}` |
| Emphasis | Bold | `tablewriter.Colors{tablewriter.Bold}` |

## Healing Process Guidelines

### Controlled Pacing Implementation

The heal-all command uses controlled pacing to prevent DOS issues. For the full implementation, see the [Implementation Guide](./heal_all_implementation.md#healing-process):

```go
// Use different pause durations based on healing activity
if totalHealedThisCycle > 0 {
    // Longer pause after healing to give the network time to process
    select {
    case <-time.After(healPause):
    case <-ctx.Done():
        return
    }
} else {
    // Shorter pause when no healing was needed
    select {
    case <-time.After(cyclePause):
    case <-ctx.Done():
        return
    }
}
```

### Processing All Partition Pairs

To ensure comprehensive healing:

1. Pre-initialize the report with all possible partition pairs
2. Process one gap per partition pair in each cycle
3. Track healing counts across all pairs
4. Continue until all pairs are healed

## Critical Implementation Details

### Transaction Wrapper Implementation

The transaction wrapper is a key design element that manages the lifecycle of healing transactions without requiring persistent state:

**Key Features:**
- Lightweight in-memory tracking of transaction metadata
- Comprehensive tracking of submission attempts, timestamps, and status
- Clean lifecycle management from creation to completion
- Efficient reuse decisions based on submission history
- Memory-efficient design with one transaction per partition pair

**Benefits:**
- No persistent state required for transaction tracking
- Reduced network load through intelligent reuse
- Clear visibility into transaction status and history
- Simplified transaction management code

**Implementation Details:**
- The `HealingTransaction` struct wraps a transaction with metadata
- The `TransactionTracker` manages a map of transactions indexed by partition pair and sequence number
- Transactions are reused until they succeed or reach the maximum retry threshold
- After maximum retries, synthetic alternatives are created

### URL Construction Standardization

There were two different URL construction methods used in the codebase:

```go
// Method 1: Used in sequence.go (now standardized)
srcUrl := protocol.PartitionUrl(src.ID)  // e.g., acc://bvn-Apollo.acme

// Method 2: Previously used in heal_anchor.go (deprecated)
srcUrl := protocol.DnUrl().JoinPath(protocol.AnchorPool).JoinPath(src.ID)  // e.g., acc://dn.acme/anchors/Apollo
```

**Impact of the Issue:**
- The code was looking for anchors at different URL paths
- Queries were returning "element does not exist" errors when checking the wrong URL format
- Anchor relationships were not properly maintained between partitions
- **Routing conflicts** leading to "cannot route message" errors during submission

**Resolution:**
- Standardized on Method 1 (the `sequence.go` approach)
- Updated all code to use the same URL construction logic
- Ensured the caching system is aware of the correct URL format
- Implemented URL normalization to handle routing conflicts

For details on the original URL construction and chain relationships in Version 1, refer to the [Chains and URLs](../heal_v1/chains_urls.md) documentation. For comprehensive information about the routing system and how it impacts URL handling, see the [Routing System](../heal_v1/route.md) and [Routing Errors](../heal_v1/routing_errors.md) documentation.

### Caching System Implementation

The caching system reduces redundant network requests:

**Key Features:**
- Thread-safe implementation with read/write locks
- Composite cache keys combining URL and query type
- Tracking of problematic nodes to avoid querying them

**Benefits:**
- Reduces "element does not exist" errors by caching negative results
- Improves performance by avoiding redundant network requests
- Enhances reliability by routing around problematic nodes

### In-Memory Database vs. Bolt DB

**Key Differences:**
- In-memory storage vs. disk-based storage
- No persistence between restarts (by design)
- Optimized for in-memory access patterns
- Reduced lock contention for read operations

**Interface Compatibility:**
- Implements the same interfaces as Bolt DB (detailed in [Version 1 Database](../heal_v1/database.md))
- Maintains ACID transaction semantics
- Uses the same key structure for compatibility

The in-memory implementation builds on the database architecture described in the [Version 1 Database Documentation](../heal_v1/database.md), maintaining the same interfaces while changing the underlying storage mechanism.

## Version-Specific Considerations

### Pre-V2 Baikonur Networks

- Uses `BadSyntheticMessage` for synthetic transactions
- Different URL construction patterns
- No caching system

### V2 Baikonur Networks

- Uses `SyntheticMessage` instead of `BadSyntheticMessage` (pre-Baikonur networks used `BadSyntheticMessage`)
- The name "Bad" doesn't indicate a fault - it refers to the original implementation that was later improved upon in the Baikonur release
- The code checks `args.NetInfo.Status.ExecutorVersion.V2BaikonurEnabled()` to determine which message type to use
- Additional validation requirements for synthetic transactions
- Different signature verification process

## Testing Your Implementation

1. **Unit Testing**:
   - Test URL construction with different partition IDs
   - Test caching with various scenarios (hits, misses, expiration)
   - Test in-memory database with concurrent operations

2. **Integration Testing**:
   - Test with different network configurations
   - Test error cases and recovery
   - Test with different transaction types

3. **Performance Testing**:
   - Test caching effectiveness
   - Compare performance against Bolt DB implementation

## Future Enhancements

The following enhancements are planned for future versions:

1. **Historical Tracking**: Store historical reports to track healing progress over time
2. **Export Functionality**: Allow exporting reports to JSON or CSV for external analysis
3. **Detailed Drill-Down**: Allow viewing detailed information about specific partition pairs
4. **Automated Recommendations**: Suggest actions based on the healing status
5. **Enhanced Routing Diagnostics**: Expand the routing statistics to provide more detailed analysis of URL construction issues
6. **Primary Node Performance Metrics**: Track and report on the performance of primary nodes versus fallback nodes
7. **Backoff Detection**: Implement detection of P2P backoff triggers to further optimize the stable node submission approach
   - Test with large transaction volumes

## See Also

### Version 2 Documentation
- [Transaction Healing Process](./transaction_healing_process.md)
- [Implementation Guide](./heal_all_implementation.md)
- [Reporting Design](./heal_all_reporting.md)
- [Overview](./overview.md)
- [In-Memory Database Implementation](./mem_db.md)
- [Fast Sync Implementation](./fast_sync.md)
- [Synthetic Chains](./synthetic_chains.md)
- [URL Diagnostics](./url_diagnostics.md)
- [Peer Management](./peers.md)
- [Routing and URL Handling](./routing_and_url_handling.md)

### Version 1 Documentation
- [Developer Guide](../heal_v1/developer_guide.md)
- [Implementation Details](../heal_v1/implementation.md)
- [Chains and URLs](../heal_v1/chains_urls.md)
- [Routing Errors](../heal_v1/routing_errors.md)
- [Database Architecture](../heal_v1/database.md)
- [Transactions](../heal_v1/transactions.md)
- [Light Client](../heal_v1/light_client.md)
- [Chains and URLs](../heal_v1/chains_urls.md)
- [Routing System](../heal_v1/route.md)
- [Routing Errors](../heal_v1/routing_errors.md)
