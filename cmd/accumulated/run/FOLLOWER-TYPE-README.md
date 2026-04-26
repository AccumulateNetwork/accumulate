# Follower Configuration Type

## Overview

The `follower` configuration type allows running an Accumulate node as a non-validator follower that:
- **Does NOT sign blocks** (voting_power = 0)
- **Uses transient validator keys** (generated randomly on each startup)
- **Follows the network consensus** without participating in block production

## Implementation

### Schema Changes (`schema.yml`)

Added `Follower` to `ConfigurationType` enum:
```yaml
values:
  CoreValidator:
    value: 1
  Gateway:
    value: 2
  Devnet:
    value: 3
  Follower:
    value: 4
```

Added `FollowerConfiguration` member with same fields as `CoreValidatorConfiguration` except NO `ValidatorKey` field.

### Code Changes

**`follower.go`** (new file)
- Implements `(*FollowerConfiguration).apply()` method
- Creates `ConsensusService` with `TransientPrivateKey` for ValidatorKey
- Identical to CoreValidator except uses transient key

**`consensus.go`** (modified)
- Updated `loadPrivVal()` to detect and allow `TransientPrivateKey`
- Transient keys won't be in genesis → voting_power=0 → follower mode

## Architecture

```
Config type="follower" 
  → TransientPrivateKey (random on startup)
  → CometBFT checks key against state.db validator set
  → Key NOT in validator set
  → voting_power = 0
  → Node follows but doesn't sign
```

## Configuration Example

```toml
dot-env = false
network = "MainNet"

[[configurations]]
  type = "follower"
  mode = "dual"
  bvn = "Cyclops"
  bvn-bootstrap-peers = []
  bvn-genesis = "cyclops-genesis.snap"
  dn-bootstrap-peers = []
  dn-genesis = "directory-genesis.snap"
  enable-healing = false
  enable-snapshots = false
  listen = "/ip4/0.0.0.0/tcp/16591"
  storage-type = "levelDB"

[logging]
  format = "plain"
  [[logging.rules]]
    level = "INFO"
```

## Validation

Run with follower config:
```bash
docker run -d --name accumulate_follower \
  -p 16591:16591 -p 16592:16592 -p 16593:16593 \
  -p 16691:16691 -p 16692:16692 -p 16693:16693 \
  -v /path/to/data:/node \
  accumulated:follower-type \
  run-dual /node/dnn /node/bvnn
```

Check status:
```bash
curl http://localhost:16595/status | jq '.result.validator_info.voting_power'
# Should return: "0"
```

## Key Benefits

1. **No validator key management** - Uses transient keys
2. **Safe for testing** - Cannot accidentally sign blocks
3. **Permissionless** - No registration needed
4. **Same databases** - Uses identical blockchain data as validators

## Files Modified

- `cmd/accumulated/run/schema.yml` - Added Follower type
- `cmd/accumulated/run/follower.go` - New implementation
- `cmd/accumulated/run/consensus.go` - Allow transient keys
- `cmd/accumulated/run/types_gen.go` - Generated types
- `cmd/accumulated/run/schema_gen.go` - Generated schema

## Testing

Verified with:
- ✅ Node starts without errors
- ✅ voting_power = 0 confirmed  
- ✅ Type shows as "Follower" not "Validator"
- ✅ TransientPrivateKey accepted
- ✅ No panics or crashes

## Deployment Guide

For complete follower deployment instructions, see:
- **[Follower Deployment Guide](../../../docs/operations/deploying-follower.md)** - Step-by-step setup
- **[Bootstrap Peers](../../../docs/operations/bootstrap-peers.md)** - Network peer configuration
- **[convert-node-id Tool](../../../tools/cmd/convert-node-id/README.md)** - Peer ID conversion

## Future Improvements

1. Implement fast-sync/state-sync for faster catchup
2. Add follower-specific metrics and monitoring
3. Automatic peer discovery mechanisms

