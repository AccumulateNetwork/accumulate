# AI-METADATA
document_type: implementation_detail
project: accumulate_network
component: caching
version: v2
related_files:
  - ../../../concepts/caching-strategies.md
  - ./transaction-healing-process.md
  - ./url-diagnostics.md
```

## Overview

The V2 implementation introduces an enhanced caching system to reduce redundant network requests and improve the performance and reliability of the healing process. This document outlines the design and implementation of the caching system in the V2 implementation.

## Caching System Design

The caching system in V2 consists of three main components:

1. **Query Cache**: Caches the results of API queries to reduce redundant network requests
2. **Node Performance Cache**: Tracks the performance and reliability of nodes to improve node selection
3. **Transaction Tracker**: Tracks healing transactions across cycles to avoid redundant transaction creation

### Query Cache

The query cache stores the results of API queries to avoid redundant network requests for the same data. This is particularly important for account queries and chain queries, which are frequently used during the healing process.

```go
// Structure for the query cache
type QueryCache struct {
    cache map[string]interface{}
    mutex sync.RWMutex
}

// Create a new query cache
func NewQueryCache() *QueryCache {
    return &QueryCache{
        cache: make(map[string]interface{}),
    }
}

// Get a value from the cache
func (c *QueryCache) Get(key string) (interface{}, bool) {
    c.mutex.RLock()
    defer c.mutex.RUnlock()
    value, ok := c.cache[key]
    return value, ok
}

// Set a value in the cache
func (c *QueryCache) Set(key string, value interface{}) {
    c.mutex.Lock()
    defer c.mutex.Unlock()
    c.cache[key] = value
}
```

#### Cache Key Construction

The cache key is constructed from the query type, URL, and parameters to ensure uniqueness:

```go
// Construct a cache key for a query
func constructCacheKey(queryType string, url *url.URL, params map[string]interface{}) string {
    // Start with the query type
    key := queryType + ":"
    
    // Add the URL
    key += url.String() + ":"
    
    // Add the parameters in a deterministic order
    var paramKeys []string
    for k := range params {
        paramKeys = append(paramKeys, k)
    }
    sort.Strings(paramKeys)
    
    for _, k := range paramKeys {
        key += k + "=" + fmt.Sprintf("%v", params[k]) + ","
    }
    
    return key
}
```

#### Usage Example

```go
// Example of using the query cache
func queryAccountWithCache(ctx context.Context, client api.Client, url *url.URL, cache *QueryCache) (*api.AccountQueryResponse, error) {
    // Construct the cache key
    key := constructCacheKey("account", url, nil)
    
    // Check if the result is in the cache
    if value, ok := cache.Get(key); ok {
        if resp, ok := value.(*api.AccountQueryResponse); ok {
            return resp, nil
        }
    }
    
    // Execute the query
    resp, err := client.QueryAccount(ctx, url, &api.AccountQuery{
        IncludeRecent: true,
    })
    if err != nil {
        return nil, err
    }
    
    // Cache the result
    cache.Set(key, resp)
    
    return resp, nil
}
```

### Node Performance Cache

The node performance cache tracks the performance and reliability of nodes to improve node selection for transaction submission. This helps avoid problematic nodes and prioritize more reliable nodes.

```go
// Structure for the node performance cache
type NodePerformanceCache struct {
    nodes map[string]*NodePerformance
    mutex sync.RWMutex
}

// Structure for node performance metrics
type NodePerformance struct {
    SuccessCount   int
    FailureCount   int
    ResponseTimes  []time.Duration
    LastAccessTime time.Time
}

// Create a new node performance cache
func NewNodePerformanceCache() *NodePerformanceCache {
    return &NodePerformanceCache{
        nodes: make(map[string]*NodePerformance),
    }
}

// Record a successful operation for a node
func (c *NodePerformanceCache) RecordSuccess(nodeID string, responseTime time.Duration) {
    c.mutex.Lock()
    defer c.mutex.Unlock()
    
    node, ok := c.nodes[nodeID]
    if !ok {
        node = &NodePerformance{}
        c.nodes[nodeID] = node
    }
    
    node.SuccessCount++
    node.ResponseTimes = append(node.ResponseTimes, responseTime)
    node.LastAccessTime = time.Now()
}

// Record a failed operation for a node
func (c *NodePerformanceCache) RecordFailure(nodeID string) {
    c.mutex.Lock()
    defer c.mutex.Unlock()
    
    node, ok := c.nodes[nodeID]
    if !ok {
        node = &NodePerformance{}
        c.nodes[nodeID] = node
    }
    
    node.FailureCount++
    node.LastAccessTime = time.Now()
}

// Get the success rate for a node
func (c *NodePerformanceCache) GetSuccessRate(nodeID string) float64 {
    c.mutex.RLock()
    defer c.mutex.RUnlock()
    
    node, ok := c.nodes[nodeID]
    if !ok {
        return 0.0
    }
    
    total := node.SuccessCount + node.FailureCount
    if total == 0 {
        return 0.0
    }
    
    return float64(node.SuccessCount) / float64(total)
}

