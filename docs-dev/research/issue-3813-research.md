# Research: Wire GossipSub into accumulated integration

## Summary

The DAG-BFT consensus requires a libp2p `host.Host` and `*pubsub.PubSub` (GossipSub) to be passed to `consensus.NewNode()` for multi-validator communication. Currently, `internal/node/dagbft/service.go:130` passes `nil, nil` for these parameters, which means the gossip layer is disabled and DAG-BFT only works in single-validator mode. The `consensus-testnet` binary demonstrates working GossipSub integration. The fix requires exposing the libp2p host from the existing `p2p.Node` and creating a dedicated GossipSub instance for consensus messaging.

## Verified Facts

### Fact 1: NewNode accepts host and pubsub parameters
- **Source**: `pkg/consensus/consensus.go:112`
- **Content**: `func NewNode(config NodeConfig, committee *types.Committee, h host.Host, ps *pubsub.PubSub) (*Node, error)`
- **Confidence**: HIGH

### Fact 2: Service.Start passes nil for host and pubsub
- **Source**: `internal/node/dagbft/service.go:130`
- **Content**: `s.node, err = consensus.NewNode(nodeConfig, committee, nil, nil)`
- **Confidence**: HIGH

### Fact 3: Gossip layer is conditionally created when host/pubsub are non-nil
- **Source**: `pkg/consensus/consensus.go:128-136`
- **Content**:
```go
// Create gossip layer (may be nil for testing)
var g *gossip.GossipLayer
var err error
if h != nil && ps != nil {
    g, err = gossip.NewGossipLayer(h, ps, config.Partition)
    if err != nil {
        return nil, fmt.Errorf("create gossip layer: %w", err)
    }
}
```
- **Confidence**: HIGH

### Fact 4: GossipLayer requires both host.Host and *pubsub.PubSub
- **Source**: `pkg/consensus/gossip/gossip.go:84-93`
- **Content**:
```go
func NewGossipLayerWithOptions(h host.Host, ps *pubsub.PubSub, partition string, opts GossipLayerOptions) (*GossipLayer, error) {
    if h == nil {
        return nil, fmt.Errorf("host is nil")
    }
    if ps == nil {
        return nil, fmt.Errorf("pubsub is nil")
    }
    ...
}
```
- **Confidence**: HIGH

### Fact 5: consensus-testnet creates its own host and GossipSub
- **Source**: `cmd/consensus-testnet/main.go:196-216`
- **Content**:
```go
host, err := libp2p.New(
    libp2p.Identity(libp2pKey),
    libp2p.ListenAddrs(listenMA),
)
...
ps, err := pubsub.NewGossipSub(ctx, host)
...
node, err := consensus.NewNode(nodeConfig, committee, host, ps)
```
- **Confidence**: HIGH

### Fact 6: The existing p2p.Node has a private host field
- **Source**: `pkg/api/v3/p2p/p2p.go:39`
- **Content**: `host     host.Host`
- **Confidence**: HIGH

### Fact 7: inst.p2p is the p2p.Node available in DAGBFTService.start()
- **Source**: `cmd/accumulated/run/dagbft.go:171`
- **Content**: `dialer := inst.p2p.DialNetwork()` (demonstrates inst.p2p is accessible)
- **Confidence**: HIGH

### Fact 8: ServiceConfig does not include host/pubsub fields
- **Source**: `internal/node/dagbft/service.go:30-49`
- **Content**:
```go
type ServiceConfig struct {
    Partition *protocol.PartitionInfo
    NodeConfig consensus.NodeConfig
    Adapter adapter.ConsensusAdapter
    EventBus *events.Bus
    Logger logging.Logger
    Genesis string
}
```
- **Confidence**: HIGH

### Fact 9: GossipSub topics are partition-specific
- **Source**: `pkg/consensus/gossip/topics.go:22-28`
- **Content**:
```go
const (
    TopicBatches  = "acc/%s/consensus/batches"
    TopicHeaders  = "acc/%s/consensus/headers"
    TopicVotes    = "acc/%s/consensus/votes"
    TopicCerts    = "acc/%s/consensus/certs"
    TopicCertSync = "acc/%s/consensus/cert-sync"
)
```
- **Confidence**: HIGH

### Fact 10: The existing p2p discovery already creates a separate GossipSub
- **Source**: `pkg/api/v3/p2p/discovery.go:64`
- **Content**: `ps, err := pubsub.NewGossipSub(ctx, host)`
- **Confidence**: HIGH

## Code References

### Primary Implementation Files
1. `internal/node/dagbft/service.go` - Service wrapper that needs to pass host/pubsub (line 130)
2. `cmd/accumulated/run/dagbft.go` - DAGBFTService that creates the service (line 265-272)
3. `pkg/consensus/consensus.go` - NewNode function that accepts host/pubsub (line 112)
4. `pkg/consensus/gossip/gossip.go` - GossipLayer implementation (line 79)
5. `pkg/api/v3/p2p/p2p.go` - Existing p2p.Node with private host (line 39)

### Reference Implementation
- `cmd/consensus-testnet/main.go` - Working example of host/GossipSub creation (lines 196-260)

## Open Questions

1. **Should we expose the host from p2p.Node or create a new host?**
   - Option A: Add a `Host()` method to `p2p.Node` to expose the existing host
   - Option B: Create a dedicated libp2p host just for consensus (like consensus-testnet does)
   - Option A is preferred to avoid running multiple hosts

2. **Should consensus GossipSub be shared with existing discovery GossipSub?**
   - The existing `discovery.go` creates its own GossipSub at line 64
   - Consensus uses different topic patterns (`acc/%s/consensus/*`)
   - Multiple GossipSub instances on the same host should work but may not be ideal

3. **How should ServiceConfig be extended?**
   - Need to add `Host host.Host` and `PubSub *pubsub.PubSub` fields
   - Or pass them through NodeConfig

## Contradictions

None found. The code is consistent in:
- Expecting host/pubsub for multi-validator mode
- Working with nil gossip layer for single-validator/testing mode
- Using the same gossip.GossipLayer interface throughout

## Implementation Approach (Recommended)

1. **Add Host() method to p2p.Node** (`pkg/api/v3/p2p/p2p.go`):
   ```go
   func (n *Node) Host() host.Host { return n.host }
   ```

2. **Extend ServiceConfig** (`internal/node/dagbft/service.go`):
   ```go
   type ServiceConfig struct {
       // ... existing fields ...
       Host   host.Host
       PubSub *pubsub.PubSub
   }
   ```

3. **Create GossipSub in DAGBFTService.start()** (`cmd/accumulated/run/dagbft.go`):
   ```go
   // Get the host from p2p node
   host := inst.p2p.Host()

   // Create GossipSub for consensus
   ps, err := pubsub.NewGossipSub(inst.context, host)
   if err != nil {
       return errors.UnknownError.WithFormat("create gossipsub: %w", err)
   }

   // Pass to ServiceConfig
   s.service, err = dagbft.NewService(dagbft.ServiceConfig{
       // ... existing fields ...
       Host:   host,
       PubSub: ps,
   })
   ```

4. **Pass host/pubsub to NewNode** (`internal/node/dagbft/service.go:130`):
   ```go
   s.node, err = consensus.NewNode(nodeConfig, committee, s.config.Host, s.config.PubSub)
   ```
