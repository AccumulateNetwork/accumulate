---
title: Accumulate Routing Architecture
description: Detailed explanation of Accumulate's routing system for transactions and messages
tags: [accumulate, routing, url, partition, transaction-routing]
created: 2025-05-16
version: 1.0
---

# Accumulate Routing Architecture

## Introduction

Accumulate's routing architecture is a fundamental component that determines how transactions and messages are directed to the appropriate partition within the network. This document explains the implementation details of Accumulate's routing system, including URL-based routing, partition determination, and the underlying mechanisms.

## URL-Based Routing

Accumulate uses a URL-based addressing scheme that forms the foundation of its routing system.

### URL Structure

```
acc://[authority]/[path]
```

- **Authority**: Typically represents an ADI (Accumulate Digital Identifier) or token issuer
- **Path**: Identifies a specific account or resource within the authority

### Routing Implementation

The core routing implementation is found in the `internal/api/routing` package:

```go
// Router determines which partition an account belongs to
type Router interface {
    // RouteAccount determines which partition an account belongs to
    RouteAccount(account *url.URL) (string, error)
}
```

The router uses a deterministic algorithm to map URLs to specific partitions:

```go
// RouteAccount determines which partition an account belongs to
func (r *router) RouteAccount(account *url.URL) (string, error) {
    // Directory accounts go to the directory partition
    if protocol.IsProtocolAccount(account) {
        return "directory", nil
    }

    // Get the authority
    authority := account.Authority()
    if authority == "" {
        return "", errors.BadRequest.WithFormat("missing authority")
    }

    // Hash the authority to determine the partition
    hash := sha256.Sum256([]byte(strings.ToLower(authority)))
    partition := r.partitionFor(hash[:])
    
    return partition, nil
}
```

## Partition Determination

Accumulate uses a consistent hashing mechanism to determine which partition should handle a particular URL.

### Partition Mapping

The mapping from URL to partition is deterministic and based on the hash of the URL's authority component:

```go
// partitionFor determines which partition a hash belongs to
func (r *router) partitionFor(hash []byte) string {
    // Convert the first 4 bytes of the hash to an integer
    h := binary.BigEndian.Uint32(hash[:4])
    
    // Modulo by the number of partitions to get the partition index
    index := h % uint32(len(r.partitions))
    
    // Return the partition name
    return r.partitions[index]
}
```

### Partition Configuration

Partitions are configured at network initialization and stored in the network's configuration:

```go
type NetworkConfig struct {
    // Network name
    Name string
    
    // Partitions
    Partitions []string
    
    // Other configuration...
}
```

## Transaction Routing

When a transaction is submitted to the network, it goes through several routing steps:

### 1. Initial Submission

Transactions can be submitted to any node in the network. The receiving node is responsible for routing it to the correct partition:

```go
// Submit submits a transaction to the network
func (api *API) Submit(ctx context.Context, envelope *messaging.Envelope) ([]*api.SubmissionResponse, error) {
    // Normalize and validate the envelope
    txns, err := envelope.Normalize()
    if err != nil {
        return nil, err
    }
    
    // Group transactions by partition
    partitioned := make(map[string][]*messaging.Transaction)
    for _, txn := range txns {
        // Determine the partition for the transaction
        partition, err := api.router.RouteAccount(txn.Header.Principal)
        if err != nil {
            return nil, err
        }
        
        // Add to the appropriate partition group
        partitioned[partition] = append(partitioned[partition], txn)
    }
    
    // Process each partition group
    responses := make([]*api.SubmissionResponse, 0, len(txns))
    for partition, txns := range partitioned {
        // Create a new envelope for this partition
        partEnv := &messaging.Envelope{
            Signatures: envelope.Signatures,
        }
        partEnv.Messages = make([]messaging.Message, len(txns))
        for i, txn := range txns {
            partEnv.Messages[i] = txn
        }
        
        // Submit to the partition
        resp, err := api.submitToPartition(ctx, partition, partEnv)
        if err != nil {
            return nil, err
        }
        
        responses = append(responses, resp...)
    }
    
    return responses, nil
}
```