// Get the average response time for a node
func (c *NodePerformanceCache) GetAverageResponseTime(nodeID string) time.Duration {
    c.mutex.RLock()
    defer c.mutex.RUnlock()
    
    node, ok := c.nodes[nodeID]
    if !ok || len(node.ResponseTimes) == 0 {
        return 0
    }
    
    var total time.Duration
    for _, t := range node.ResponseTimes {
        total += t
    }
    
    return total / time.Duration(len(node.ResponseTimes))
}
```

#### Usage Example

```go
// Example of using the node performance cache for node selection
func selectBestNode(nodes []string, cache *NodePerformanceCache) string {
    var bestNode string
    var bestScore float64
    
    for _, nodeID := range nodes {
        // Calculate a score based on success rate and response time
        successRate := cache.GetSuccessRate(nodeID)
        responseTime := cache.GetAverageResponseTime(nodeID)
        
        // Higher success rate and lower response time is better
        score := successRate
        if responseTime > 0 {
            score = score / float64(responseTime/time.Millisecond)
        }
        
        if bestNode == "" || score > bestScore {
            bestNode = nodeID
            bestScore = score
        }
    }
    
    return bestNode
}
```

### Transaction Tracker

The transaction tracker keeps track of healing transactions across cycles to avoid redundant transaction creation and submission. This is particularly important for transactions that require multiple submission attempts.

```go
// Structure for the transaction tracker
type TransactionTracker struct {
    transactions map[string]*HealingTransaction
    mutex        sync.RWMutex
}

// Structure for a healing transaction
type HealingTransaction struct {
    ID              string
    SourcePartition string
    DestPartition   string
    SequenceNumber  uint64
    Transaction     *protocol.Transaction
    SubmissionCount int
    LastAttemptTime time.Time
    Status          TxStatus
}

// Transaction status
type TxStatus int

const (
    TxStatusPending TxStatus = iota
    TxStatusSubmitted
    TxStatusConfirmed
    TxStatusFailed
)

// Create a new transaction tracker
func NewTransactionTracker() *TransactionTracker {
    return &TransactionTracker{
        transactions: make(map[string]*HealingTransaction),
    }
}

// Add a transaction to the tracker
func (t *TransactionTracker) AddTransaction(tx *HealingTransaction) {
    t.mutex.Lock()
    defer t.mutex.Unlock()
    t.transactions[tx.ID] = tx
}

// Find a transaction for a gap
func (t *TransactionTracker) FindTransactionForGap(srcPartition, dstPartition string, sequenceNumber uint64) *HealingTransaction {
    t.mutex.RLock()
    defer t.mutex.RUnlock()
    
    for _, tx := range t.transactions {
        if tx.SourcePartition == srcPartition &&
           tx.DestPartition == dstPartition &&
           tx.SequenceNumber == sequenceNumber {
            return tx
        }
    }
    
    return nil
}

// Update the status of a transaction
func (t *TransactionTracker) UpdateTransactionStatus(tx *HealingTransaction, status TxStatus) {
    t.mutex.Lock()
    defer t.mutex.Unlock()
    
    tx.Status = status
    t.transactions[tx.ID] = tx
}

// Remove a transaction from the tracker
func (t *TransactionTracker) RemoveTransaction(txID string) {
    t.mutex.Lock()
    defer t.mutex.Unlock()
    delete(t.transactions, txID)
}

// Clean up stale transactions
func (t *TransactionTracker) CleanupStaleTransactions(maxAge time.Duration) {
    t.mutex.Lock()
    defer t.mutex.Unlock()
    
    now := time.Now()
    for id, tx := range t.transactions {
        if now.Sub(tx.LastAttemptTime) > maxAge {
            delete(t.transactions, id)
        }
    }
}
```

#### Usage Example

```go
// Example of using the transaction tracker in the healing process
func processGap(ctx context.Context, h *Healer, src, dst *Partition, gapNum uint64) (bool, error) {
    // Check if we already have a transaction for this gap
    existingTx := h.txTracker.FindTransactionForGap(src.ID, dst.ID, gapNum)
    if existingTx != nil {
        // If we've already tried this transaction too many times, create a synthetic alternative
        if existingTx.SubmissionCount >= maxSubmissionAttempts {
            return h.createSyntheticAlternative(ctx, src, dst, gapNum)
        }
        
        // Otherwise, increment the submission count and try again
        existingTx.SubmissionCount++
        existingTx.LastAttemptTime = time.Now()
        h.submit <- existingTx
        h.txTracker.UpdateTransactionStatus(existingTx, TxStatusPending)
        return true, nil
    }
    
    // If we don't have a transaction for this gap, create one
    tx, err := h.createHealingTransaction(ctx, src, dst, gapNum)
    if err != nil {
        return false, err
    }
    
    // Add the transaction to the tracker
    h.txTracker.AddTransaction(tx)
    
    // Submit the transaction
    h.submit <- tx
    
    return true, nil
}
```

## Problematic Node Tracking

The V2 implementation also includes a mechanism for tracking problematic nodes to avoid sending requests to nodes that are known to be unreliable or unresponsive.

```go
// Structure for tracking problematic nodes
type ProblematicNodeTracker struct {
    problematicNodes map[string]map[string]time.Time // node -> query type -> last failure time
    cooldownPeriod   time.Duration
    mutex            sync.RWMutex
}

