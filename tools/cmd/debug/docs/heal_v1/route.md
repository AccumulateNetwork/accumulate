# Accumulate Routing System

## Overview

The Accumulate routing system is responsible for determining which partition should process a particular message or transaction. Routing is a critical component of the Accumulate network architecture, as it ensures that messages are delivered to the correct partition for processing. This document explains how routing works in Accumulate and how URL construction impacts routing decisions.

The routing system operates at multiple levels in the Accumulate architecture:

1. **URL-to-Partition Routing**: Determining which partition should handle a specific URL
2. **Message Routing**: Determining the destination for messages based on their content
3. **Network-Level Routing**: Directing API requests to appropriate nodes

## Routing Architecture

### Core Components

1. **Router Interface**
   - Defined in `internal/api/routing/router.go`
   - Provides two key methods:
     - `RouteAccount(*url.URL) (string, error)`: Routes a single account URL to a partition
     - `Route(...*messaging.Envelope) (string, error)`: Routes envelopes based on their signatures

2. **RouteTree**
   - Defined in `internal/api/routing/tree.go`
   - Implements a prefix tree for efficient routing lookups
   - Contains two routing mechanisms:
     - Overrides map for specific account routing
     - Prefix tree for general routing based on URL routing numbers

3. **Routing Table**
   - Defined in the protocol package
   - Contains routes and overrides that determine how URLs are routed

4. **Message Transport Layer**
   - Defined in `pkg/api/v3/message/transport.go`
   - Implements the `RoutedTransport` which handles message routing
   - Uses the Router interface to determine message destinations

5. **Simple Routing Table Builder**
   - Defined in `internal/api/routing/simple.go`
   - Builds routing tables for simple network configurations

## URL Construction and Routing

### URL Formats in Accumulate

Accumulate uses different URL formats for different types of accounts and operations:

1. **Partition URLs**
   - Format: `acc://bvn-{PartitionID}.{domain}`
   - Example: `acc://bvn-Apollo.acme`
   - Used in: `sequence.go` for tracking partition relationships
   - Created using: `protocol.PartitionUrl(partitionID)`

2. **Anchor Pool URLs**
   - Format: `acc://{domain}/anchors/{PartitionID}`
   - Example: `acc://dn.acme/anchors/Apollo`
   - Used in: `heal_anchor.go` for anchor healing operations
   - Created using: `protocol.DnUrl().JoinPath(protocol.AnchorPool).JoinPath(partitionID)`

3. **Directory URLs**
   - Format: `acc://dn.{domain}`
   - Example: `acc://dn.acme`
   - Created using: `protocol.DnUrl()`

4. **Synthetic Transaction URLs**
   - Format: `acc://{domain}/synthetic`
   - Example: `acc://dn.acme/synthetic`
   - Created using: `url.JoinPath(protocol.Synthetic)`

### How URLs Impact Routing

The way URLs are constructed directly impacts how they are routed:

1. **Routing Numbers**
   - Each URL has a routing number derived from its components
   - The `url.Routing()` method calculates this number based on the URL's hash
   - Different URL formats for the same logical entity can produce different routing numbers

2. **Routing Conflicts**
   - Occur when different parts of a message would route to different partitions
   - Detected in `RouteEnvelopes` function in `router.go`
   - Error message: `"cannot route message(s): conflicting routes"`

3. **URL Construction Inconsistencies**
   - When different parts of the codebase use different URL formats for the same entity
   - Can lead to routing conflicts when these URLs are used together in a transaction

4. **Routing Process Flow**
   - When a message is submitted, the `routeMessage` function is called for each message component
   - For each component, `RouteAccount` is called to determine the partition
   - If different components route to different partitions, a conflict error is returned
   - The `RoutedTransport` in the API layer uses this routing information to direct messages

## Current Issues and Best Practices

### Identified Issues