### 2. Dispatcher Routing

The dispatcher is responsible for sending transactions to the appropriate partition:

```go
// Submit queues a message to be sent to the specified URL
func (d *dispatcher) Submit(ctx context.Context, u *url.URL, env *messaging.Envelope) error {
    // Route the account
    partition, err := d.router.RouteAccount(u)
    if err != nil {
        return err
    }
    
    // Queue the envelope
    partition = strings.ToLower(partition)
    d.queue[partition] = append(d.queue[partition], env.Copy())
    return nil
}
```

### 3. Cross-Partition Routing

For transactions that affect multiple partitions, synthetic transactions are generated and routed to the appropriate partitions:

```go
// ProcessTransaction processes a transaction and generates synthetic transactions as needed
func (x *Executor) ProcessTransaction(ctx context.Context, tx *protocol.Transaction) (*protocol.TransactionStatus, error) {
    // Process the transaction
    status, err := x.executor.ProcessTransaction(ctx, tx)
    if err != nil {
        return nil, err
    }
    
    // Generate synthetic transactions
    for _, synth := range status.SyntheticTransactions {
        // Determine the destination partition
        partition, err := x.router.RouteAccount(synth.Transaction.Header.Principal)
        if err != nil {
            return nil, err
        }
        
        // Submit the synthetic transaction to the destination partition
        err = x.dispatcher.Submit(ctx, synth.Transaction.Header.Principal, &messaging.Envelope{
            Messages: []messaging.Message{synth.Transaction},
        })
        if err != nil {
            return nil, err
        }
    }
    
    return status, nil
}
```

## URL Resolution

Accumulate's routing system also handles URL resolution, which is the process of finding the actual account data for a given URL.

### Resolution Process

```go
// Resolve resolves a URL to an account
func (api *API) Resolve(ctx context.Context, u *url.URL) (*protocol.Account, error) {
    // Determine the partition
    partition, err := api.router.RouteAccount(u)
    if err != nil {
        return nil, err
    }
    
    // Get the partition client
    client, err := api.getPartitionClient(ctx, partition)
    if err != nil {
        return nil, err
    }
    
    // Query the partition
    return client.Query().Account(ctx, u)
}
```

### Directory Lookup

For certain types of URLs, a directory lookup is required:

```go
// ResolveWithDirectory resolves a URL with directory lookup if needed
func (api *API) ResolveWithDirectory(ctx context.Context, u *url.URL) (*protocol.Account, error) {
    // Try direct resolution first
    account, err := api.Resolve(ctx, u)
    if err == nil {
        return account, nil
    }
    
    // If not found and this is a token account URL, try directory lookup
    if protocol.IsTokenAccount(u) {
        // Look up the token issuer in the directory
        issuer, err := api.LookupTokenIssuer(ctx, u.Authority())
        if err != nil {
            return nil, err
        }
        
        // Construct the actual token account URL
        tokenUrl := protocol.FormatTokenAccount(issuer, u.Path)
        
        // Resolve the token account
        return api.Resolve(ctx, tokenUrl)
    }
    
    // Not found
    return nil, err
}
```

## Performance Considerations

Accumulate's routing architecture is designed for performance and scalability:

### 1. Deterministic Routing

The deterministic nature of URL-to-partition mapping ensures consistent routing without requiring global state lookups.

### 2. Caching

Frequently accessed routing information is cached to reduce computational overhead:

```go
// cachedRouter implements Router with caching
type cachedRouter struct {
    router  Router
    cache   map[string]string
    mutex   sync.RWMutex
}

// RouteAccount determines which partition an account belongs to, with caching
func (r *cachedRouter) RouteAccount(account *url.URL) (string, error) {
    key := account.String()
    
    // Check cache first
    r.mutex.RLock()
    partition, ok := r.cache[key]
    r.mutex.RUnlock()
    if ok {
        return partition, nil
    }
    
    // Not in cache, compute the route
    partition, err := r.router.RouteAccount(account)
    if err != nil {
        return "", err
    }
    
    // Update cache
    r.mutex.Lock()
    r.cache[key] = partition
    r.mutex.Unlock()
    
    return partition, nil
}
```