// Create a new problematic node tracker
func NewProblematicNodeTracker(cooldownPeriod time.Duration) *ProblematicNodeTracker {
    return &ProblematicNodeTracker{
        problematicNodes: make(map[string]map[string]time.Time),
        cooldownPeriod:   cooldownPeriod,
    }
}

// Mark a node as problematic for a specific query type
func (t *ProblematicNodeTracker) MarkNodeAsProblematic(node, queryType string) {
    t.mutex.Lock()
    defer t.mutex.Unlock()
    
    if _, ok := t.problematicNodes[node]; !ok {
        t.problematicNodes[node] = make(map[string]time.Time)
    }
    
    t.problematicNodes[node][queryType] = time.Now()
}

// Check if a node is problematic for a specific query type
func (t *ProblematicNodeTracker) IsNodeProblematic(node, queryType string) bool {
    t.mutex.RLock()
    defer t.mutex.RUnlock()
    
    if nodeMap, ok := t.problematicNodes[node]; ok {
        if lastFailure, ok := nodeMap[queryType]; ok {
            // Check if the cooldown period has elapsed
            return time.Since(lastFailure) < t.cooldownPeriod
        }
    }
    
    return false
}

// Get a list of healthy nodes for a specific query type
func (t *ProblematicNodeTracker) GetHealthyNodes(nodes []string, queryType string) []string {
    var healthyNodes []string
    
    for _, node := range nodes {
        if !t.IsNodeProblematic(node, queryType) {
            healthyNodes = append(healthyNodes, node)
        }
    }
    
    return healthyNodes
}
```

## Integration with the Healing Process

The caching system is integrated with the healing process to improve performance and reliability:

1. **Query Caching**: All API queries use the query cache to avoid redundant network requests
2. **Node Selection**: The node performance cache is used to select the best node for transaction submission
3. **Transaction Tracking**: The transaction tracker is used to avoid redundant transaction creation and submission
4. **Problematic Node Avoidance**: The problematic node tracker is used to avoid sending requests to unreliable nodes

```go
// Example of integrating the caching system with the healing process
func (h *Healer) healCycle(ctx context.Context) error {
    // Get the network status
    netInfo, err := h.getNetworkInfo(ctx)
    if err != nil {
        return err
    }
    
    // Iterate through all partition pairs
    for srcID, srcPeers := range netInfo.Peers {
        for dstID := range netInfo.Peers {
            if srcID == dstID {
                continue
            }
            
            // Process anchor gaps
            processed, err := h.processAnchorGaps(ctx, srcID, dstID)
            if err != nil {
                h.logger.Error("Failed to process anchor gaps", "src", srcID, "dst", dstID, "error", err)
            }
            
            // If we processed an anchor gap, move on to the next partition pair
            if processed {
                continue
            }
            
            // Process synthetic transaction gaps
            processed, err = h.processSyntheticGaps(ctx, srcID, dstID)
            if err != nil {
                h.logger.Error("Failed to process synthetic gaps", "src", srcID, "dst", dstID, "error", err)
            }
        }
    }
    
    // Clean up stale transactions
    h.txTracker.CleanupStaleTransactions(24 * time.Hour)
    
    return nil
}
```

## Performance Improvements

The enhanced caching system in V2 provides several performance improvements:

1. **Reduced Network Requests**: The query cache reduces redundant network requests, improving performance and reducing load on the network
2. **Improved Node Selection**: The node performance cache helps select more reliable nodes, reducing the likelihood of transaction submission failures
3. **Reduced Transaction Creation**: The transaction tracker reduces redundant transaction creation, improving performance and reducing load on the network
4. **Avoided Problematic Nodes**: The problematic node tracker helps avoid sending requests to unreliable nodes, improving reliability

## Memory Management

The V2 implementation includes several mechanisms for managing memory usage:

1. **Cache Size Limits**: The query cache has a maximum size to prevent unbounded memory growth
2. **Transaction Cleanup**: The transaction tracker periodically cleans up stale transactions to prevent memory leaks
3. **Response Time Limiting**: The node performance cache limits the number of response times stored for each node
4. **Problematic Node Expiration**: The problematic node tracker has a cooldown period after which nodes are no longer considered problematic

## See Also

- [Transaction Healing Process](./transaction-healing-process.md): Comprehensive design documentation for V2
- [URL Diagnostics](./url-diagnostics.md): Enhanced URL diagnostics in V2
- [Caching Strategies](../../concepts/caching-strategies.md): Comparison of different caching approaches