1. **Inconsistent URL Construction**
   - `sequence.go` uses raw partition URLs (e.g., `acc://bvn-Apollo.acme`)
   - `heal_anchor.go` uses anchor pool URLs with partition IDs (e.g., `acc://dn.acme/anchors/Apollo`)
   - This inconsistency leads to routing conflicts during message submission

2. **Error Handling**
   - Current error handling doesn't specifically address routing conflicts
   - Retry mechanisms don't resolve the underlying URL inconsistency

3. **Routing Table Complexity**
   - The routing table uses a prefix tree structure that can be complex to debug
   - Overrides can create unexpected routing behaviors

4. **API-Level Routing**
   - The `RoutedTransport` in the API layer adds another level of routing complexity
   - Messages may be routed differently at the API level versus the protocol level

### Best Practices

1. **Standardized URL Construction**
   - Use a consistent approach across the codebase
   - Prefer the format used in `sequence.go` as it appears to be the original implementation
   - Always use the protocol package's URL construction functions:
     - `protocol.PartitionUrl(partitionID)` for partition URLs
     - `protocol.DnUrl()` for directory URLs

2. **URL Normalization**
   - Implement URL normalization functions to convert between different formats
   - Apply normalization before routing to ensure consistency
   - Normalize at the submission point to catch all inconsistencies

3. **Caching**
   - Cache successful URL resolutions to avoid redundant calculations
   - Track problematic URL patterns to avoid repeated failures
   - Implement a thread-safe caching mechanism

4. **Routing Error Handling**
   - Specifically check for "conflicting routes" errors
   - Implement fallback mechanisms that apply URL normalization
   - Log detailed information about routing conflicts for debugging

## Implementation Guide

### URL Standardization

To standardize URL construction:

```go
// Convert anchor pool URL to partition URL
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

// Normalize URLs in a message to ensure consistent routing
func normalizeUrlsInMessage(msg interface{}) {
    switch m := msg.(type) {
    case interface{ GetDestination() *url.URL }:
        if dest := m.GetDestination(); dest != nil {
            if setter, ok := m.(interface{ SetDestination(*url.URL) }); ok {
                setter.SetDestination(normalizeUrl(dest))
            }
        }
    case interface{ GetUrl() *url.URL }:
        if u := m.GetUrl(); u != nil {
            if setter, ok := m.(interface{ SetUrl(*url.URL) }); ok {
                setter.SetUrl(normalizeUrl(u))
            }
        }
    }
}
```

### Enhanced Error Handling

To better handle routing conflicts:

```go
// Submit with routing conflict handling
func submitWithRoutingHandling(client api.Client, env *messaging.Envelope) ([]*api.Submission, error) {
    // First try with original envelope
    subs, err := client.Submit(ctx, env, api.SubmitOptions{})
    if err == nil {
        return subs, nil
    }
    
    // Check specifically for routing conflicts
    if errors.Is(err, errors.BadRequest) && strings.Contains(err.Error(), "conflicting routes") {
        slog.Warn("Detected routing conflict, attempting with normalized URLs")
        
        // Create a new envelope with normalized URLs
        normalizedEnv := new(messaging.Envelope)
        for _, msg := range env.Messages {
            // Apply URL normalization
            normalizeUrlsInMessage(msg)
            normalizedEnv.Messages = append(normalizedEnv.Messages, msg)
        }
        
        // Try submission with normalized envelope
        return client.Submit(ctx, normalizedEnv, api.SubmitOptions{})
    }
    
    // Return original error if not a routing conflict
    return nil, err
}
```

### Integration with Message Transport

To integrate with the API message transport layer:

