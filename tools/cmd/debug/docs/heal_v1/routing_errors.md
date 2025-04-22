# Understanding "Cannot Route Request" Errors

## Overview

The "cannot route request" errors are a common issue encountered during the healing process in Accumulate. These errors typically manifest with the message:

```
cannot route request: cannot route message(s): conflicting routes
```

This document explains the causes of these errors and how they can be resolved.

## Root Causes

### 1. URL Construction Inconsistencies

The primary cause of routing conflicts is inconsistent URL construction across different parts of the codebase:

- **Different URL Formats**: The same logical entity (e.g., a partition) can be referenced using different URL formats:
  - Partition URL format: `acc://bvn-Apollo.acme`
  - Anchor pool URL format: `acc://dn.acme/anchors/Apollo`

- **Impact on Routing**: These different formats can route to different partitions, causing conflicts when they're used together in a transaction.

### 2. Message Composition Issues

- **Multiple Signatures**: When a message contains multiple signatures that would route to different partitions, a routing conflict occurs.

- **Mixed Message Types**: Combining different message types in a single envelope can lead to routing conflicts if they have different routing destinations.

### 3. Routing Table Configuration

- **Routing Table Changes**: If the routing table has been updated but not all components are aware of the changes, routing conflicts can occur.

- **Overrides vs. General Rules**: Conflicts between specific routing overrides and general routing rules can cause routing errors.

## Technical Details

### How Routing Works

1. **URL-Based Routing**: Each URL in Accumulate has a routing number derived from its components.

2. **Routing Determination**:
   - The `RouteAccount` function determines which partition should handle a specific URL.
   - The `Route` function routes envelopes based on their signatures' routing locations.

3. **Conflict Detection**:
   - The system checks if all parts of a message route to the same partition.
   - If different parts would route to different partitions, a routing conflict error is raised.

### Code Path for Routing Errors

The error originates in the `RouteEnvelopes` function in `router.go`:

```go
func RouteEnvelopes(routeAccount func(*url.URL) (string, error), envs ...*messaging.Envelope) (string, error) {
    // ...
    var route string
    for _, env := range envs {
        for _, msg := range env.Messages {
            err := routeMessage(routeAccount, &route, msg)
            if err != nil {
                return "", errors.UnknownError.WithFormat("cannot route message(s): %w", err)
            }
        }
        // ...
    }
    // ...
}

func routeMessage(routeAccount func(*url.URL) (string, error), route *string, msg messaging.Message) error {
    // ...
    if *route != "" && *route != r {
        return errors.BadRequest.With("conflicting routes")
    }
    return nil
}
```

When the `routeMessage` function detects that a message component would route to a different partition than previously determined, it returns the "conflicting routes" error.

## Common Scenarios

### 1. Anchor Healing Conflicts

During anchor healing, conflicts often occur when:

- The source URL is constructed using one format (e.g., `acc://bvn-Apollo.acme`)
- The destination URL is constructed using another format (e.g., `acc://dn.acme/anchors/Apollo`)
- These URLs route to different partitions, causing a conflict

### 2. Synthetic Transaction Conflicts

Similar conflicts can occur with synthetic transactions when:

- Different components of the transaction reference the same partition using different URL formats
- The routing system treats these as different destinations

### 3. Multi-Signature Transactions

Transactions with multiple signatures can encounter routing conflicts when:

- Signatures from different accounts are included
- These accounts route to different partitions

## Resolution Strategies

### 1. URL Normalization

The most effective solution is to normalize URLs to a consistent format before submission:

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
```

### 2. Enhanced Error Handling

Specifically handling routing conflicts in the submission process:

```go
subs, err := client.Submit(ctx, env, api.SubmitOptions{})
if err != nil && strings.Contains(err.Error(), "conflicting routes") {
    // Apply URL normalization to resolve conflicts
    normalizedEnv := normalizeEnvelopeUrls(env)
    return client.Submit(ctx, normalizedEnv, api.SubmitOptions{})
}
```

### 3. Consistent URL Construction

Standardizing on one URL format throughout the codebase:

- Use `protocol.PartitionUrl(partitionID)` consistently
- Avoid mixing different URL construction methods

## Conclusion

"Cannot route request" errors are primarily caused by inconsistent URL construction across the codebase. By standardizing URL formats and implementing proper normalization, these errors can be effectively eliminated, ensuring reliable message delivery across the Accumulate network.
