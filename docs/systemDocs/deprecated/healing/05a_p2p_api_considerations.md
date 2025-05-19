# P2P API Considerations for Healing

<!-- AI-METADATA
type: technical_guide
version: 1.0
topic: p2p_apis
subtopics: ["performance", "alternatives", "healing_implementation"]
related_code: ["tools/cmd/debug/heal_common.go", "pkg/api/v3/p2p/dial.go"]
tags: ["healing", "p2p", "performance", "api", "ai_optimized"]
-->

## Overview

This document provides information about P2P API usage in the Accumulate healing processes and outlines a plan to validate potential performance considerations. The healing implementation makes use of P2P APIs, and there are indications in the code comments that alternative APIs might be preferred in certain scenarios.

## Current P2P API Usage in Healing

The healing tools use P2P APIs in several key areas:

1. **Peer Discovery**: Finding available nodes in the network
2. **Transaction Fetching**: Retrieving transaction data from peers
3. **Network Status Checks**: Verifying the state of the network
4. **Message Submission**: Submitting healed transactions to the network

Key P2P-related code in the healing implementation:

```go
// From heal_common.go
node, err := p2p.New(p2p.Options{
    BootstrapPeers: []multiaddr.Multiaddr{},
    ListenAddrs:    []multiaddr.Multiaddr{},
})
checkf(err, "start p2p node")
```

## Evidence of Potential Concerns

There are indications in the codebase that P2P APIs might have limitations in certain scenarios:

```go
// From heal_common.go
// We should be able to use only the p2p client but it doesn't work well for
// some queries, so we also create a JSON-RPC client
```

This comment suggests that the developers found cases where P2P APIs didn't perform as expected and implemented fallbacks to JSON-RPC.

## Plan to Validate Performance Concerns

To properly evaluate the performance characteristics of P2P APIs compared to alternatives, we should implement a systematic testing approach:

### 1. Benchmark Test Suite

Develop a benchmark test suite that compares P2P APIs with alternatives:

```go
// Example benchmark test structure
func BenchmarkAPIComparison(b *testing.B) {
    // Test scenarios
    scenarios := []struct{
        name string
        size int  // transaction size
        load int  // concurrent requests
    }{
        {"small-low", 1024, 1},
        {"small-high", 1024, 100},
        {"large-low", 1024*1024, 1},
        {"large-high", 1024*1024, 100},
    }
    
    // API types to test
    apiTypes := []string{"p2p", "json-rpc", "rest", "grpc"}
    
    // Run benchmarks
    for _, s := range scenarios {
        for _, api := range apiTypes {
            b.Run(fmt.Sprintf("%s-%s", s.name, api), func(b *testing.B) {
                // Setup test environment
                // Run the benchmark
                // Collect metrics
            })
        }
    }
}
```

### 2. Metrics to Collect

For each API type, collect the following metrics:

- **Latency**: Time to complete requests (min, max, average, p95, p99)
- **Throughput**: Requests per second
- **Resource Usage**: CPU, memory, network I/O
- **Error Rates**: Failed requests percentage
- **Connection Establishment Time**: Time to establish initial connection

### 3. Test Scenarios

Test the following scenarios:

- **Simple Queries**: Basic transaction lookups
- **Complex Queries**: Multi-step operations
- **High Concurrency**: Many simultaneous requests
- **Network Degradation**: Performance under simulated network issues
- **Long-Running Operations**: Behavior during extended operations

### 4. Implementation Plan

1. Create a dedicated test package: `internal/testing/api_benchmarks`
2. Implement client wrappers for each API type with consistent interfaces
3. Create a test harness that can simulate various network conditions
4. Develop data collection and analysis tools
5. Run tests in both controlled environments and production-like settings

## Alternative APIs to Evaluate

The following alternatives should be compared against P2P APIs:

1. **Direct JSON-RPC API**:
   ```go
   client := jsonrpc.NewClient("http://localhost:8080/v3")
   ```

2. **REST API**:
   ```go
   resp, err := http.Get("http://localhost:8080/v3/query-tx?txid=" + txid)
   ```

3. **gRPC API**:
   ```go
   conn, err := grpc.Dial("localhost:8080", grpc.WithInsecure())
   client := api.NewAccumulateClient(conn)
   ```

## Potential Optimization Strategies

Based on the results of the performance testing, we can develop optimization strategies such as:

1. **API Selection Guidelines**: Clear guidance on which API to use for which operation
2. **Hybrid Approach**: Using different APIs for different operations based on their characteristics
3. **Fallback Mechanisms**: Implementing smart fallbacks between API types
4. **Connection Management**: Optimizing connection handling for each API type

## Example Test Implementation

```go
// Example implementation of a test case
func TestP2PvsJSONRPC(t *testing.T) {
    ctx := context.Background()
    txid := createTestTransaction(t)
    
    // Measure P2P performance
    p2pStart := time.Now()
    p2pClient := setupP2PClient(t)
    p2pTx, err := p2pClient.QueryTransaction(ctx, txid, nil)
    p2pDuration := time.Since(p2pStart)
    require.NoError(t, err)
    
    // Measure JSON-RPC performance
    jsonStart := time.Now()
    jsonClient := jsonrpc.NewClient("http://localhost:8080/v3")
    jsonTx, err := jsonClient.QueryTransaction(ctx, txid, nil)
    jsonDuration := time.Since(jsonStart)
    require.NoError(t, err)
    
    // Compare results
    require.Equal(t, p2pTx.Transaction.ID(), jsonTx.Transaction.ID())
    
    // Log performance metrics
    t.Logf("P2P duration: %v", p2pDuration)
    t.Logf("JSON-RPC duration: %v", jsonDuration)
}
```

## Reporting and Documentation

After completing the performance testing, we should:

1. Document the results with detailed metrics
2. Create clear guidelines for API selection based on empirical data
3. Update the healing documentation with evidence-based recommendations
4. Implement any necessary code changes to optimize API usage

## Summary

While there are indications in the code that P2P APIs might have performance implications, we need systematic testing to validate these concerns. This document outlines a comprehensive plan to evaluate P2P APIs against alternatives and develop evidence-based guidelines for API selection in the healing implementation.

## Related Documents

- [API Layers](./05_api_layers.md)
- [API Implementation Analysis](./healing_api_comparison.md)
- [Technical Infrastructure](./00c_technical_index.md)
