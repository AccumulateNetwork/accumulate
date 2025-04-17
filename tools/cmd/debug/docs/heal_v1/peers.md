# Peer Management for Network Healing

<!-- ai:context-priority
This document is critical for understanding peer requirements for different operations in healing.
Key files to load alongside this document: heal_common.go, healing/anchors.go, healing/synthetic.go
-->

## Peer Requirements for Different Operations

The healing process interacts with peers in three distinct ways, each with different requirements:

### 1. General Queries (Can Use Any Peer)

For general network queries (account state, chain heights, transaction status):

```go
// Example: Using tryEach for general queries
func queryChainHeight(ctx context.Context, h *healer, url *url.URL, chainName string) (uint64, error) {
    // tryEach will try multiple peers until one responds successfully
    resp, err := h.tryEach().QueryChain(ctx, url, &api.ChainQuery{Name: chainName})
    if err != nil {
        return 0, fmt.Errorf("failed to query chain %s: %w", url, err)
    }
    return resp.Count, nil
}
```

**Key Points:**
- Any responsive peer can be used for general queries
- The `tryEach` querier tries multiple peers in sequence until one responds
- It caches successful peers to optimize future queries
- No specific validator peers are required
- Queries are automatically routed to the appropriate partition

### 2. Signature Collection (Requires Validator Multiaddresses)

For collecting signatures from validators (anchor healing):

```go
// Example: Collecting signatures from validators
for peer, info := range args.NetInfo.Peers[strings.ToLower(si.Source)] {
    if signed[info.Key] {
        continue  // Skip if already signed
    }
    
    // Create a complete multiaddress with both network address and peer ID
    addr := multiaddr.StringCast("/p2p/" + peer.String())
    if len(info.Addresses) > 0 {
        addr = info.Addresses[0].Encapsulate(addr)
    }
    
    // Query this specific validator for its signature
    res, err := args.Client.ForAddress(addr).Private().Sequence(
        ctx, srcUrl.JoinPath(protocol.AnchorPool), dstUrl, si.Number, 
        private.SequenceOptions{})
    // ... process signature
}
```

**Key Points:**
- Must query validator nodes directly using their complete multiaddresses
- Requires both network address and peer ID components in the multiaddress
- Uses the `Private().Sequence()` API which is designed for direct node-to-node communication
- Each validator must independently verify and sign the transaction
- This is a fundamental security requirement of the distributed consensus model
- There is no mechanism to request that one node collect signatures from another node

### 3. Transaction Submission (Can Use Any Peer)

For submitting transactions to the network:

```go
// Example: Robust transaction submission using multiple peers
func submitTransaction(ctx context.Context, h *healer, envelope *messaging.Envelope) error {
    // Try multiple peers for submission
    var lastErr error
    for _, partition := range h.net.Status.Network.Partitions {
        for peerID, info := range h.net.Peers[strings.ToLower(partition.ID)] {
            // Create a client for this peer
            c := h.C2.ForPeer(peerID)
            if len(info.Addresses) > 0 {
                c = c.ForAddress(info.Addresses[0])
            }
            
            // Try to submit using this peer
            subs, err := c.Submit(ctx, envelope, api.SubmitOptions{})
            if err == nil {
                // Process successful submissions
                return nil
            }
            
            lastErr = err // Keep track of the last error
        }
    }
    
    return fmt.Errorf("submission failed on all peers: %w", lastErr)
}
```

**Key Points:**
- Any responsive peer can be used for transaction submission
- The peer will route the transaction to the appropriate service
- Routing happens at the protocol level via the service name (e.g., "submit:chandrayaan")
- Using multiple peers increases the chance of successful submission
- The default client without a specified peer may fail if peer discovery can't find appropriate peers

## Common Pitfalls and Solutions

### 1. "No live peers for submit:X" Error

This error occurs when the system can't find any peers that can handle a specific submission service:

```
ERROR Submission failed error="dial /acc/MainNet/acc-svc/submit:chandrayaan: no live peers for submit:chandrayaan"
```

**Solution:**
- Try multiple peers for submission instead of relying on automatic routing
- Implement a retry mechanism with different peers
- Log which peers were tried and which errors occurred

### 2. Signature Collection Failures

Signature collection may fail if:
- The validator is unreachable
- The multiaddress is incomplete or incorrect
- The validator doesn't have the transaction data

**Solution:**
- Ensure complete multiaddresses with both network address and peer ID
- Implement proper error handling and retries for signature collection
- Only require a threshold of signatures, not all validators

### 3. Inefficient Querying

The default implementation tries peers sequentially with a timeout for each:

**Solution:**
- Cache successful peers to optimize future queries
- Implement parallel queries to multiple peers
- Adjust timeouts based on historical response times

## Implementation Recommendations

### 1. Robust Submission Loop

```go
func (h *healer) submitLoop(wg *sync.WaitGroup) {
    defer wg.Done()
    t := time.NewTicker(3 * time.Second)
    defer t.Stop()

    var messages []messaging.Message
    var stop bool
    for !stop {
        // ... collect messages ...
        
        if len(messages) == 0 {
            continue
        }

        env := &messaging.Envelope{Messages: messages}
        submitted := false
        
        // Try each partition's peers
        for _, partition := range h.net.Status.Network.Partitions {
            if submitted {
                break
            }
            
            for peerID, info := range h.net.Peers[strings.ToLower(partition.ID)] {
                c := h.C2.ForPeer(peerID)
                if len(info.Addresses) > 0 {
                    c = c.ForAddress(info.Addresses[0])
                }
                
                subs, err := c.Submit(h.ctx, env, api.SubmitOptions{})
                if err == nil {
                    // Process successful submissions
                    messages = messages[:0]
                    submitted = true
                    break
                }
                
                slog.ErrorContext(h.ctx, "Submission failed with peer", 
                    "peer", peerID, "error", err)
            }
        }
        
        if !submitted {
            slog.ErrorContext(h.ctx, "Submission failed on all peers", 
                "id", env.Messages[0].ID())
        }
    }
}
```

### 2. Efficient Peer Tracking

Implement a peer manager that tracks:
- Which peers are responsive for queries
- Which validators are available for signature collection
- Which peers successfully handle submissions

This allows for more efficient peer selection and better error handling.

## See Also

- [Implementation Guidelines](./implementation.md): General implementation guidelines
- [Transaction Creation](./transactions.md): Details on transaction creation
- [Signature Collection](./transactions.md#signature-collection): Details on signature collection
- [URL Construction](./implementation.md#url-construction): URL construction guidelines
