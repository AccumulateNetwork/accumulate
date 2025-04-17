# Enhanced Routing Implementation for Anchor Healing

## Overview

This document describes the enhanced routing implementation for the Accumulate debug tools, specifically focusing on resolving the "cannot route message" errors in the P2P network healing functionality.

## Problem Statement

The anchor healing process was encountering "conflicting routes" errors due to inconsistent URL construction across different parts of the codebase:

- `sequence.go` uses raw partition URLs for tracking (e.g., `acc://bvn-Apollo.acme`)
- `heal_anchor.go` was appending the partition ID to the anchor pool URL (e.g., `acc://dn.acme/anchors/Apollo`)

This inconsistency caused routing conflicts when trying to submit messages to peers, resulting in errors like:

```
cannot route request: cannot route message(s): conflicting routes
```

## Solution Architecture

The solution implements several enhancements to ensure consistent URL construction and routing:

### 1. URL Normalization

The `normalizeUrl` function standardizes URLs to a consistent format:

```go
func normalizeUrl(u *url.URL) *url.URL {
    // Standardize URL format to ensure consistent routing
    // e.g., ensure BVN URLs have the "bvn-" prefix
}
```

### 2. Normalizing Router

The `NormalizingRouter` wraps a standard router and normalizes URLs before routing:

```go
type NormalizingRouter struct {
    Router routing.Router
}

func (r *NormalizingRouter) RouteAccount(u *url.URL) (string, error) {
    normalized := normalizeUrl(u)
    return r.Router.RouteAccount(normalized)
}
```

### 3. Direct Router

The `DirectRouter` provides explicit routing overrides for problematic URLs:

```go
type DirectRouter struct {
    BaseRouter routing.Router
    Overrides  map[string]string // URL string -> partition ID
}

func (r *DirectRouter) RouteAccount(u *url.URL) (string, error) {
    // Check if this URL has an override
    if partition, ok := r.Overrides[u.String()]; ok {
        return partition, nil
    }
    
    // Fall back to the base router
    return r.BaseRouter.RouteAccount(u)
}
```

### 4. Enhanced Submission Loop

The `enhancedSubmitLoop` function improves the submission process with:

- Caching of successful submissions
- Tracking of problematic peers
- Prioritization of successful peers
- Better error handling for routing conflicts

## Implementation Details

### URL Standardization

URLs are standardized to use the format from `sequence.go` (e.g., `acc://bvn-Apollo.acme`) rather than the anchor pool URL format (e.g., `acc://dn.acme/anchors/Apollo`).

### Routing Overrides

Explicit routing overrides are created for potentially problematic URLs:

1. Partition URLs with and without the "bvn-" prefix
2. Directory network URLs with different path structures
3. Anchor URLs with different formats

### Integration Points

The enhanced routing is integrated at several key points:

1. **Router Setup**: The standard router is wrapped with a `DirectRouter` and a `NormalizingRouter`
2. **Message Submission**: URLs are normalized before submission
3. **Anchor Healing**: The healing process uses normalized URLs consistently

## Usage

The enhanced routing is automatically used by the debug tools without requiring any additional configuration. It works transparently to resolve routing conflicts while maintaining compatibility with the existing codebase.

## Limitations

1. This implementation only affects the debug tools and does not modify the core Accumulate ADIs.
2. Some complex routing scenarios may still require manual intervention.

## Future Improvements

1. Consider standardizing URL construction across the entire codebase
2. Implement a more sophisticated routing conflict resolution mechanism
3. Add telemetry to track routing performance and conflicts
