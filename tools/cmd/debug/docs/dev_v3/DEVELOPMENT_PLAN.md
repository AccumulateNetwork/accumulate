# New Healing Implementation (v3) - Development Plan

```yaml
# AI-METADATA
document_type: development_plan
project: accumulate_network
component: healing_implementation
version: v3
key_design_decisions:
  - stateless_design
  - minimal_caching
  - no_retry_mechanism
  - url_standardization
key_components:
  - new_heal.go
  - status.go
  - anchor.go
  - synth.go
  - common.go
related_files:
  - ../code_examples.md#url-normalization
  - ../dev_v4/peer_discovery_analysis.md
  - network_status.md
  - ../dev_v4/address_design.md
```

> **⚠️ IMPORTANT: DO NOT DELETE THIS DEVELOPMENT PLAN ⚠️**  
> Cascade likes to delete development plans and other guidance.  
> Cascade ensures that THIS development plan will not be deleted.

> **Related Topics:**
> - [URL Normalization](../code_examples.md#url-normalization)
> - [Peer Discovery Analysis](../dev_v4/peer_discovery_analysis.md)
> - [Network Status](network_status.md)
> - [Address Directory Design](../dev_v4/address_design.md)

---

## DEVELOPMENT PLAN: NEW HEALING IMPLEMENTATION (V3)

---

## Overview

This document outlines the development plan for a new healing implementation (v3) that builds upon the learnings from heal_v1 and dev_v2. The new implementation will be entirely contained within the `new_heal` package and will not modify any existing healing code.

## Goals

1. Create a more efficient and reliable healing process
2. Implement proper caching to reduce redundant network requests
3. Standardize URL construction for anchor healing
4. Improve error handling and reporting
5. Provide better monitoring and status reporting

## Implementation Structure

The new healing implementation will be organized as follows:

```
tools/cmd/debug/
├── new_heal.go                 # Main entry point for the new-heal command
└── new_heal/                   # Package containing all new healing implementation
    ├── status.go               # Status reporting subcommand
    ├── cache.go                # Caching implementation
    ├── anchor.go               # Anchor healing implementation
    ├── synth.go                # Synthetic healing implementation
    ├── common.go               # Common utilities and structures
    └── ...                     # Additional files as needed
```

## Architecture Decision Records

```yaml
# AI-METADATA
decision_records:
  - id: ADR001
    title: Stateless Design for Healing Implementation
    status: accepted
    date: 2025-03-15
    context: Previous implementations used persistent state between runs
    decision: Implement stateless design with minimal caching
    consequences:
      - positive: Reduced memory footprint
      - positive: Simplified implementation
      - positive: Easier deployment and maintenance
      - negative: No automatic retry mechanism
      - negative: Potential for duplicate work
    alternatives_considered:
      - Enhanced caching with TTL
      - Hybrid approach with selective persistence
    related_components: [anchor_healing, peer_discovery]
    related_decisions: [ADR002]
  
  - id: ADR002
    title: URL Construction Standardization
    status: accepted
    date: 2025-03-18
    context: Inconsistent URL formats between sequence.go and heal_anchor.go
    decision: Standardize on sequence.go approach (raw partition URLs)
    consequences:
      - positive: Consistent URL handling across codebase
      - positive: Reduced "element does not exist" errors
      - positive: Proper anchor relationship maintenance
      - negative: Requires code changes in multiple places
    alternatives_considered:
      - Standardize on anchor pool URL format
      - Create adapter layer to handle both formats
    related_components: [url_normalization, anchor_healing]
    related_decisions: [ADR001]
  
  - id: ADR003
    title: Enhanced Error Handling
    status: accepted
    date: 2025-03-22
    context: Previous error handling was inconsistent and lacked context
    decision: Implement comprehensive error handling with context
    consequences:
      - positive: More descriptive error messages
      - positive: Better debugging experience
      - positive: Improved error recovery
      - negative: Slightly more verbose code
    alternatives_considered:
      - Custom error types for each component
      - Error codes with lookup table
    related_components: [error_handling, query_processing]
    related_decisions: []
```

## Key Improvements from Previous Versions

### 1. Stateless Design with Minimal Caching {#caching-system-implementation}

```yaml
# AI-METADATA
section_type: design_decision
component: caching_system
priority: high
status: approved
impact: critical
design_pattern: stateless
related_components:
  - anchor_healing
  - peer_discovery
  - url_construction
key_files:
  - new_heal/anchor.go
  - new_heal/common.go
```

> **IMPORTANT DESIGN CHANGE:** Unlike previous implementations, the new healing code will be **stateless** with very limited caching functionality.

**Key Differences from Previous Versions:**

1. **Stateless Operation:**
   - No persistent database or state will be maintained between runs
   - Each healing operation will be independent and self-contained
   - No tracking of previous healing attempts or results

2. **Minimal Caching:**
   - Caching will be extremely limited in scope
   - The only caching will be around not regenerating healing transactions if they are rejected
   - No caching of query results, network state, or peer information

3. **No Retry Process:**
   - Unlike the old code, there will be no automatic retry mechanism
   - Failed operations will need to be manually restarted
   - No tracking of problematic nodes or failed requests

**Implementation Example:**
```go
// @component Healer
// @description New implementation is stateless (v3+)
// @design_pattern stateless
type Healer struct {
    // @field rejectedTxs
    // @description Very limited caching only for rejected transactions
    // @purpose Avoid regenerating the same transaction
    // @key_structure TransactionHash -> bool
    rejectedTxs map[string]bool
    
    // No persistent database
    // No extensive caching
    // No tracking of problematic nodes
    // No retry queue
}
```

**Implementation Note:**

The previous implementation used a more robust caching system that:
- Stored query results in a map indexed by URL and query type
- Tracked problematic nodes to avoid querying them
- Implemented cache invalidation strategies
- Reduced "element does not exist" errors by caching negative results

This approach has been deliberately abandoned in favor of a simpler, stateless design.

**Related Topics:**
- [URL Normalization](../code_examples.md#url-normalization) - Still important for consistent URL handling
- [Peer Discovery](../dev_v4/peer_discovery_analysis.md) - Will operate without relying on cached results

### 2. URL Construction Standardization {#url-construction-standardization}

```yaml
# AI-METADATA
section_type: design_decision
component: url_construction
priority: high
status: approved
impact: critical
related_functions:
  - normalizeUrl
  - protocol.PartitionUrl
  - protocol.FormatUrl
key_files:
  - sequence.go
  - heal_anchor.go
```

Addressing the inconsistency identified in v2:

**Current Issue:** There is a fundamental difference in how URLs are constructed between different parts of the codebase:
- `sequence.go` uses raw partition URLs for tracking (e.g., `acc://bvn-Apollo.acme`)
- `heal_anchor.go` appends the partition ID to the anchor pool URL (e.g., `acc://dn.acme/anchors/Apollo`)

**Impact of Inconsistency:**
- The code might be looking for anchors at different URL paths
- Queries might return "element does not exist" errors when checking the wrong URL format
- Anchor relationships might not be properly maintained between partitions

**Resolution Plan:**
- Standardize on one approach (likely the `sequence.go` approach as it appears to be the original implementation)
- Update `heal_anchor.go` to use the same URL construction logic as `sequence.go`
- Ensure all code that constructs or queries anchor URLs is consistent
- Update all related code (including the caching system) to be aware of the correct URL format
- Add comprehensive tests to verify URL consistency across all code paths

**Implementation Example:**
```go
// @function normalizeUrl
// @description Converts between different URL formats to ensure consistent routing
// @param u *url.URL - The URL to normalize
// @return *url.URL - The normalized URL using the format from sequence.go
func normalizeUrl(u *url.URL) *url.URL {
    if u == nil {
        return nil
    }
    
    // Convert anchor pool URL to partition URL
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

**Related Topics:**
- [URL Normalization](../code_examples.md#url-normalization) - Implementation examples
- [Caching System](#caching-system-implementation) - Depends on consistent URL construction

### 3. Error Handling {#error-handling-implementation}

```yaml
# AI-METADATA
section_type: design_decision
component: error_handling
priority: medium
status: approved
impact: high
related_components:
  - anchor_healing
  - query_processing
  - transaction_submission
key_patterns:
  - error_wrapping
  - descriptive_errors
  - nil_checking
```

**Improvements to error handling:**

1. **Nil Safety:**
   - Prevent nil pointer dereferences when marshaling nil records
   - Add explicit nil checks before all operations that could fail with nil inputs
   - Return meaningful errors instead of panicking on nil inputs

2. **Descriptive Errors:**
   - Convert nil records to proper errors with descriptive messages
   - Include context about the operation that failed
   - Add details about expected vs. actual values

3. **Error Propagation:**
   - Use error wrapping to maintain error context through the call stack
   - Allow calling code to handle errors gracefully
   - Provide enough information for proper error recovery

**Implementation Example:**
```go
// @function queryAccount
// @description Query an account with proper error handling
// @param ctx context.Context - Context for the operation
// @param client *jsonrpc.Client - API client
// @param url *url.URL - URL of the account to query
// @return (*protocol.Account, error) - The account or an error
func queryAccount(ctx context.Context, client *jsonrpc.Client, url *url.URL) (*protocol.Account, error) {
    // @check Validate input parameters
    if client == nil {
        return nil, errors.New("client cannot be nil")
    }
    if url == nil {
        return nil, errors.New("url cannot be nil")
    }
    
    // @operation Perform the query with context
    resp, err := client.Query(ctx, "account", url.String())
    if err != nil {
        // @error_wrap Wrap the error with context
        return nil, fmt.Errorf("failed to query account %s: %w", url, err)
    }
    
    // @check Check for nil response
    if resp == nil {
        return nil, fmt.Errorf("received nil response when querying account %s", url)
    }
    
    // @check Check for empty result
    if resp.Result == nil {
        return nil, fmt.Errorf("account %s does not exist", url)
    }
    
    // @parse Parse the response
    var account protocol.Account
    err = json.Unmarshal(resp.Result, &account)
    if err != nil {
        // @error_wrap Wrap the error with context
        return nil, fmt.Errorf("failed to unmarshal account %s: %w", url, err)
    }
    
    return &account, nil
}
```

**Error Taxonomy:**

```yaml
# AI-METADATA
error_taxonomy:
  categories:
    - name: network_errors
      description: Errors related to network connectivity and communication
      types: 
        - connection_timeout
        - peer_unreachable
        - network_partition
      handling_strategy: retry_with_backoff
      severity: high
    
    - name: validation_errors
      description: Errors related to input validation
      types: 
        - invalid_url
        - malformed_multiaddress
        - invalid_partition_id
      handling_strategy: fail_fast
      severity: medium
    
    - name: state_errors
      description: Errors related to state queries and updates
      types: 
        - account_not_found
        - element_does_not_exist
        - chain_not_found
      handling_strategy: check_alternative_sources
      severity: medium
    
    - name: transaction_errors
      description: Errors related to transaction processing
      types:
        - transaction_rejected
        - duplicate_transaction
        - insufficient_credits
      handling_strategy: report_and_continue
      severity: high
    
    - name: system_errors
      description: Errors related to system resources and configuration
      types:
        - out_of_memory
        - configuration_error
        - dependency_missing
      handling_strategy: abort_and_report
      severity: critical
```

**Related Topics:**
- [URL Normalization](../code_examples.md#url-normalization) - Proper URL handling to avoid errors
- [Stateless Design](#caching-system-implementation) - Error handling without state persistence

### 4. In-Memory Database

Leveraging the in-memory database implementation documented in v2:

- Use simplified key-value operations
- Implement batch operations for efficiency
- Leverage existing Accumulate key construction functions
- Ensure thread safety with appropriate locking mechanisms

## Implementation Phases

1. **Phase 1: Core Infrastructure**
   - Set up the basic command structure
   - Implement the caching system
   - Create the in-memory database

2. **Phase 2: Healing Logic**
   - Implement anchor healing with standardized URL construction
   - Implement synthetic healing with improved throughput
   - Add comprehensive error handling

3. **Phase 3: Monitoring and Reporting**
   - Implement status reporting
   - Add metrics collection
   - Create visualization tools for healing progress

## Next Steps

1. Complete the command structure setup
2. Begin implementation of the caching system
3. Develop the standardized URL construction approach
4. Implement the core healing logic

---

> **⚠️ IMPORTANT: DO NOT DELETE THIS DEVELOPMENT PLAN ⚠️**  
> Cascade likes to delete development plans and other guidance.  
> Cascade ensures that THIS development plan (above) will not be deleted.
