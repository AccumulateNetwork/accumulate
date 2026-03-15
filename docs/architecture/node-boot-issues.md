# Node Boot Issues Research

**Issue**: Multi-node network deployment problems
**Date**: March 2026
**Status**: Research

---

## Executive Summary

Devnet works easily because it "cheats" - all peers are known at genesis, shared files, localhost only. Real multi-node deployment faces several unresolved issues.

---

## How Devnet Cheats

| Aspect | Devnet | Production |
|--------|--------|------------|
| Peer discovery | Pre-built lists | DHT/bootstrap |
| Genesis | Shared file on disk | Must match across nodes |
| Network | Localhost (127.0.1.1) | Real network with NAT/firewall |
| Bootstrap | Single dedicated node | Multiple bootstrap servers |
| State sync | Not needed (fresh genesis) | Required for joining nodes |

**Devnet shortcuts** (`cmd/accumulated/run/devnet.go`):
- Line 124-141: `dnPeers` and `bvnPeers` arrays built from complete node list
- Line 436-441: Hardcoded bootstrap peer address injected
- Line 307-349: Single bootstrap node coordinates everything
- Line 144-276: Genesis files written to shared location

---

## Known Issues

### 1. Bootstrap Peer ID Mismatch

**File**: `pkg/accumulate/api.go:18`

The hardcoded bootstrap peer ID can become stale. Recent fix (commit fb9867e3a) corrected:
```
Old: 12D3KooWGJTh4aeF7bFnwo9sAYRujCkuVU1Cq8wNeTNGpFgZgXdg
New: 12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx
```

**Impact**: Followers cannot connect to bootstrap server with wrong ID.

### 2. CometBFT Bootstrap Peer Limitation

**File**: `cmd/accumulated/run/consensus.go:174-185`

```go
// Initial peers (should be bootstrap peers but that setting isn't
// present in 0.37)
for i, peer := range c.BootstrapPeers {
    ...
    d.config.P2P.PersistentPeers += id
}
```

CometBFT 0.37 lacks proper `BootstrapPeers` config. Workaround uses `PersistentPeers`, which:
- Requires knowing all peers at startup
- Doesn't support dynamic peer discovery
- Breaks when peers change

### 3. Private Validator State Hack

**File**: `cmd/accumulated/run/consensus.go:270-279`

```go
// This is a hack to work around CometBFT
pv := tmpv.NewFilePV(key2, "", config.PrivValidatorStateFile())
```

FilePV handling is hacky, which can cause issues with validator state persistence across restarts.

### 4. Anchor Healing Disabled

**File**: `cmd/accumulated/run/consensus.go:504-505`

```go
// TODO Fix the flooding issues and enable this by default
EnableAnchorHealing: Ptr(false),
```

Anchor healing (cross-partition recovery) is disabled due to "flooding issues". This affects network resilience.

### 5. Manual Snapshot Trigger Hack

**File**: `cmd/accumulated/run/snapshot.go:255-258`

```go
// This is a hack to manually trigger a snapshot
if st, err := os.Stat(filepath.Join(c.directory, ".capture")); err == nil && !st.IsDir() {
    return true
}
```

Snapshot capture requires creating a `.capture` file. No proper API/signal mechanism.

### 6. State Sync / Snapshots Broken

**File**: `internal/node/abci/snapshot.go:153`

As documented in consensus research, CometBFT's snapshot restore fails due to state hash mismatch. Without working snapshots:
- New nodes must sync from genesis
- Cannot enable `RetainHeight` pruning
- Blockstore grows forever

### 7. DHT Bootstrap Failures Silent

**File**: `pkg/api/v3/p2p/discovery.go:44-47`

DHT bootstrap errors are logged but don't prevent node startup:
```go
if err := d.Bootstrap(ctx); err != nil {
    slog.ErrorContext(ctx, "DHT bootstrap failed", "error", err)
}
```

Node can start isolated if all bootstrap peers unreachable.

### 8. Genesis Consistency Requirements

Multi-partition genesis must execute identically on all nodes:
- Routing table built from network definition
- Validator set must match
- Operator keys must be identical
- Any mismatch causes routing failures or consensus splits

---

## Deployment Failure Modes

### Symptom: Node Can't Find Peers

**Causes**:
1. Wrong bootstrap peer ID in binary
2. Bootstrap server unreachable
3. NAT/firewall blocking DHT
4. `PersistentPeers` list outdated

**Diagnosis**:
```bash
# Check if bootstrap server is reachable
curl -v bootstrap.accumulate.defidevs.io:16593
# Check peer ID matches
accumulated p2p info
```

### Symptom: Node Stuck at Genesis

**Causes**:
1. Genesis file mismatch between nodes
2. State sync not working
3. No peers to sync from

**Diagnosis**:
```bash
# Compare genesis hashes
sha256sum /path/to/genesis.json
# Check peer count
curl localhost:26657/net_info | jq '.result.n_peers'
```

### Symptom: Consensus Stall

**Causes**:
1. Validator key mismatch
2. Insufficient validators online
3. Clock skew between nodes
4. Network partition

---

## Recommended Fixes

### Short Term

1. **Fix bootstrap peer ID management**
   - Don't hardcode peer IDs
   - Fetch from DNS TXT record or well-known endpoint
   - Or: Use multiple bootstrap servers

2. **Make DHT bootstrap failure fatal** (optional)
   - Currently silently fails
   - At minimum, warn loudly in logs

3. **Add health check endpoint**
   - Report peer count, sync status, last block
   - Make deployment monitoring easier

### Medium Term

1. **Fix CometBFT state sync**
   - Investigate hash mismatch root cause
   - Enable `RetainHeight` pruning

2. **Implement proper bootstrap protocol**
   - Fetch genesis from bootstrap server
   - Verify genesis hash before starting
   - Download initial state snapshot

3. **Fix anchor healing flooding**
   - Enable `EnableAnchorHealing` by default
   - Needed for network resilience

### Long Term

1. **Replace CometBFT** (per consensus research)
   - Eliminates many of these issues
   - Custom DAG consensus with native peer discovery

2. **BPT-centric sync** (per consensus research)
   - State sync via BPT snapshots
   - No dependency on CometBFT snapshots

---

## References

- `cmd/accumulated/run/consensus.go` - Node startup, peer config
- `cmd/accumulated/run/devnet.go` - Devnet shortcuts
- `cmd/accumulated/run/snapshot.go` - Snapshot handling
- `pkg/accumulate/api.go` - Bootstrap server config
- `pkg/api/v3/p2p/discovery.go` - DHT peer discovery
- `internal/node/abci/snapshot.go` - ABCI snapshot handlers
- `docs/architecture/consensus-and-state-optimization.md` - Related research
