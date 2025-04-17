# Heal-All Implementation Guide

## Overview

This document provides implementation details and guidelines for the enhanced `heal-all` command in the Accumulate Network. For a high-level overview of the features and components, see the [Developer Guide](./developer_guide.md).

This document focuses on the code implementation details and examples for developers working on the codebase.

## Implementation Focus Areas

This document covers the implementation details for the following key components:

1. **Stable Node Submission**: Code implementation for the stable node submission approach
2. **Healing Process**: Main loop implementation for processing all partition pairs
3. **Reporting System**: Implementation of the comprehensive reporting system

For the conceptual overview of these enhancements, see the [Developer Guide](./developer_guide.md#recent-enhancements).

## Code Implementation Details

## Stable Node Submission

The stable node submission approach addresses issues with the P2P backoff mechanism by significantly reducing the number of submission attempts. For the conceptual overview, see the [Stable Node Submission Approach](./developer_guide.md#stable-node-submission-approach) section in the Developer Guide.

```go
// stableNodeSubmitLoop is a version of submitLoop that focuses on using stable nodes
// for submissions to avoid triggering backoff mechanisms
func (h *healer) stableNodeSubmitLoop(wg *sync.WaitGroup) {
    // Track primary nodes for each partition
    primaryNodes := make(map[string]peer.ID)
    
    // Track success/failure counts for nodes
    successCounts := make(map[string]int)
    failureCounts := make(map[string]int)
    
    // Track last test time for nodes
    lastTestTime := make(map[string]time.Time)
    
    // Statistics for tracking routing conflicts
    stats := &routingStats{}
    
    // Main submission loop
    for {
        // Process messages
        // ...
        
        // Try primary node first
        if primaryNode, hasPrimary := primaryNodes[targetPartition]; hasPrimary {
            // Try submission with primary node
            // ...
        }
        
        // If primary node submission failed, try at most ONE additional peer
        if !submitted {
            // Find the peer with the highest success count
            // Try submission with this single peer only
            // ...
        }
        
        // Add delay between retries to avoid pounding peers
        if !submitted {
            time.Sleep(5 * time.Second)
        }
    }
}
```

The implementation follows the design principles outlined in the [Developer Guide](./developer_guide.md#stable-node-submission-approach), with the following key aspects in the code:

1. **Primary Node Selection**: Each partition maintains a primary node for submissions in the `primaryNodes` map
2. **Success/Failure Tracking**: The `successCounts` and `failureCounts` maps track node performance
3. **Controlled Retry Timing**: The `time.Sleep(5 * time.Second)` adds delay between submission attempts
4. **Conflicting Routes Tracking**: The `routingStats` structure collects detailed statistics on routing errors

### Transaction Resubmission Limits

The implementation strictly limits transaction resubmission attempts to prevent overwhelming peers and triggering backoff mechanisms:

1. **Maximum Two Attempts Per Cycle**: Each transaction is attempted at most twice per healing cycle:
   - First attempt: Using the primary node for the target partition
   - Second attempt: Using at most one additional peer with the best success history

2. **No Further Fallbacks**: Unlike previous implementations, there are no fallbacks to other partitions or random nodes

3. **Submission Cache**: Successfully submitted transactions are tracked in a cache to prevent redundant submissions:
   ```go
   // Create a cache for successful submissions to avoid redundant work
   submissionCache := make(map[string]bool)
   
   // Later in the code when submission succeeds:
   submissionCache[cacheKey] = true
   ```

4. **Failed Submission Handling**: If both submission attempts fail:
   - The transaction isn't retried immediately
   - A delay is enforced: `time.Sleep(5 * time.Second)`
   - The transaction will be retried in the next healing cycle

5. **API Resubmission Support**: The Accumulate API allows resubmitting failed transactions because:
   - The `Submit` method in the API interface accepts any valid envelope
   - There's no state in the API that prevents resubmission
   - The network handles duplicate submissions gracefully

This approach ensures that no transaction is pounded against the network repeatedly, while still allowing for eventual consistency through the cyclical nature of the healing process.

### Prioritizing Conflicting Routes Failures

Transactions that fail specifically with "conflicting routes" errors require special handling, as these failures often indicate URL construction issues rather than network problems. Importantly, **if we fail to submit an entry for a partition pair, any following anchors or synthetic transactions cannot be submitted** due to the sequential dependency between transactions.

#### Conservative Transaction Ordering Approach

While the Accumulate network technically has some ability to handle out-of-order transactions, our implementation takes a conservative approach that does not rely on this capability. Instead, we strictly maintain transaction order through the following mechanisms:

1. **Partition-Pair Tracking Structure**: We'll track failed transactions by partition pair to maintain the sequential dependency:
   ```go
   // Track transactions that fail with conflicting routes errors
   type conflictingRoutesEntry struct {
       envelope *messaging.Envelope
       timestamp time.Time
       attempts int
       sourcePartition string      // Source partition ID
       destinationPartition string // Destination partition ID
       sequenceNumber uint64       // Sequence number of the transaction
   }
   
   // Map of partition pairs to ordered queues of failed transactions
   // Key format: "sourcePartition:destinationPartition"
   conflictingRoutesMap := make(map[string][]*conflictingRoutesEntry)
   ```

2. **Failure Detection and Tracking**: When a transaction fails with a conflicting routes error, it's added to the appropriate partition pair queue:
   ```go
   if strings.Contains(err.Error(), "conflicting routes") {
       // Record statistics
       stats.recordConflictingRoutes(true)
       
       // Create the partition pair key
       pairKey := fmt.Sprintf("%s:%s", sourcePartition, destinationPartition)
       
       // Add to the appropriate queue
       entry := &conflictingRoutesEntry{
           envelope: env,
           timestamp: time.Now(),
           attempts: 1,
           sourcePartition: sourcePartition,
           destinationPartition: destinationPartition,
           sequenceNumber: sequenceNumber,
       }
       
       // Ensure entries are ordered by sequence number
       conflictingRoutesMap[pairKey] = append(conflictingRoutesMap[pairKey], entry)
       sort.Slice(conflictingRoutesMap[pairKey], func(i, j int) bool {
           return conflictingRoutesMap[pairKey][i].sequenceNumber < conflictingRoutesMap[pairKey][j].sequenceNumber
       })
   }
   ```

3. **Sequential Prioritized Resubmission**: Before processing new healing tasks for a partition pair, we first attempt to resolve any pending conflicting routes entries for that pair:
   ```go
   // Maximum attempts for conflicting routes entries
   const maxConflictingRoutesAttempts = 2
   
   // Before processing new healing tasks for a partition pair
   pairKey := fmt.Sprintf("%s:%s", sourcePartition, destinationPartition)
   if entries, exists := conflictingRoutesMap[pairKey]; exists && len(entries) > 0 {
       // Process entries in sequence number order
       entry := entries[0]
       
       // Attempt resubmission
       submitted, err := attemptSubmission(entry.envelope)
       
       if submitted {
           // Success - remove from queue
           conflictingRoutesMap[pairKey] = entries[1:]
           slog.Info("Successfully resubmitted previously failed transaction",
               "source", entry.sourcePartition,
               "destination", entry.destinationPartition,
               "sequence", entry.sequenceNumber)
               
           // Don't process new healing tasks for this pair until next cycle
           // to allow the network to process this transaction
           return
       } else {
           // Failed again - increment attempt count
           entry.attempts++
           
           // If too many attempts, remove and log
           if entry.attempts >= maxConflictingRoutesAttempts {
               conflictingRoutesMap[pairKey] = entries[1:]
               slog.Warn("Giving up on transaction with conflicting routes",
                   "source", entry.sourcePartition,
                   "destination", entry.destinationPartition,
                   "sequence", entry.sequenceNumber,
                   "attempts", entry.attempts)
           }
           
           // Don't process new healing tasks for this pair since we can't get past this entry
           return
       }
   }
   
   // Only proceed with new healing tasks if there are no pending failed entries
   // for this partition pair
   ```

4. **Strict Sequential Processing**: This approach ensures that:
   - We don't attempt to heal newer gaps until older ones are resolved
   - The sequential dependency between transactions is strictly maintained
   - Each partition pair is handled independently
   - We never rely on the network's ability to handle out-of-order transactions
   - Failed transactions get multiple chances to succeed before we give up
   - If a transaction permanently fails after maximum retries, we log it clearly and move on to prevent the healing process from getting permanently stuck

5. **Healing Cycle Integration**: The healing cycle will check for pending conflicting routes entries for each partition pair before attempting to heal new gaps:
   ```go
   // For each partition pair in the healing cycle
   for _, src := range partitions {
       for _, dst := range partitions {
           // Skip self-anchors for non-DN partitions
           if src.ID == dst.ID && !strings.EqualFold(src.ID, protocol.Directory) {
               continue
           }
           
           // First try to resolve any pending conflicting routes entries
           pairKey := fmt.Sprintf("%s:%s", src.ID, dst.ID)
           if handlePendingConflictingRoutes(pairKey) {
               // Skip to next pair if we processed a pending entry
               continue
           }
           
           // Only proceed with normal healing if no pending entries exist
           // Process anchor gaps first, then synthetic gaps
           // ...
       }
   }
   ```

This approach ensures that we respect the sequential dependency between transactions while still making multiple attempts to resolve transactions that failed due to routing issues. By organizing the failed transactions by partition pair and sequence number, we maintain the correct submission order and prevent the healing process from getting stuck.

#### Implementation Guarantees

Our implementation provides the following guarantees regarding transaction ordering:

1. **Strict Sequence Ordering**: Transactions are always processed in sequence number order within each partition pair

2. **No Out-of-Order Submissions**: We never attempt to submit a higher sequence number transaction until all lower sequence number transactions have been successfully submitted

3. **Partition Pair Independence**: Each partition pair maintains its own independent queue, allowing different pairs to progress independently

4. **Deadlock Prevention**: The maximum retry limit ensures that even if some transactions permanently fail, the healing process can eventually continue

5. **Transparent Failure Handling**: Clear logging of permanently failed transactions allows for manual intervention if needed

These guarantees ensure a robust and reliable healing process that maintains data integrity while still being resilient to temporary failures.

## Enhanced Transaction Reuse Strategy

A key optimization in our implementation is to prioritize reusing existing healing transactions before creating new ones. This approach has several benefits:

1. **Reduces Transaction Creation Overhead**: Creating new transactions can be expensive, especially for complex anchor or synthetic transactions.

2. **Maintains Transaction History**: By reusing the same transaction, we maintain a consistent history of attempts for a specific gap.

3. **Avoids Duplicate Submissions**: Multiple healing transactions for the same gap could potentially conflict with each other.

4. **Allows for Rebuilding After Persistent Failures**: After a significant number of failed attempts (10), we rebuild the transaction to address potential issues with the original transaction.

The implementation follows this process flow:

1. When a gap is detected, first check if we already have a transaction for that specific gap (source, destination, sequence number)
2. If an existing transaction is found:
   - If it has been attempted fewer than 10 times, reuse it
   - If it has been attempted 10 or more times, discard it and create a new one
3. Only create a new transaction if no existing one is found or if the existing one has exceeded the retry limit

This approach ensures we give each transaction a fair chance to succeed while still allowing for transaction rebuilding in case of persistent failures.

## Implementation Plan: Code Changes Needed

After reviewing the current code against our design, the following modifications are needed to implement the partition-pair based tracking for conflicting routes errors:

### 1. Enhanced Transaction Tracking Structure

```go
// Add to a new file called heal_transaction_tracking.go

// HealingTransaction represents a transaction that needs to be sent from one partition to another
type HealingTransaction struct {
    // Core transaction data
    Envelope         *messaging.Envelope
    SourcePartition  string
    DestPartition    string
    DestinationUrl   *url.URL  // The specific URL at the destination
    SequenceNumber   uint64
    
    // Tracking information
    FirstAttemptTime time.Time
    LastAttemptTime  time.Time
    AttemptCount     int
    LastError        error
    
    // Status tracking
    Status           HealingTxStatus
    ExpectedHeight   uint64          // The height at which we expect this tx to be visible
    LastCheckedTime  time.Time       // Last time we checked if the tx was present
    
    // Additional metadata for debugging and analysis
    TxID             string          // Transaction ID for logging
    IsAnchor         bool            // Whether this is an anchor transaction
    IsSynthetic      bool            // Whether this is a synthetic transaction
    Priority         int             // Priority level (higher = more important)
}

// HealingTxStatus represents the current status of a healing transaction
type HealingTxStatus int

const (
    // Status values for healing transactions
    TxStatusPending HealingTxStatus = iota    // Not yet submitted
    TxStatusSubmitted                         // Submitted but not confirmed
    TxStatusFailed                            // Failed with an error
    TxStatusConflictingRoutes                 // Failed with conflicting routes error
    TxStatusConfirmed                         // Confirmed at destination
)

// Maximum attempts for transaction submissions before rebuilding
const maxSubmissionAttempts = 10

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

// TransactionStats tracks statistics about healing transactions
type TransactionStats struct {
    TotalTransactions      int
    SuccessfulTransactions int
    FailedTransactions     int
    ConflictingRoutes      int
    AverageAttempts        float64
    MaxAttempts            int
}

// NewTransactionTracker creates a new transaction tracker
func NewTransactionTracker() *TransactionTracker {
    return &TransactionTracker{
        txMap: make(map[string][]*HealingTransaction),
        stats: TransactionStats{},
    }
}
```

### 2. Transaction Tracker Methods

Implement the core methods for the transaction tracker:

```go
// Add a new transaction to track
func (t *TransactionTracker) AddTransaction(tx *HealingTransaction) {
    t.mutex.Lock()
    defer t.mutex.Unlock()
    
    // Set initial values if not already set
    if tx.FirstAttemptTime.IsZero() {
        tx.FirstAttemptTime = time.Now()
    }
    if tx.Status == 0 {
        tx.Status = TxStatusPending
    }
    if tx.TxID == "" && len(tx.Envelope.Messages) > 0 {
        tx.TxID = tx.Envelope.Messages[0].ID().String()
    }
    
    // Create the partition pair key
    pairKey := fmt.Sprintf("%s:%s", tx.SourcePartition, tx.DestPartition)
    
    // Add to the appropriate queue and sort by sequence number
    t.txMap[pairKey] = append(t.txMap[pairKey], tx)
    sort.Slice(t.txMap[pairKey], func(i, j int) bool {
        return t.txMap[pairKey][i].SequenceNumber < t.txMap[pairKey][j].SequenceNumber
    })
    
    // Update stats
    t.stats.TotalTransactions++
    
    slog.Info("Added transaction to tracking", 
        "source", tx.SourcePartition, 
        "destination", tx.DestPartition, 
        "sequence", tx.SequenceNumber,
        "txid", tx.TxID)
}

// Get the next transaction to process for a partition pair
func (t *TransactionTracker) GetNextTransaction(sourcePartition, destPartition string) *HealingTransaction {
    t.mutex.RLock()
    defer t.mutex.RUnlock()
    
    pairKey := fmt.Sprintf("%s:%s", sourcePartition, destPartition)
    txs := t.txMap[pairKey]
    if len(txs) == 0 {
        return nil
    }
    
    // Return the first transaction (lowest sequence number)
    return txs[0]
}

// Find an existing transaction for a specific gap
func (t *TransactionTracker) FindTransactionForGap(sourcePartition, destPartition string, sequenceNumber uint64) *HealingTransaction {
    t.mutex.RLock()
    defer t.mutex.RUnlock()
    
    pairKey := fmt.Sprintf("%s:%s", sourcePartition, destPartition)
    txs := t.txMap[pairKey]
    
    // Look for a transaction with the matching sequence number
    for _, tx := range txs {
        if tx.SequenceNumber == sequenceNumber {
            return tx
        }
    }
    
    return nil
}

// Update a transaction's status after an attempt
func (t *TransactionTracker) UpdateTransactionStatus(tx *HealingTransaction, submitted bool, err error) {
    t.mutex.Lock()
    defer t.mutex.Unlock()
    
    // Update attempt information
    tx.AttemptCount++
    tx.LastAttemptTime = time.Now()
    tx.LastError = err
    
    // Update status based on result
    if submitted {
        tx.Status = TxStatusSubmitted
        slog.Info("Transaction submitted successfully",
            "source", tx.SourcePartition,
            "destination", tx.DestPartition,
            "sequence", tx.SequenceNumber,
            "attempts", tx.AttemptCount)
    } else if err != nil && strings.Contains(err.Error(), "conflicting routes") {
        tx.Status = TxStatusConflictingRoutes
        t.stats.ConflictingRoutes++
        slog.Error("Conflicting routes error",
            "source", tx.SourcePartition,
            "destination", tx.DestPartition,
            "sequence", tx.SequenceNumber,
            "attempts", tx.AttemptCount,
            "error", err)
    } else {
        tx.Status = TxStatusFailed
        slog.Error("Transaction submission failed",
            "source", tx.SourcePartition,
            "destination", tx.DestPartition,
            "sequence", tx.SequenceNumber,
            "attempts", tx.AttemptCount,
            "error", err)
    }
    
    // Check if we should remove the transaction
    pairKey := fmt.Sprintf("%s:%s", tx.SourcePartition, tx.DestPartition)
    txs := t.txMap[pairKey]
    
    // If submitted successfully or too many attempts, remove from queue
    if submitted || tx.AttemptCount >= maxSubmissionAttempts {
        // Find and remove this transaction
        for i, entry := range txs {
            if entry == tx {
                // Remove this entry
                t.txMap[pairKey] = append(txs[:i], txs[i+1:]...)
                
                // Update stats
                if submitted {
                    t.stats.SuccessfulTransactions++
                } else {
                    t.stats.FailedTransactions++
                }
                
                // Update average attempts
                totalAttempts := float64(t.stats.SuccessfulTransactions + t.stats.FailedTransactions)
                if totalAttempts > 0 {
                    t.stats.AverageAttempts = float64(t.stats.TotalTransactions) / totalAttempts
                }
                
                // Update max attempts
                if tx.AttemptCount > t.stats.MaxAttempts {
                    t.stats.MaxAttempts = tx.AttemptCount
                }
                
                // Log if giving up
                if !submitted && tx.AttemptCount >= maxSubmissionAttempts {
                    slog.Warn("Giving up on transaction after max attempts",
                        "source", tx.SourcePartition,
                        "destination", tx.DestPartition,
                        "sequence", tx.SequenceNumber,
                        "attempts", tx.AttemptCount)
                }
                
                break
            }
        }
    }
}

// Check if a transaction has been confirmed at the destination
func (t *TransactionTracker) CheckTransactionConfirmation(tx *HealingTransaction) bool {
    // Record that we checked
    tx.LastCheckedTime = time.Now()
    
    // TODO: Implement the actual check logic here
    // This would query the destination to see if the transaction is present
    // For now, we'll just return false
    return false
}

// Explicitly remove a transaction from tracking
func (t *TransactionTracker) RemoveTransaction(tx *HealingTransaction) {
    t.mutex.Lock()
    defer t.mutex.Unlock()
    
    pairKey := fmt.Sprintf("%s:%s", tx.SourcePartition, tx.DestPartition)
    txs := t.txMap[pairKey]
    
    // Find and remove this transaction
    for i, entry := range txs {
        if entry == tx {
            // Remove this entry
            t.txMap[pairKey] = append(txs[:i], txs[i+1:]...)
            
            // Update stats
            t.stats.FailedTransactions++
            
            slog.Info("Removed transaction from tracking",
                "source", tx.SourcePartition,
                "destination", tx.DestPartition,
                "sequence", tx.SequenceNumber,
                "attempts", tx.AttemptCount)
            
            break
        }
    }
}

// Get statistics about tracked transactions
func (t *TransactionTracker) GetStats() TransactionStats {
    t.mutex.RLock()
    defer t.mutex.RUnlock()
    return t.stats
}

// Log current statistics
func (t *TransactionTracker) LogStats() {
    stats := t.GetStats()
    slog.Info("Transaction tracking statistics",
        "total", stats.TotalTransactions,
        "successful", stats.SuccessfulTransactions,
        "failed", stats.FailedTransactions,
        "conflictingRoutes", stats.ConflictingRoutes,
        "avgAttempts", stats.AverageAttempts,
        "maxAttempts", stats.MaxAttempts)
}
```

### 3. Helper Function to Create Healing Transactions

```go
// Create a new healing transaction from an envelope
func CreateHealingTransaction(env *messaging.Envelope) *HealingTransaction {
    // Extract information from the envelope
    sourcePartition, destPartition, destUrl, sequenceNumber, isAnchor, isSynthetic := extractTransactionInfo(env)
    
    // Create the transaction object
    tx := &HealingTransaction{
        Envelope:        env,
        SourcePartition: sourcePartition,
        DestPartition:   destPartition,
        DestinationUrl:  destUrl,
        SequenceNumber:  sequenceNumber,
        Status:          TxStatusPending,
        IsAnchor:        isAnchor,
        IsSynthetic:     isSynthetic,
    }
    
    // Set the transaction ID if possible
    if len(env.Messages) > 0 {
        tx.TxID = env.Messages[0].ID().String()
    }
    
    return tx
}

// Extract transaction information from an envelope
func extractTransactionInfo(env *messaging.Envelope) (string, string, *url.URL, uint64, bool, bool) {
    // Default values
    var sourcePartition, destPartition string
    var destUrl *url.URL
    var sequenceNumber uint64
    isAnchor := false
    isSynthetic := false
    
    // Extract information based on message type
    if len(env.Messages) > 0 {
        msg := env.Messages[0]
        
        // Try to extract information based on message type
        switch m := msg.(type) {
        case *messaging.ChainMessage:
            // This might be an anchor message
            destUrl = m.Destination
            if destUrl != nil {
                // Try to determine partitions from the URL
                destPartition = extractPartitionFromUrl(destUrl)
            }
            
            // Check if this is an anchor message
            if _, ok := m.Message.(*protocol.AnchorLedger); ok {
                isAnchor = true
                // Extract more specific information for anchors
                // ...
            }
            
        case *messaging.TransactionMessage:
            // This might be a synthetic transaction
            if txn := m.Transaction; txn != nil {
                if body := txn.Body; body != nil {
                    // Try to extract information from the transaction body
                    sourcePartition, destPartition, destUrl, sequenceNumber = extractInfoFromTxnBody(body)
                    
                    // Check if this is a synthetic transaction
                    // ...
                    isSynthetic = true
                }
            }
        }
        
        // If we couldn't determine the partitions from the message directly,
        // try to use the router to get more information
        if sourcePartition == "" || destPartition == "" {
            // Use the router to get more information
            // ...
        }
    }
    
    return sourcePartition, destPartition, destUrl, sequenceNumber, isAnchor, isSynthetic
}

// Extract partition from URL
func extractPartitionFromUrl(u *url.URL) string {
    // Use the standardized approach from sequence.go
    // This will depend on your URL format
    // For example: acc://bvn-Apollo.acme -> "Apollo"
    
    // For now, a simple implementation
    if u == nil {
        return ""
    }
    
    // Handle different URL formats
    if strings.HasPrefix(u.Authority, "bvn-") {
        // Format: acc://bvn-Apollo.acme
        return strings.TrimPrefix(u.Authority, "bvn-")
    } else if u.Path != "" && strings.Contains(u.Path, "/anchors/") {
        // Format: acc://dn.acme/anchors/Apollo
        parts := strings.Split(u.Path, "/")
        if len(parts) > 0 {
            return parts[len(parts)-1]
        }
    }
    
    return ""
}
```

### 4. Modify the Healing Process to Use the Transaction Tracker

Update the `healAll` function to use the transaction tracker and implement the transaction reuse strategy:

```go
// Add the transaction tracker to the healer struct
type healer struct {
    // ... existing fields ...
    txTracker *TransactionTracker
}

// Initialize the tracker in the healer constructor
func newHealer() *healer {
    h := &healer{
        // ... existing initialization ...
        txTracker: NewTransactionTracker(),
    }
    return h
}

// Modify the healAll function to check for pending transactions
func (h *healer) healAll(args []string) {
    // Get the list of partitions from the protocol
    partitions, err := h.getPartitions()
    if err != nil {
        slog.Error("Failed to get partitions", "error", err)
        return
    }
    
    slog.Info("Starting healing cycle", "partitionCount", len(partitions))
    for _, p := range partitions {
        slog.Debug("Found partition", "id", p.ID, "type", p.Type)
    }
    
    // For each partition pair in the healing cycle
    for _, src := range partitions {
        for _, dst := range partitions {
            // Skip self-anchors for non-DN partitions
            if src.ID == dst.ID && !strings.EqualFold(src.ID, protocol.Directory) {
                continue
            }
            
            // First try to process any pending transactions for this pair
            if h.processPendingTransactions(src.ID, dst.ID) {
                // Skip to next pair if we processed a pending transaction
                continue
            }
            
            // Only proceed with normal healing if no pending transactions exist
            // Process anchor gaps first, then synthetic gaps
            h.processAnchorGaps(src.ID, dst.ID)
            h.processSyntheticGaps(src.ID, dst.ID)
        }
    }
    
    // Log transaction statistics periodically
    h.txTracker.LogStats()
}

// Note: The existing code already dynamically obtains partition information
// from the protocol using h.net.Status.Network.Partitions

// Process pending transactions for a partition pair
func (h *healer) processPendingTransactions(sourcePartition, destPartition string) bool {
    // Get the next transaction to process
    tx := h.txTracker.GetNextTransaction(sourcePartition, destPartition)
    if tx == nil {
        return false // No pending transactions
    }
    
    // If the transaction is already submitted, check if it's confirmed
    if tx.Status == TxStatusSubmitted {
        if h.txTracker.CheckTransactionConfirmation(tx) {
            // Transaction is confirmed, remove it from tracking
            tx.Status = TxStatusConfirmed
            h.txTracker.UpdateTransactionStatus(tx, true, nil)
            return true
        }
        
        // If it's been a while since we submitted, try again
        if time.Since(tx.LastAttemptTime) > 5*time.Minute {
            // Reset status to retry
            tx.Status = TxStatusPending
        } else {
            // Still waiting for confirmation, skip for now
            return true
        }
    }
    
    // Attempt to submit the transaction
    submitted := false
    var err error
    
    // Select a node and create a client
    // ... code to select a node and create a client ...
    
    // Try to submit the envelope
    slog.Info("Attempting to submit transaction",
        "source", tx.SourcePartition,
        "destination", tx.DestPartition,
        "sequence", tx.SequenceNumber,
        "attempt", tx.AttemptCount+1)
        
    // ... code to submit the envelope ...
    
    // Update the transaction status
    h.txTracker.UpdateTransactionStatus(tx, submitted, err)
    
    // Return true to indicate we processed a transaction
    return true
}

// Process anchor gaps between partitions
func (h *healer) processAnchorGaps(sourcePartition, destPartition string) {
    // Get the current anchor state
    srcState, dstState := h.getAnchorState(sourcePartition, destPartition)
    if srcState == nil || dstState == nil {
        return // Can't determine state, skip
    }
    
    // Find gaps in anchor sequence
    gaps := h.findAnchorGaps(srcState, dstState)
    
    // Process each gap
    for _, gap := range gaps {
        // First check if we already have a transaction for this gap
        existingTx := h.txTracker.FindTransactionForGap(sourcePartition, destPartition, gap.SequenceNumber)
        if existingTx != nil {
            // We already have a transaction for this gap, reuse it
            slog.Info("Reusing existing transaction for anchor gap",
                "source", sourcePartition,
                "destination", destPartition,
                "sequence", gap.SequenceNumber,
                "attempts", existingTx.AttemptCount)
                
            // If we've tried too many times with this transaction, rebuild it
            if existingTx.AttemptCount >= maxSubmissionAttempts {
                slog.Info("Rebuilding transaction after too many attempts",
                    "source", sourcePartition,
                    "destination", destPartition,
                    "sequence", gap.SequenceNumber,
                    "attempts", existingTx.AttemptCount)
                    
                // Remove the existing transaction
                h.txTracker.RemoveTransaction(existingTx)
                
                // Create a new one below
            } else {
                // Resubmit the existing transaction
                h.submitTransaction(existingTx)
                return // Processed one transaction, return to maintain sequential processing
            }
        }
        
        // Create a new transaction for this gap
        tx := h.createAnchorTransaction(sourcePartition, destPartition, gap)
        if tx != nil {
            // Add to tracker and submit
            h.txTracker.AddTransaction(tx)
            h.submitTransaction(tx)
            return // Processed one transaction, return to maintain sequential processing
        }
    }
}

// Process synthetic transaction gaps between partitions
func (h *healer) processSyntheticGaps(sourcePartition, destPartition string) {
    // Get the current synthetic transaction state
    srcState, dstState := h.getSyntheticState(sourcePartition, destPartition)
    if srcState == nil || dstState == nil {
        return // Can't determine state, skip
    }
    
    // Find gaps in synthetic transaction sequence
    gaps := h.findSyntheticGaps(srcState, dstState)
    
    // Process each gap
    for _, gap := range gaps {
        // First check if we already have a transaction for this gap
        existingTx := h.txTracker.FindTransactionForGap(sourcePartition, destPartition, gap.SequenceNumber)
        if existingTx != nil {
            // We already have a transaction for this gap, reuse it
            slog.Info("Reusing existing transaction for synthetic gap",
                "source", sourcePartition,
                "destination", destPartition,
                "sequence", gap.SequenceNumber,
                "attempts", existingTx.AttemptCount)
                
            // If we've tried too many times with this transaction, rebuild it
            if existingTx.AttemptCount >= maxSubmissionAttempts {
                slog.Info("Rebuilding transaction after too many attempts",
                    "source", sourcePartition,
                    "destination", destPartition,
                    "sequence", gap.SequenceNumber,
                    "attempts", existingTx.AttemptCount)
                    
                // Remove the existing transaction
                h.txTracker.RemoveTransaction(existingTx)
                
                // Create a new one below
            } else {
                // Resubmit the existing transaction
                h.submitTransaction(existingTx)
                return // Processed one transaction, return to maintain sequential processing
            }
        }
        
        // Create a new transaction for this gap
        tx := h.createSyntheticTransaction(sourcePartition, destPartition, gap)
        if tx != nil {
            // Add to tracker and submit
            h.txTracker.AddTransaction(tx)
            h.submitTransaction(tx)
            return // Processed one transaction, return to maintain sequential processing
        }
    }
}

// Submit a transaction with appropriate node selection
func (h *healer) submitTransaction(tx *HealingTransaction) {
    // Select a node and create a client
    // ... code to select a node and create a client ...
    
    // Try to submit the envelope
    slog.Info("Submitting transaction",
        "source", tx.SourcePartition,
        "destination", tx.DestPartition,
        "sequence", tx.SequenceNumber,
        "attempt", tx.AttemptCount+1,
        "txType", getTxTypeName(tx))
        
    submitted := false
    var err error
    
    // ... code to submit the envelope ...
    
    // Update the transaction status
    h.txTracker.UpdateTransactionStatus(tx, submitted, err)
}

// Helper to get transaction type name for logging
func getTxTypeName(tx *HealingTransaction) string {
    if tx.IsAnchor {
        return "anchor"
    } else if tx.IsSynthetic {
        return "synthetic"
    }
    return "unknown"
}
```

### 5. Modify the Submission Loop to Use the Transaction Tracker

Update the `stableNodeSubmitLoop` function to use the transaction tracker:

```go
// Modify the stableNodeSubmitLoop function
func (h *healer) stableNodeSubmitLoop(wg *sync.WaitGroup) {
    defer wg.Done()
    
    // ... existing initialization code ...
    
    // Main submission loop
    var messages []messaging.Message
    for {
        // ... existing message collection code ...
        
        // Create an envelope with the messages
        env := &messaging.Envelope{Messages: messages}
        
        // Generate a cache key based on the message IDs
        cacheKey := ""
        for _, msg := range env.Messages {
            cacheKey += msg.ID().String()
        }
        
        // Check if we've already successfully submitted this envelope
        if _, ok := submissionCache[cacheKey]; ok {
            slog.Info("Skipping cached submission", "id", env.Messages[0].ID())
            messages = messages[:0]
            continue
        }
        
        // Create a healing transaction
        tx := CreateHealingTransaction(env)
        
        // Try to determine the target partition for this message
        if tx.DestPartition == "" && h.router != nil {
            // Try to route the envelope to determine the target partition
            targetPartition, err := h.router.Route(env)
            if err != nil {
                slog.Error("Failed to route envelope", "error", err, "id", env.Messages[0].ID())
                // Continue anyway with the information we have
            } else {
                tx.DestPartition = targetPartition
            }
        }
        
        // Flag to track if the submission was successful
        var submitted bool
        var lastErr error
        
        // Try submission with primary node first
        // ... existing submission code ...
        
        // If primary node submission failed, try one alternative node
        // ... existing alternative node code ...
        
        // If submission failed with conflicting routes error, add to tracker
        if !submitted && lastErr != nil && strings.Contains(lastErr.Error(), "conflicting routes") {
            // Add to the transaction tracker
            h.txTracker.AddTransaction(tx)
        }
        
        // Clear messages if submitted successfully, otherwise keep for retry
        if submitted {
            messages = messages[:0]
            
            // Add to cache to avoid resubmitting
            submissionCache[cacheKey] = true
        } else {
            // Wait a bit before retrying
            time.Sleep(5 * time.Second)
        }
    }
}
```

### 6. Standardize URL Construction

Ensure all URL construction uses the standardized approach from sequence.go:

```go
// Use this approach consistently throughout the code
srcUrl := protocol.PartitionUrl(src.ID)  // e.g., acc://bvn-Apollo.acme

// Instead of
// srcUrl := protocol.DnUrl().JoinPath(protocol.AnchorPool).JoinPath(src.ID)
```

This will help address the URL construction differences that can lead to conflicting routes errors.

*Note: This implementation plan will be removed after the code is working correctly.*

## Partition Pairs and Transaction Types

The Accumulate network has specific patterns for anchoring and synthetic transactions between partitions. **Important**: The actual partition names are not hardcoded in the implementation but are dynamically obtained from the protocol at runtime. The examples below use mainnet partition names for illustration purposes only:

### Anchoring Partition Pairs (6 pairs)

Anchoring happens bidirectionally between the Directory Network (DN) and the BVNs. For mainnet, the BVNs are Apollo, Yutu, and Chandrayaan:
- DN → BVN-Apollo
- BVN-Apollo → DN
- DN → BVN-Yutu
- BVN-Yutu → DN
- DN → BVN-Chandrayaan
- BVN-Chandrayaan → DN

### Synthetic Transaction Partition Pairs (12 pairs)

Synthetic transactions sync between all partitions (DN and all BVNs):
- DN → BVN-Apollo
- DN → BVN-Yutu
- DN → BVN-Chandrayaan
- BVN-Apollo → DN
- BVN-Apollo → BVN-Yutu
- BVN-Apollo → BVN-Chandrayaan
- BVN-Yutu → DN
- BVN-Yutu → BVN-Apollo
- BVN-Yutu → BVN-Chandrayaan
- BVN-Chandrayaan → DN
- BVN-Chandrayaan → BVN-Apollo
- BVN-Chandrayaan → BVN-Yutu

This distinction is important for the healing process, as it determines which partition pairs need to be checked for each type of transaction.

## Healing Process

The main healing loop follows this pattern to implement the process-all-partition-pairs enhancement:

```go
// Main healing loop
for {
    // Initialize a new healing report for this cycle
    report := &healingReport{
        Cycle:            cycleCount,
        StartTime:        time.Now(),
        PairStatuses:     make(map[string]*partitionPairStatus),
        TotalAnchorHealed: totalAnchorHealed,
        TotalSynthHealed:  totalSynthHealed,
    }

    // Track total healing count for this cycle
    totalHealedThisCycle := 0

    // Pre-initialize the report with all possible partition pairs
    for _, src := range partitions {
        for _, dst := range partitions {
            // Skip self-anchors for non-DN partitions
            if src.ID == dst.ID && !strings.EqualFold(src.ID, protocol.Directory) {
                continue
            }
            
            // Create the partition pair status
            pairKey := fmt.Sprintf("%s:%s", src.ID, dst.ID)
            report.PairStatuses[pairKey] = &partitionPairStatus{
                Source:      src.ID,
                Destination: dst.ID,
            }
        }
    }

    // Process each partition pair
    for _, src := range partitions {
        for _, dst := range partitions {
            // Process anchor gaps first
            // Then process synthetic gaps
            // Update totalHealedThisCycle as needed
        }
    }

    // Pause based on healing activity
    if totalHealedThisCycle > 0 {
        // Longer pause after healing
    } else {
        // Shorter pause when no healing needed
    }
}
```

## Reporting System Implementation

This section covers the code implementation of the reporting system. For the complete design specification, see the [Reporting Design](./heal_all_reporting.md) document.

```go
// Display the healing report
func displayHealingReport(report *healingReport) {
    // Print header
    fmt.Println(strings.Repeat("=", 80))
    fmt.Printf("HEALING REPORT - Cycle %d - Duration: %s\n", report.Cycle, report.Duration.Round(time.Millisecond))
    fmt.Printf("Total Healed: %d (Anchors: %d, Synth: %d)\n", 
        report.TotalHealed, report.TotalAnchorHealed, report.TotalSynthHealed)
    fmt.Println(strings.Repeat("=", 80))
    
    // Create a new table
    table := tablewriter.NewWriter(os.Stdout)
    table.SetHeader([]string{
        "Source", 
        "Destination", 
        "Anchor Gaps", 
        "Anchor Healed", 
        "Synth Gaps", 
        "Synth Healed", 
        "Status",
    })
    
    // Set table formatting
    table.SetBorder(false)
    table.SetColumnSeparator(" | ")
    table.SetCenterSeparator("+")
    table.SetHeaderAlignment(tablewriter.ALIGN_CENTER)
    table.SetAlignment(tablewriter.ALIGN_CENTER)
    
    // Add rows with color coding
    // Render the table
}
```

## TableWriter Usage Guidelines

All reporting in Version 2 should use the `github.com/olekukonko/tablewriter` package for consistent formatting:

### Table Configuration

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
```

### Color Coding

```go
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

### Footer Configuration

```go
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

## Compatibility Notes

The enhanced heal-all command maintains compatibility with the existing healing approaches described in the heal_v1 documentation:

1. **Shared Components**: Uses the same `sequenceInfo` structure with the same fields (Source, Destination, Number, ID).

2. **URL Construction**: Follows the standardized URL construction method recommended in the documentation.

3. **Healing Approaches**: Maintains separate processing paths for anchor and synthetic transactions.

4. **Implementation Guidelines**: Adheres to the implementation guidelines for URL construction, database interaction, and version handling.

## Testing Recommendations

1. **Unit Tests**: Create unit tests for the reporting functionality to ensure proper formatting and content.

2. **Visual Verification**: Include a test function that prints a sample report for visual verification.

3. **Integration Tests**: Test the heal-all command with different network configurations to ensure it correctly processes all partition pairs.

4. **Performance Testing**: Verify that the controlled pacing mechanism prevents DOS issues while ensuring efficient healing.