### 3. Parallel Processing

Transactions for different partitions can be processed in parallel:

```go
// SubmitParallel submits transactions to multiple partitions in parallel
func (api *API) SubmitParallel(ctx context.Context, partitioned map[string]*messaging.Envelope) ([]*api.SubmissionResponse, error) {
    var wg sync.WaitGroup
    responses := make(chan []*api.SubmissionResponse, len(partitioned))
    errors := make(chan error, len(partitioned))
    
    // Submit to each partition in parallel
    for partition, env := range partitioned {
        wg.Add(1)
        go func(partition string, env *messaging.Envelope) {
            defer wg.Done()
            
            resp, err := api.submitToPartition(ctx, partition, env)
            if err != nil {
                errors <- err
                return
            }
            
            responses <- resp
        }(partition, env)
    }
    
    // Wait for all submissions to complete
    wg.Wait()
    close(responses)
    close(errors)
    
    // Check for errors
    if len(errors) > 0 {
        return nil, <-errors
    }
    
    // Collect responses
    var allResponses []*api.SubmissionResponse
    for resp := range responses {
        allResponses = append(allResponses, resp...)
    }
    
    return allResponses, nil
}
```

## Routing Edge Cases

Accumulate's routing system handles several edge cases:

### 1. Unknown URLs

When a URL cannot be routed to a specific partition, the system falls back to the directory partition:

```go
// RouteUnknownAccount routes an unknown account to the directory
func (r *router) RouteUnknownAccount(account *url.URL) (string, error) {
    // Try normal routing first
    partition, err := r.RouteAccount(account)
    if err == nil {
        return partition, nil
    }
    
    // Fall back to directory
    return "directory", nil
}
```

### 2. Synthetic Transactions

Synthetic transactions may need special routing logic:

```go
// RouteSyntheticTransaction routes a synthetic transaction
func (r *router) RouteSyntheticTransaction(tx *protocol.Transaction) (string, error) {
    // Check if this is an anchor transaction
    if tx.Body.Type() == protocol.TransactionTypeAnchor {
        // Anchor transactions go to the directory
        return "directory", nil
    }
    
    // Otherwise, use normal routing
    return r.RouteAccount(tx.Header.Principal)
}
```

### 3. Multi-Partition Transactions

Some transactions affect multiple partitions and require special handling:

```go
// RouteMultiPartitionTransaction routes a transaction that affects multiple partitions
func (api *API) RouteMultiPartitionTransaction(ctx context.Context, tx *protocol.Transaction) (map[string]*messaging.Envelope, error) {
    // Analyze the transaction to determine affected partitions
    partitions, err := api.analyzeAffectedPartitions(ctx, tx)
    if err != nil {
        return nil, err
    }
    
    // Create envelopes for each partition
    result := make(map[string]*messaging.Envelope)
    for partition, txns := range partitions {
        env := &messaging.Envelope{
            Messages: make([]messaging.Message, len(txns)),
        }
        for i, txn := range txns {
            env.Messages[i] = txn
        }
        result[partition] = env
    }
    
    return result, nil
}
```

## Conclusion

Accumulate's routing architecture is a sophisticated system that efficiently directs transactions and messages to the appropriate partitions within the network. The URL-based addressing scheme, combined with deterministic partition mapping, enables scalable and performant routing across the distributed system.

The routing system is a critical component of Accumulate's multi-chain architecture, enabling seamless communication between different partitions while maintaining the independence and scalability of each partition.

## References

1. [Accumulate Multi-Chain Structure](/tendermint/01_multi_chain_structure.md)
2. [Cross-Network Communication](02_cross_network_communication.md)
3. [Transaction Processing](03_transaction_processing.md)
