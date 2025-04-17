# Advanced Routing Mechanisms in Accumulate

This document explores the advanced routing mechanisms in Accumulate, particularly focusing on alternative routing methods when a URL doesn't have an ADI or when special routing directives are needed.

## Routing Architecture Overview

The Accumulate routing system is built on several key components:

1. **Router Interface**: Defined in `internal/api/routing/router.go`, this interface provides two core methods:
   - `RouteAccount(*url.URL) (string, error)`: Routes based on a URL
   - `Route(...*messaging.Envelope) (string, error)`: Routes based on message envelopes

2. **RouteTree**: Implements a prefix tree for efficient routing based on the URL's routing number.

3. **Routing Overrides**: A mechanism to explicitly route specific accounts regardless of their URL structure.

## URL-Based Routing

The primary routing mechanism in Accumulate is URL-based. Each URL has a routing number derived from its identity:

```go
// Routing returns the first 8 bytes of the identity account ID as an integer.
//
// Routing = uint64(u.IdentityAccountID()[:8])
func (u *URL) Routing() uint64 {
    return binary.BigEndian.Uint64(u.IdentityAccountID())
}
```

This routing number is used to determine which partition should handle the URL.

## Alternative Routing Mechanisms

### 1. Routing Overrides

The most powerful alternative routing mechanism is the **routing override** system. This allows explicit routing of specific accounts regardless of their URL structure:

```go
func (r *RouteTree) Route(u *url.URL) (string, error) {
    s, ok := r.overrides[u.IdentityAccountID32()]
    if ok {
        return s, nil
    }

    return r.RouteNr(u.Routing())
}
```

Overrides are added to the routing table using:

```go
func (r *RoutingTable) AddOverride(account *url.URL, partition string) {
    ptr, _ := sortutil.BinaryInsert(&r.Overrides, func(o RouteOverride) int {
        return o.Account.Compare(account)
    })
    ptr.Account = account
    ptr.Partition = partition
}
```

### 2. Direct Addressing with Multiaddrs

For cases where normal URL-based routing isn't sufficient, Accumulate supports direct addressing using multiaddr format:

```go
// Check if the address specifies a network or p2p node or is /acc-svc/node
sa, network, peer, err := extractAddr(addr)
if err != nil {
    return nil, errors.InternalError.WithFormat("parse routed address: %w", err)
}
if peer != "" || network != "" || sa != nil && sa.Type == api.ServiceTypeNode {
    return addr, nil
}
```

This allows messages to be directed to specific nodes or services regardless of the URL routing.

### 3. Network-Specific Routing

When a URL doesn't contain enough information for routing, the system can fall back to network-specific routing:

```go
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
```

## Handling URLs without ADIs

For URLs without ADIs (which typically lack the traditional routing information), there are several approaches:

1. **Default Partition Routing**: Route to a default partition (often the Directory Network)
2. **Override-Based Routing**: Use explicit overrides for well-known non-ADI URLs
3. **Direct Addressing**: Use multiaddr format to directly specify the destination

## Practical Application for Healing

When implementing the healing process, consider these approaches to resolve routing conflicts:

1. **Use Routing Overrides**: For problematic URLs, explicitly add overrides to ensure consistent routing:

```go
// Add an override for a specific anchor URL
routingTable.AddOverride(anchorUrl, partitionID)
```

2. **Normalize URLs Before Routing**: Ensure all URLs are in a consistent format before attempting to route them:

```go
// Convert anchor pool URLs to partition URLs
if strings.EqualFold(u.Authority, protocol.Directory) && strings.HasPrefix(u.Path, "/anchors/") {
    parts := strings.Split(u.Path, "/")
    if len(parts) >= 3 {
        partitionID := parts[2]
        return protocol.PartitionUrl(partitionID)
    }
}
```

3. **Use Direct Addressing When Necessary**: For cases where normal routing fails, fall back to direct addressing:

```go
// Create a direct address to a specific partition
partitionAddr, _ := multiaddr.NewComponent(api.N_ACC, partitionID)
serviceAddr, _ := api.NewServiceAddress(api.ServiceTypeNode, nil)
addr := partitionAddr.Encapsulate(serviceAddr.Multiaddr())
```

## Conclusion

Understanding these alternative routing mechanisms is crucial for resolving "cannot route message" errors. By leveraging routing overrides, direct addressing, and consistent URL normalization, we can ensure that messages are routed correctly even in complex scenarios where traditional URL-based routing might fail.

For the healing process specifically, implementing URL normalization and utilizing routing overrides when necessary will help prevent routing conflicts and ensure consistent behavior across the system.
