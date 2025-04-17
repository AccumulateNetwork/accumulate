# Understanding the "Cannot Route Message" Error

## Error Origins and Propagation

The "cannot route message" error is a specific error that occurs during the routing process in Accumulate. This document provides a detailed analysis of how this error originates, propagates through the system, and how it can be resolved.

## Error Generation

### Exact Error Message

The error message typically appears in the following format:

```
cannot route message(s): conflicting routes
```

### Source Code Location

The error originates in the `routeMessage` function in `internal/api/routing/router.go`:

```go
func routeMessage(routeAccount func(*url.URL) (string, error), route *string, msg messaging.Message) error {
    // ...
    if *route != r {
        return errors.BadRequest.With("conflicting routes")
    }
    return nil
}
```

This error is then wrapped and propagated by the `RouteEnvelopes` function:

```go
func RouteEnvelopes(routeAccount func(*url.URL) (string, error), envs ...*messaging.Envelope) (string, error) {
    // ...
    err := routeMessage(routeAccount, &route, msg)
    if err != nil {
        return "", errors.UnknownError.WithFormat("cannot route message(s): %w", err)
    }
    // ...
}
```

## Error Propagation Path

1. **Initial Detection**: The error is first detected in `routeMessage` when it finds that different parts of a message would route to different partitions.

2. **Error Wrapping**: The error is wrapped by `RouteEnvelopes` with the prefix "cannot route message(s): ".

3. **API Layer Propagation**: In the API layer, the `RoutedTransport.routeRequest` function calls `Router.Route`, which in turn calls `RouteEnvelopes`. When an error occurs, it's wrapped again:

   ```go
   if a, err := c.Router.Route(req); err != nil {
       return nil, errors.UnknownError.Wrap(err)
   }
   ```

4. **Client Layer**: Finally, the error reaches the client's `Submit` function, where it's returned to the caller.

## Root Causes

### 1. URL Construction Inconsistencies

The primary cause of routing conflicts is inconsistent URL construction. When different parts of the codebase use different URL formats for the same logical entity, these URLs can route to different partitions.

#### Example:

```go
// In sequence.go - using partition URL format
srcUrl := protocol.PartitionUrl(src.ID)  // e.g., acc://bvn-Apollo.acme

// In heal_anchor.go - using anchor pool URL format
srcUrl := protocol.DnUrl().JoinPath(protocol.AnchorPool).JoinPath(src.ID)  // e.g., acc://dn.acme/anchors/Apollo
```

These two URLs refer to the same logical entity (the Apollo partition) but have different routing numbers and may route to different partitions.

### 2. Message Composition

The error can also occur when a message contains components that would naturally route to different partitions. This happens in the following scenarios:

1. **Multiple Signatures**: When a message has signatures from accounts in different partitions.
2. **Mixed Message Types**: When an envelope contains messages with different routing destinations.
3. **Inconsistent Routing Locations**: When signatures and message destinations have inconsistent routing locations.

## Detailed Error Flow

Let's trace the exact flow of a "cannot route message" error:

1. A message is submitted via `client.Submit()`
2. The message is passed to the transport layer's `RoundTrip` method
3. `RoundTrip` calls `routeRequest` to determine where to send the message
4. `routeRequest` calls `Router.Route` to get the routing address
5. `Router.Route` calls `RouteEnvelopes` to route based on envelope content
6. `RouteEnvelopes` calls `routeMessage` for each message and signature
7. `routeMessage` determines the routing partition for each component
8. If different components route to different partitions, `routeMessage` returns "conflicting routes"
9. This error is wrapped and propagated back through the call stack
10. The client receives "cannot route message(s): conflicting routes"

## Debugging the Error

When you encounter this error, you should:

1. **Examine the Message Components**: Look at all URLs in the message, including:
   - Message destinations
   - Signature routing locations
   - Any other URLs that might affect routing

2. **Check URL Construction**: Verify how each URL is constructed and ensure consistency.

3. **Trace Routing Decisions**: Use logging to trace how each URL is routed.

## Resolution Strategies

### 1. URL Normalization

The most effective solution is to normalize URLs before submission:

```go
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
```

### 2. Consistent URL Construction

Standardize on one URL construction method throughout the codebase:

```go
// CORRECT: Use this consistently
srcUrl := protocol.PartitionUrl(partitionID)

// AVOID: Don't mix with this
srcUrl := protocol.DnUrl().JoinPath(protocol.AnchorPool).JoinPath(partitionID)
```

### 3. Custom Router Implementation

For more complex cases, implement a custom router that normalizes URLs before routing:

```go
type NormalizingRouter struct {
    Router routing.Router
}

func (r *NormalizingRouter) RouteAccount(u *url.URL) (string, error) {
    normalized := normalizeUrl(u)
    return r.Router.RouteAccount(normalized)
}

func (r *NormalizingRouter) Route(envs ...*messaging.Envelope) (string, error) {
    // Normalize URLs in all envelopes
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

## Implemented Solution

We have implemented a comprehensive solution to address the "cannot route message" error in the debug tools:

### 1. Enhanced Direct Router

A `DirectRouter` that explicitly maps problematic URLs to specific partitions:

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

### 2. URL Normalization

Standardized URLs to use the format from `sequence.go` (e.g., `acc://bvn-Apollo.acme`) rather than the anchor pool URL format:

```go
func normalizeAnchorUrl(srcId, dstId string) (*url.URL, *url.URL) {
    // Standardize on the partition URL format used in sequence.go
    srcUrl := protocol.PartitionUrl(srcId)
    dstUrl := protocol.PartitionUrl(dstId)
    
    return srcUrl, dstUrl
}
```

### 3. Enhanced Submission Loop

Implemented a more robust submission loop with:

- Caching of successful submissions
- Tracking of problematic peers
- Prioritization of successful peers
- Better error handling for routing conflicts

### 4. Integration Points

The solution is integrated at key points:

```go
// Create URL overrides for problematic URLs
overrides := createUrlOverrides(h.net)

// Create a direct router with overrides
directRouter := createDirectRouter(h.router, overrides)

// Wrap with a normalizing router for consistent URL handling
normRouter := &NormalizingRouter{Router: directRouter}
h.router = normRouter
```

## Conclusion

The "cannot route message" error is a direct result of routing conflicts in the Accumulate system. By understanding its exact cause and propagation path, we have implemented effective solutions to prevent it.

Our approach standardizes URL construction throughout the debug tools and implements URL normalization at the submission point. This ensures that all components of a message will route to the same partition, eliminating routing conflicts. Additionally, the direct routing overrides provide a safety net for any remaining edge cases.
