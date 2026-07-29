# P2P Reliability Plan

This document outlines improvements needed to make Accumulate's P2P systems reliable.

## Current Problems

### CometBFT P2P Issues

1. **Hardcoded/stale seed addresses** - Seeds in distribution packages become stale when validators go offline
2. **No automatic validator discovery** - Followers must be manually configured with active validators
3. **Address book corruption** - Failed dial attempts persist and cause backoff loops
4. **No health checking of seeds** - Node doesn't verify seeds are actually synced before using them

### libp2p Issues

1. **Single bootstrap server** - No redundancy if bootstrap.accumulate.defidevs.io goes down
2. **No dynamic peer list** - Followers can't discover active validators dynamically
3. **Disconnect between systems** - libp2p and CometBFT P2P don't share peer information

## Proposed Solutions

### Phase 1: Immediate Fixes (v1.4.x)

#### 1.1 Seed Health Validation
Before using a seed, verify it's actually synced:

```go
func validateSeed(addr string) bool {
    // Parse seed address
    parts := strings.Split(addr, "@")
    if len(parts) != 2 {
        return false
    }

    // Query the seed's status
    resp, err := http.Get(fmt.Sprintf("http://%s/status",
        strings.Replace(parts[1], ":16591", ":16592", 1)))
    if err != nil {
        return false
    }

    // Check if block time is recent (within last hour)
    var status struct {
        Result struct {
            SyncInfo struct {
                LatestBlockTime time.Time `json:"latest_block_time"`
            } `json:"sync_info"`
        } `json:"result"`
    }
    json.NewDecoder(resp.Body).Decode(&status)

    return time.Since(status.Result.SyncInfo.LatestBlockTime) < time.Hour
}
```

**File:** `cmd/accumulated/run/consensus.go`

#### 1.2 Dynamic Seed Fallback
If configured seeds are unhealthy, query the bootstrap server for active nodes:

```go
func getActiveValidators() ([]string, error) {
    resp, err := http.Get("http://bootstrap.accumulate.defidevs.io:8080/peers")
    if err != nil {
        return nil, err
    }
    // Parse peers and return as CometBFT seed format
    // ...
}
```

#### 1.3 Address Book Auto-Cleanup
Periodically clean stale entries from the address book:

```go
func cleanAddressBook(path string, maxAttempts int) error {
    // Read address book
    // Remove entries with attempts > maxAttempts and no recent success
    // Write back
}
```

### Phase 2: Infrastructure Improvements

#### 2.1 Multiple Bootstrap Servers
Deploy bootstrap servers in multiple regions:
- `bootstrap-us.accumulate.defidevs.io`
- `bootstrap-eu.accumulate.defidevs.io`
- `bootstrap-asia.accumulate.defidevs.io`

#### 2.2 Validator Registry
Create an on-chain or off-chain registry of active validators:

```go
type ValidatorEntry struct {
    NodeID      string    `json:"node_id"`
    DNAddress   string    `json:"dn_address"`   // ip:port for DN P2P
    BVNAddress  string    `json:"bvn_address"`  // ip:port for BVN P2P
    LastSeen    time.Time `json:"last_seen"`
    BlockHeight uint64    `json:"block_height"`
}
```

Options:
- Store in Accumulate data account (`acc://validators.acme/registry`)
- Store in bootstrap server's DHT
- External service with DNS-based discovery

#### 2.3 Bootstrap Server Enhancements

Add CometBFT peer information to the bootstrap server:

```go
// New endpoint: /cometbft-peers
type CometBFTPeer struct {
    Network   string `json:"network"`      // "MainNet.Directory" or "MainNet.Cyclops"
    NodeID    string `json:"node_id"`      // 40-char hex
    Address   string `json:"address"`      // ip:port
    Height    uint64 `json:"height"`
    LastBlock time.Time `json:"last_block"`
}

func (s *InfoServer) handleCometBFTPeers(w http.ResponseWriter, r *http.Request) {
    // Query known validators for their CometBFT status
    // Return list of healthy validators
}
```

### Phase 3: Automatic Discovery

#### 3.1 Validator Self-Registration
Validators periodically register with the bootstrap server:

```go
func (v *Validator) registerWithBootstrap() {
    payload := ValidatorEntry{
        NodeID:      v.NodeID(),
        DNAddress:   v.DNP2PAddress(),
        BVNAddress:  v.BVNP2PAddress(),
        BlockHeight: v.CurrentHeight(),
    }

    http.Post("http://bootstrap.accumulate.defidevs.io:8080/register",
        "application/json",
        json.Marshal(payload))
}
```

#### 3.2 Follower Auto-Configuration
Followers automatically discover and configure seeds:

```go
func (f *Follower) autoConfigureSeeds() error {
    // 1. Query bootstrap server for active validators
    validators, err := getActiveValidators()
    if err != nil {
        return err
    }

    // 2. Validate each validator is healthy
    healthy := []string{}
    for _, v := range validators {
        if validateSeed(v) {
            healthy = append(healthy, v)
        }
    }

    // 3. Update tendermint.toml with healthy seeds
    return updateSeeds(healthy)
}
```

## Implementation Priority

| Priority | Task | Complexity | Impact |
|----------|------|------------|--------|
| P0 | Seed health validation | Low | High |
| P0 | Fix unconditional_peer_ids format | Done | High |
| P1 | Address book auto-cleanup | Low | Medium |
| P1 | Multiple bootstrap servers | Medium | High |
| P2 | CometBFT peer endpoint in bootstrap | Medium | High |
| P2 | Validator self-registration | Medium | High |
| P3 | On-chain validator registry | High | Medium |

## Monitoring & Alerting

### Metrics to Track
- `accumulate_p2p_peers_count` - Number of connected CometBFT peers
- `accumulate_libp2p_peers_count` - Number of connected libp2p peers
- `accumulate_blocks_behind` - How far behind mainnet the node is
- `accumulate_last_block_age_seconds` - Age of the latest block

### Alerts
- Peer count drops to 0
- Block age exceeds 5 minutes
- Bootstrap server unreachable
- Validator stops producing blocks

## Testing Plan

### Unit Tests
- Seed validation logic
- Address book cleanup
- Peer address parsing

### Integration Tests
- Follower syncs with healthy validator
- Follower recovers when primary validator goes down
- Address book recovery after network partition

### Chaos Testing
- Kill random validators
- Network partition between regions
- Bootstrap server failure
- DNS resolution failure

## Open Questions

1. Should we use DNS-based seed discovery (like `_seed._tcp.accumulate.network`)?
2. How to handle validator key rotation affecting node IDs?
3. Should followers be able to serve as seeds for other followers?
4. How to prevent malicious nodes from poisoning the validator registry?
