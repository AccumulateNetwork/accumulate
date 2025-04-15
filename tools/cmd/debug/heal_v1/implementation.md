# Implementation Guidelines for Healing

This document provides practical guidelines and best practices for implementing and modifying the healing code in the Accumulate network.

<!-- ai:context-priority
This document is critical for understanding implementation patterns and API usage in healing.
Key files to load alongside this document: healing/heal_anchor.go, healing/heal_synth.go, protocol/url.go
-->

## Shared Components {#shared-components}

All healing approaches share several core components that should be maintained consistently:

### 1. SequencedInfo Structure {#sequenced-info}

```go
// In healing/sequenced.go
type SequencedInfo struct {
    Source      string    // Source partition ID
    Destination string    // Destination partition ID
    Number      uint64    // Sequence number
    ID          *url.TxID // Transaction ID (optional)
}
```

This structure is used to identify transactions across all healing approaches. When modifying code that uses this structure, ensure:
- The fields are used consistently
- The Source and Destination fields use the same partition ID format
- The Number field refers to the same sequence number concept

**Related Documentation**:
- [heal_synth Transaction Creation](./transactions.md#heal_synth-transaction-creation)
- [heal_anchor Transaction Creation](./transactions.md#heal_anchor-transaction-creation)

### 2. NetworkInfo Structure {#network-info}

```go
// In healing/scan.go
type NetworkInfo struct {
    Status api.NetworkStatus
    Peers  map[string]map[peer.ID]PeerInfo
}
```

This structure provides information about the network topology and is used by all healing approaches. When modifying:
- Maintain the map structure for efficient peer lookup
- Preserve the case-insensitive lookup for partition IDs
- Ensure PeerInfo contains all necessary node information

**Related Documentation**:
- [Network Tracking](#network-tracking)
- [Light Client as Infrastructure](./overview.md#light-client)

### 3. Submission Process {#submission-process}

All healing approaches use a common channel-based submission process:

```go
// In heal_common.go
func (h *healer) processSubmissions() {
    for msgs := range h.submit {
        // Process and submit messages
    }
}
```

When modifying the submission process:
- Maintain the buffered channel approach
- Preserve error handling and retry logic
- Ensure proper context cancellation

**Related Documentation**:
- [Transaction Creation](./transactions.md#transaction-creation-overview)
- [Error Handling](#error-handling)

## Bright Line Guidelines {#bright-line-guidelines}

When implementing or modifying healing code, follow these guidelines:

### 1. URL Construction {#url-construction}

<!-- ai:pattern-library
This section documents the URL construction patterns that should be used in healing code.
-->

There is a fundamental difference in how URLs are constructed between different parts of the codebase:
- `sequence.go` uses raw partition URLs (e.g., `acc://bvn-Apollo.acme`)
- `heal_anchor.go` appends the partition ID to the anchor pool URL (e.g., `acc://dn.acme/anchors/Apollo`)

#### URL Construction Code Patterns

```go
// Method 1: Raw partition URLs (preferred in Version 2)
// Used in sequence.go
srcUrl := protocol.PartitionUrl(src.ID)  // e.g., acc://bvn-Apollo.acme

// Method 2: Anchor pool URLs
// Used in heal_anchor.go
srcUrl := protocol.DnUrl().JoinPath(protocol.AnchorPool).JoinPath(src.ID)  // e.g., acc://dn.acme/anchors/Apollo
```

#### Implementation Guidelines

When modifying URL construction:
- Be aware of which format is expected by the code you're modifying
- Do not mix URL construction methods within a single component
- Consider standardizing on the `sequence.go` approach (Method 1) for new code
- Document the URL format used by your component

#### Common URL Construction Errors

| Error | Cause | Solution |
|-------|-------|----------|
| "Element does not exist" | URL construction mismatch | Use consistent URL construction method |
| URL parsing error | Malformed URL string | Use protocol package methods instead of string concatenation |
| Case sensitivity issues | Inconsistent case in partition IDs | Normalize case or use case-insensitive comparisons |

**Related Documentation**:
- [URL Construction Patterns](./chains_urls.md#url-construction-patterns)
- [URL Troubleshooting Guide](./chains_urls.md#url-troubleshooting-guide)

### 2. Database Interaction {#database-interaction}

When interacting with the database:
- Use transactions appropriately (read-only vs. read-write)
- Always commit or discard transactions
- Use proper error handling for database operations
- Do not assume the database is always available (especially for `heal_anchor`)

**Related Documentation**:
- [Database Requirements](./database.md#database-interface-requirements)
- [Database Transaction Management](#database-transaction-management)
- [Light Client Database Schema](./light_client.md#light-client-database-schema)

### 3. Version Handling {#version-handling}

The healing code handles different network versions:
- Vandenberg v2
- Baikonur v2
- Older network versions

When modifying version-specific code:
- Maintain separate code paths for different versions
- Use the `ExecutorVersion` to determine the appropriate code path
- Test with all supported network versions
- Document version-specific behavior

**Related Documentation**:
- [Version-Specific Code](#version-specific-code)
- [Testing Version Handling](#testing-recommendations)

### 4. Error Handling {#error-handling}

Proper error handling is critical for healing:
- Use appropriate error types from the `errors` package
- Distinguish between temporary and permanent errors
- Implement appropriate retry logic
- Log errors with sufficient context

**Related Documentation**:
- [Testing Error Handling](#testing-error-handling)
- [Submission Process](#submission-process)

### 5. Caching {#caching}

A caching system has been implemented for the anchor healing process to reduce redundant network requests:

```go
// Simplified example of caching system
type queryCache struct {
    cache map[string]interface{}
    mutex sync.RWMutex
}

func (c *queryCache) Get(key string) (interface{}, bool) {
    c.mutex.RLock()
    defer c.mutex.RUnlock()
    v, ok := c.cache[key]
    return v, ok
}

func (c *queryCache) Set(key string, value interface{}) {
    c.mutex.Lock()
    defer c.mutex.Unlock()
    c.cache[key] = value
}
```

When modifying or extending the caching system:
- Use thread-safe access methods
- Implement appropriate cache invalidation
- Consider cache size limits
- Document cache key structure

**Related Documentation**:
- [Testing Caching](#testing-caching)
- [Light Client Implementation](./light_client.md#light-client-overview)

### 6. Peer Query Requirements {#peer-query-requirements}

The healing process must query validator nodes directly to collect signatures. This is a fundamental requirement of the distributed consensus model:

```go
// In healing/anchors.go
for peer, info := range args.NetInfo.Peers[strings.ToLower(si.Source)] {
    if signed[info.Key] {
        continue
    }
    
    addr := multiaddr.StringCast("/p2p/" + peer.String())
    if len(info.Addresses) > 0 {
        addr = info.Addresses[0].Encapsulate(addr)
    }
    
    // Direct query to the specific validator node
    res, err := args.Client.ForAddress(addr).Private().Sequence(ctx, srcUrl.JoinPath(protocol.AnchorPool), dstUrl, si.Number, private.SequenceOptions{})
    // ... process signature
}
```

#### Why Direct Queries Are Required

1. **Private Key Locality**: Each validator's signature is created using its private key, which is never shared between nodes for security reasons. Only the node that owns a private key can generate a valid signature with it.

2. **Private API Design**: The `Private().Sequence()` API is specifically designed for direct node-to-node communication. There is no mechanism in the current API to request that one node collect signatures from another node.

3. **Security Considerations**: The current design ensures each validator independently verifies and signs transactions, which is essential for the security and integrity of the network.

4. **Implementation Details**: The `ForAddress(addr)` method creates an `AddressedClient` that routes requests directly to the specified node. The `Sequence` method on the server side only returns signatures that the node itself has generated.

#### Multiaddr Requirements

The private interface requires a complete multiaddr specification that includes:

1. **Peer ID**: 
   - The unique identifier for the validator node (line 209: `addr := multiaddr.StringCast("/p2p/" + peer.String())`)
   - This is cryptographically derived from the node's public key

2. **Network Address**:
   - If available, the physical network address is encapsulated with the peer ID (lines 210-211)
   - A complete multiaddr might look like: `/ip4/192.168.1.1/tcp/26656/p2p/QmNodeIdentifierHere`

3. **Protocol Information**:
   - The `/p2p/` prefix indicates the use of the libp2p protocol
   - This is essential for the networking stack to establish the connection

#### Implementation Implications

- The healing process must have network connectivity to all validator nodes to collect signatures
- Network failures between the healing process and any validator will prevent collecting that validator's signature
- The caching system helps mitigate network issues by tracking problematic nodes
- Only a threshold of signatures is required, so the process can succeed even if some validators are unreachable

**Related Documentation**:
- [Signature Collection](./transactions.md#signature-collection)
- [Network Tracking](#network-tracking)

## Network Tracking {#network-tracking}

The `heal_light` approach uses network tracking to identify missing anchors and synthetic transactions:

```go
// In heal_light.go
func (h *healer) healLight(args []string) {
    // ... initialization
    
    // Use the Light client to track the network
    err = h.light.Track(h.ctx, h.C2)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Failed to track the network: %v\n", err)
        return
    }
    
    // ... healing logic
}
```

When modifying network tracking:
- Ensure proper error handling
- Consider performance implications
- Maintain compatibility with different network versions
- Document tracking behavior

**Related Documentation**:
- [Light Client Usage in Synthetic Transaction Healing](./light_client.md#light-client-usage-in-synthetic-healing)
- [Database Usage in Healing Approaches](./database.md#database-usage-in-healing-approaches)

## API Usage Guidelines {#api-usage-guidelines}

<!-- ai:context-management
This section provides guidelines for using the Accumulate API in healing code.
-->

### Core API Interfaces

```go
// Client interface for interacting with the Accumulate API
type Client interface {
    // Query a chain
    QueryChain(ctx context.Context, url *url.URL, query *api.ChainQuery) (*api.ChainQueryResponse, error)
    
    // Query an account
    QueryAccount(ctx context.Context, url *url.URL, query *api.AccountQuery) (*api.AccountQueryResponse, error)
    
    // Submit a transaction
    Submit(ctx context.Context, envelope *protocol.Envelope) (*api.SubmitResponse, error)
}
```

### API Usage Patterns

#### Chain Queries

```go
// Query a chain with proper error handling
func queryChain(ctx context.Context, client api.Client, chainUrl *url.URL) (*api.ChainQueryResponse, error) {
    resp, err := client.QueryChain(ctx, chainUrl, &api.ChainQuery{
        IncludeReceipt: true,
    })
    if err != nil {
        if errors.Is(err, api.ErrNotFound) {
            // Handle not found case
            return nil, fmt.Errorf("chain not found: %v", chainUrl)
        }
        // Handle other errors
        return nil, fmt.Errorf("failed to query chain %v: %w", chainUrl, err)
    }
    return resp, nil
}
```

#### Account Queries

```go
// Query an account with proper error handling
func queryAccount(ctx context.Context, client api.Client, accountUrl *url.URL) (*api.AccountQueryResponse, error) {
    resp, err := client.QueryAccount(ctx, accountUrl, &api.AccountQuery{
        IncludeRecent: true,
    })
    if err != nil {
        if errors.Is(err, api.ErrNotFound) {
            // Handle not found case
            return nil, fmt.Errorf("account not found: %v", accountUrl)
        }
        // Handle other errors
        return nil, fmt.Errorf("failed to query account %v: %w", accountUrl, err)
    }
    return resp, nil
}
```

#### Transaction Submission

```go
// Submit a transaction with proper error handling
func submitTransaction(ctx context.Context, client api.Client, envelope *protocol.Envelope) (*api.SubmitResponse, error) {
    resp, err := client.Submit(ctx, envelope)
    if err != nil {
        // Handle submission errors
        return nil, fmt.Errorf("failed to submit transaction: %w", err)
    }
    return resp, nil
}
```

### API Error Handling Best Practices

1. **Distinguish between error types**:
   - `api.ErrNotFound`: Element doesn't exist
   - `api.ErrBadRequest`: Invalid request parameters
   - Network errors: Temporary connection issues

2. **Implement appropriate retry logic**:
   - Retry temporary errors with backoff
   - Don't retry permanent errors (e.g., bad request)
   - Use context with timeout for bounded retries

3. **Log errors with context**:
   - Include the URL being queried
   - Include the query parameters
   - Include the error message and type

## Implementation Checklist {#implementation-checklist}

<!-- ai:diagnostic-flow
This checklist helps ensure healing code is implemented correctly.
-->

When implementing new healing code or modifying existing code, use this checklist:

### 1. URL Construction {#url-construction-checklist}
- [ ] Consistent URL construction method used throughout the component
- [ ] URL construction method documented
- [ ] Compatible with existing code that will interact with this component

### 2. Database Usage {#database-usage-checklist}
- [ ] Appropriate database interfaces used
- [ ] Transactions properly managed (begin, commit, discard)
- [ ] Error handling for database operations
- [ ] Fallback behavior if database is unavailable

### 3. Version Handling {#version-handling-checklist}
- [ ] Version detection using `ExecutorVersion`
- [ ] Separate code paths for different versions
- [ ] Tested with all supported network versions
- [ ] Version-specific behavior documented

### 4. Error Handling {#error-handling-checklist}
- [ ] Appropriate error types used
- [ ] Retry logic for temporary errors
- [ ] Proper logging with context
- [ ] Error propagation to caller when appropriate

### 5. Caching {#caching-checklist}
- [ ] Thread-safe cache access
- [ ] Appropriate cache invalidation
- [ ] Cache size limits considered
- [ ] Cache key structure documented

### 6. Testing {#testing-checklist}
- [ ] Unit tests for core functionality
- [ ] Integration tests with network components
- [ ] Tests for different network versions
- [ ] Error case testing

**Related Documentation**:
- [Testing Recommendations](#testing-recommendations)
- [Implementation Guidelines](./implementation.md)

## Common Pitfalls {#common-pitfalls}

### 1. URL Construction Differences {#url-construction-differences}

The difference in URL construction between `sequence.go` and `heal_anchor.go` can cause issues:
- Code might look for anchors at different URL paths
- Queries might return "element does not exist" errors
- Anchor relationships might not be properly maintained

To avoid these issues:
- Be explicit about which URL construction method you're using
- Document the expected URL format for your component
- Consider standardizing on one approach for new code

**Related Documentation**:
- [URL Construction](#url-construction)
- [URL Construction Considerations](./database.md#url-construction-considerations)

### 2. Database Transaction Management {#database-transaction-management}

Improper database transaction management can cause issues:
- Forgetting to commit or discard transactions can cause resource leaks
- Using a transaction after it's been committed or discarded can cause errors
- Not handling database errors properly can cause data inconsistency

To avoid these issues:
- Always use defer to ensure transactions are properly managed
- Check error returns from database operations
- Use appropriate transaction isolation levels

**Related Documentation**:
- [Database Interaction](#database-interaction)
- [Database Requirements](./database.md#database-interface-requirements)

### 3. Version-Specific Code {#version-specific-code}

Not handling different network versions correctly can cause issues:
- Code might use features not available in older versions
- Different message formats might not be handled correctly
- Version-specific optimizations might not be applied

To avoid these issues:
- Always check the network version before using version-specific features
- Maintain separate code paths for different versions
- Test with all supported network versions

**Related Documentation**:
- [Version Handling](#version-handling)
- [Testing Version Handling](#testing-version-handling)

## Testing Recommendations {#testing-recommendations}

When testing healing code, focus on:

### 1. URL Construction {#testing-url-construction}
- Test with different URL formats
- Verify URL parsing and construction
- Test edge cases (e.g., special characters, long names)

### 2. Database Interaction {#testing-database-interaction}
- Test transaction management
- Test error handling for database operations
- Test with different database states

### 3. Version Handling {#testing-version-handling}
- Test with different network versions
- Verify version detection
- Test version-specific code paths

### 4. Error Handling {#testing-error-handling}
- Test error propagation
- Test retry logic
- Test error logging

### 5. Caching {#testing-caching}
- Test cache hit/miss behavior
- Test cache invalidation
- Test concurrent cache access

**Related Documentation**:
- [Testing the Database Implementation](./database.md#testing-the-database)
- [Testing the Light Client Implementation](./light_client.md#testing-light-client)

## See Also {#see-also}

- [Healing Overview](./overview.md): A high-level overview of all healing approaches
- [Transaction Creation](./transactions.md): Details on how transactions are created
- [Database Requirements](./database.md): Database interface requirements
- [Light Client Implementation](./light_client.md): Details on the Light client
- [URL Construction Differences](./chains_urls.md#key-differences-in-healing-behavior)
