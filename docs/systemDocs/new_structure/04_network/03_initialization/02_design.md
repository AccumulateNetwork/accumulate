# Network Initialization Design Document

> **IMPORTANT**: Before implementing any functions, always refer to the [Function Locations Document](/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/tools/cmd/debug/FUNCTION_LOCATIONS.md) to avoid reimplementing existing functionality. This document tracks the canonical location of each function in the debug tools codebase.

## Overview

Network initialization is a critical component of the Accumulate debug tools, responsible for creating a comprehensive model of the network that supports anchor healing, synthetic healing, monitoring, and performance analysis. This document outlines the design and implementation details for network initialization.

## Core Requirements

1. **Complete Network Model**: Create a comprehensive model that includes all validators and non-validators
2. **URL Format Compatibility**: Maintain compatibility with existing URL formats used by different components
3. **Resilient Operation**: Handle network errors and timeouts gracefully
4. **Performance Optimization**: Minimize redundant network requests through caching
5. **Accurate Reporting**: Provide consistent and accurate network status information

## Network Initialization Differences

### Network Status Command vs. Heal Commands

**IMPORTANT**: The network initialization process used by the `heal anchor` and `heal synth` commands is different from the one used by the `network status` command. This difference was causing issues with version information and heights not being properly displayed in the heal commands.

### Version Retrieval Fix

A critical issue was identified and fixed in the version retrieval process for the `heal anchor` command. The problem involved how the version information was being extracted from the API response:

1. **Issue**: The heal_anchor code was incorrectly trying to access the version information directly from the response object (`versionResp.Version`), but the version was actually nested inside the `Data` field and needed to be properly unmarshaled.

2. **API Response Structure**: The version information in the v2 API response is structured as:
   ```json
   {
     "type": "version",
     "data": {
       "commit": "a424cff91ea24d093de4bb077abee41b46a5a01f",
       "version": "v1.4.0",
       "versionIsKnown": true
     }
   }
   ```

3. **Solution**: The fix involved:
   - Using the v2 API consistently
   - Properly marshaling the `Data` field to JSON and then unmarshaling it into a struct with a `Version` field
   - Setting the version correctly in the node structs
   - Adding proper concurrency handling with mutexes
   - Implementing detailed logging for debugging

After these changes, all nodes now correctly report their version as "v1.4.0" instead of the incorrect "0.38.0-rc3" that was previously being reported.

**Implementation Note**: When retrieving version information, always use the approach demonstrated in the `mainnet-status` tool, which has been proven to work correctly.

### Nil Pointer Dereference Fix

A nil pointer dereference issue was identified and fixed in the API V3 querier code that was causing the `heal_anchor` command to crash:

1. **Issue**: The `doQuery` function in `pkg/api/v3/querier.go` was attempting to call `r.MarshalBinary()` on a potentially nil response object returned from the API call.

2. **Root Cause**: The API call could return a nil response without an error, which then caused the nil pointer dereference when trying to access methods on the nil object.

3. **Solution**: The fix involved implementing defensive programming techniques:
   - Adding a nil check in the `doQuery` function to detect and properly handle nil responses
   - Adding checks for required components (router, network info, API client) before making API calls
   - Implementing panic recovery to catch and handle unexpected nil pointer dereferences
   - Adding detailed logging for error conditions
   - Improving error handling with more descriptive messages

After these changes, the `heal_anchor` command no longer crashes with nil pointer dereferences and properly handles error conditions with informative messages.

**Implementation Note**: When making API calls that might return nil responses, always implement defensive programming techniques with explicit nil checks and proper error handling.

1. **Network Status Command**:
   - Implemented in `network.go`
   - Uses a comprehensive approach to collect version information and heights
   - Successfully displays version numbers and heights for all responsive nodes
   - Serves as the reference implementation for how network initialization should work

2. **Heal Commands (anchor, synth)**:
   - Implemented in `heal_network.go` and related files
   - Currently missing proper version information and heights for nodes
   - Needs to be updated to match the network initialization approach used by the network status command
   - Must maintain compatibility with existing URL formats (see URL Construction Differences section)

### Current Fix Approach