```go
// Custom router that normalizes URLs before routing
type NormalizingRouter struct {
    Router routing.Router
}

func (r *NormalizingRouter) RouteAccount(u *url.URL) (string, error) {
    normalized := normalizeUrl(u)
    return r.Router.RouteAccount(normalized)
}

func (r *NormalizingRouter) Route(envs ...*messaging.Envelope) (string, error) {
    // Create normalized copies of the envelopes
    normalizedEnvs := make([]*messaging.Envelope, len(envs))
    for i, env := range envs {
        normalizedEnv := new(messaging.Envelope)
        for _, msg := range env.Messages {
            normalizeUrlsInMessage(msg)
            normalizedEnv.Messages = append(normalizedEnv.Messages, msg)
        }
        normalizedEnvs[i] = normalizedEnv
    }
    
    return r.Router.Route(normalizedEnvs...)
}
```

## Routing Implementation Details

### RouterInstance Implementation

The `RouterInstance` in `internal/api/routing/router.go` is the primary implementation of the Router interface:

```go
func (r *RouterInstance) RouteAccount(account *url.URL) (string, error) {
    if r.tree == nil {
        return "", errors.InternalError.With("the routing table has not been initialized")
    }
    if protocol.IsUnknown(account) {
        return "", errors.BadRequest.With("URL is unknown, cannot route")
    }

    route, err := r.tree.Route(account)
    if err != nil {
        r.logger.Debug("Failed to route", "account", account, "error", err)
        return "", errors.UnknownError.Wrap(err)
    }

    r.logger.Debug("Routing", "account", account, "to", route)
    return route, nil
}

func (r *RouterInstance) Route(envs ...*messaging.Envelope) (string, error) {
    return RouteEnvelopes(r.RouteAccount, envs...)
}
```

### Message Transport Routing

The `RoutedTransport` in `pkg/api/v3/message/transport.go` handles routing at the API level:

```go
func (c *RoutedTransport) routeRequest(req Message) (multiaddr.Multiaddr, error) {
    // Extract the address if it's already specified
    var addr multiaddr.Multiaddr
    if a, ok := req.(*Addressed); ok {
        addr = a.Address
        req = a.Message
        
        // Extract the service address if it's specified
        sa, _, _, err := extractAddr(addr)
        if err != nil {
            return nil, errors.InternalError.WithFormat("parse address: %w", err)
        }
        if sa != nil {
            goto routed // Skip routing
        }
    }

    // Can we route it?
    if c.Router == nil {
        return nil, errors.BadRequest.With("cannot route message: router not setup")
    }

    // Ask the router to route the request
    if a, err := c.Router.Route(req); err != nil {
        return nil, errors.UnknownError.Wrap(err)
    } else if addr == nil {
        addr = a
    } else {
        addr = addr.Encapsulate(a)
    }

routed:
    // Check if the address specifies a network or p2p node or is /acc-svc/node
    sa, network, peer, err := extractAddr(addr)
    if err != nil {
        return nil, errors.InternalError.WithFormat("parse routed address: %w", err)
    }
    if peer != "" || network != "" || sa != nil && sa.Type == api.ServiceTypeNode {
        return addr, nil
    }

    // Did the caller specify a network?
    if c.Network == "" {
        return nil, errors.BadRequest.With("cannot route message: network is unspecified")
    }

    // Add the network
    net, err := multiaddr.NewComponent(api.N_ACC, c.Network)
    if err != nil {
        return nil, errors.UnknownError.WithFormat("build network address: %w", err)
    }

    // Encapsulate the address with /acc/{network}
    return net.Encapsulate(addr), nil
}
```

## Conclusion

Consistent URL construction is critical for proper routing in the Accumulate network. By standardizing URL formats and implementing proper normalization, we can avoid routing conflicts and ensure reliable message delivery across the network.

The routing system in Accumulate operates at multiple levels, from the low-level URL routing to the API message transport layer. Understanding these layers and how they interact is essential for developing robust applications and for troubleshooting issues like the "cannot route message" errors encountered in the healing process.

By implementing the best practices outlined in this document, particularly standardizing on the `protocol.PartitionUrl()` approach and adding URL normalization, we can eliminate routing conflicts and improve the reliability of the healing process.