We are using the network status command's implementation as a reference to fix the network initialization process used by the heal commands. The goal is to ensure that heal commands properly display version information and heights while maintaining compatibility with the existing URL formats and healing processes.

## Network Characteristics

1. **Heterogeneous Node Types**: The network consists of validators and non-validators, each with different capabilities
2. **Multiple Partitions**: Directory Network (DN) and multiple Block Validator Networks (BVNs)
3. **Dynamic Topology**: Nodes may join or leave the network at any time
4. **API Version Differences**: Nodes may support different API versions (v2, v3, or both)
5. **URL Format Variations**: Different components use different URL formats
6. **Non-responsive Nodes**: Some nodes on mainnet (specifically ConsensusNetworks.acme and DetroitLedgerTech.acme) do not respond to any API requests. These appear as "unknown" for both host and version in the network status output.

## URL Construction Differences

**CRITICAL**: There is a fundamental difference in how URLs are constructed between different components:

1. **Sequence Code** (in `sequence.go`):
   - Uses raw partition URLs for tracking (e.g., `acc://bvn-Apollo.acme`)

2. **Heal Anchor Code** (in `heal_anchor.go`):
   - Appends the partition ID to the anchor pool URL (e.g., `acc://dn.acme/anchors/Apollo`)

### Impact and Resolution

This discrepancy can cause anchor healing to fail because the code might be looking for anchors at different URL paths. The resolution approach is:

- **DO NOT** attempt to standardize URL formats - this will break compatibility with the network
- Each component must continue using the URL format it was designed for
- The heal anchor code must use its specific URL format, and sequence code must use its format
- Any changes to URL handling must be carefully tested against the actual network

## Components and Responsibilities

### 1. Network Information Collection (`collectCompleteNetworkInfo`)

The `collectCompleteNetworkInfo` function in heal_network.go is responsible for:

- Getting network status with appropriate timeouts
- Using ScanNetwork to get peer information for each partition
- Extracting node information using the existing function from heal_node_info.go
- Getting version information for each node
- Checking API v3 capabilities
- Checking consensus status to get accurate partition heights

Implementation details:
```go
func collectCompleteNetworkInfo(ctx context.Context, networkName string) (*healing.NetworkInfo, []*nodeStatus, error) {
    // Initialize a context with timeout for the entire operation
    ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
    defer cancel()
    
    // Get network status with increased timeout
    ctxTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
    ns, err := public.NetworkStatus(ctxTimeout, api.NetworkStatusOptions{})
    cancel()
    if err != nil {
        return nil, nil, fmt.Errorf("failed to get network status: %w", err)
    }

    // Use ScanNetwork to get peer information for each partition
    networkInfo, err := healing.ScanNetwork(ctx, public)
    
    // Extract node information
    nodes := extractNodeInfo(ctx, networkInfo, ns)
    
    // Get version information, check API capabilities, check consensus status
    // ...
    
    return networkInfo, nodes, nil
}
```

### 2. Resilient Network Initialization (`resilientNetworkInitializer`)

The `resilientNetworkInitializer` function in heal_network.go provides:

- Retry capabilities with exponential backoff
- Proper timeout handling
- Detailed error reporting
- Network status reporting

Implementation details:
```go
func resilientNetworkInitializer(ctx context.Context, h *healer, networkName string) {
    // Initial timeout - increased for better network scanning
    timeout := 30 * time.Second
    maxTimeout := 120 * time.Second
    backoffFactor := 1.5

    for {
        // Create a timeout context for this attempt
        attemptCtx, cancel := context.WithTimeout(ctx, timeout)

        // Try to initialize the network
        network, nodes, err := collectCompleteNetworkInfo(attemptCtx, networkName)
        cancel()

        if err == nil && network != nil {
            // Success - store the network information
            h.net = network
            h.nodes = nodes
            // Report network status
            reportNetworkStatus(h.net, h.nodes)
            return
        }

        // Failed attempt - log and prepare for retry
        slog.Error("Network initialization failed, will retry", "error", err)
        
        // Increase timeout with backoff
        timeout = time.Duration(float64(timeout) * backoffFactor)
        if timeout > maxTimeout {
            timeout = maxTimeout
        }

        // Check if we should abort retrying
        select {
        case <-ctx.Done():
            return
        case <-time.After(5 * time.Second):
            // Continue to next attempt
        }
    }
}
```

### 3. URL Construction (Respecting Existing Patterns)

**IMPORTANT**: Different components require different URL formats. Each component must use the URL format expected by the specific API it's calling.

- **Sequence Operations**: Use raw partition URLs (e.g., `acc://bvn-Apollo.acme`)
- **Anchor Healing**: Use anchor pool URLs with partition ID (e.g., `acc://dn.acme/anchors/Apollo`)

The design does NOT standardize URL formats, as this would break compatibility with the network. Instead, it provides utility functions that construct URLs in the format expected by each component:

```go
// Used by sequence operations
func ConstructSequenceChainURL(partitionID string) *url.URL {
    // Construct URL in the format expected by sequence operations
}

// Used by anchor healing
func ConstructAnchorPoolURL(partitionID string) *url.URL {
    // Construct URL in the format expected by anchor healing
}
```

### 4. Caching System

The caching system reduces redundant network requests by:

- Storing query results in a map indexed by a key composed of the URL and query type
- Tracking problematic nodes to avoid querying them for certain types of requests
- Implementing cache invalidation based on time or network changes

Implementation details:
```go
// Cache key construction
func CacheKey(urlStr string, queryType string) string {
    return fmt.Sprintf("%s:%s", urlStr, queryType)
}

// Cache retrieval
func GetFromCache(cache map[string]interface{}, urlStr, queryType string) (interface{}, bool) {
    key := CacheKey(urlStr, queryType)
    value, exists := cache[key]
    return value, exists
}

// Cache storage
func StoreInCache(cache map[string]interface{}, urlStr, queryType string, value interface{}) {
    key := CacheKey(urlStr, queryType)
    cache[key] = value
}
```

### 5. Node Health Classification

Nodes are classified based on their health status:

- **Healthy**: Node is responsive and up-to-date
- **Lagging**: Node is responsive but behind in height
- **Zombie**: Node is unresponsive or stuck at a height
- **Unknown**: Node status cannot be determined

This classification is used to prioritize healthy nodes for queries and provide accurate status reporting.

### 6. Partition Relationship Mapping

The network model includes a mapping of partition relationships:

- Which partitions anchor to which other partitions
- Dependencies between partitions for healing operations
- Anchor sequence tracking between partitions

This mapping is critical for anchor healing and synthetic healing operations.

## Error Handling

1. **Timeouts**: All network operations use context with timeouts to prevent hanging
2. **Retries**: Network initialization includes retry capabilities with exponential backoff
3. **Fallbacks**: When primary nodes are unavailable, the system falls back to alternative nodes
4. **Logging**: Detailed error messages are logged with appropriate context
5. **Circuit Breaking**: Persistently failing nodes are temporarily excluded from queries

## Performance Optimization

1. **Connection Pooling**: Reuse connections to frequently accessed nodes
2. **Parallel Queries**: Perform independent queries in parallel
3. **Prioritization**: Prioritize critical operations during network initialization
4. **Resource Management**: Ensure proper cleanup of resources after operations
5. **Monitoring**: Track performance metrics for optimization

## Implementation Guidelines

1. **Preserve Existing URL Formats**: Each component must continue to use the URL format it was designed for
2. **Comprehensive Testing**: Test each component against the actual network to verify compatibility
3. **Clear Documentation**: Document which URL format is expected by each API
4. **Incremental Improvements**: Focus on making the existing code work correctly before adding new features
5. **Unit Testing**: Create specific tests for URL construction and network initialization
6. **Function Reuse**: Consult FUNCTION_LOCATIONS.md before implementing any new functions to avoid duplication
7. **URL Construction**: Use the appropriate URL construction functions from url_utils.go for each component

## Future Enhancements

After ensuring the current implementation works correctly:

1. **Enhanced Caching**: Implement more sophisticated caching strategies
2. **Dynamic Node Prioritization**: Adjust node priorities based on performance
3. **Automated Recovery**: Implement automated recovery from network failures
4. **Visualization**: Add network visualization capabilities
5. **Standardization Exploration**: Explore possible URL standardization through unit tests, not in the application code
