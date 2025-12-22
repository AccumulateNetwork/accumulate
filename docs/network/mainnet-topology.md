# Accumulate MainNet Topology - November 17, 2025

**Investigation Date**: 2025-11-17
**Network**: MainNet
**Active Nodes**: 2 confirmed

---

## Network Nodes

### 1. apollo-mainnet (Primary Validator)
- **Hostname**: apollo-mainnet.accumulate.defidevs.io
- **IP Address**: 23.22.212.106
- **Role**: Core Validator (Cyclops BVN)
- **CometBFT Version**: v0.38.0-rc3
- **Moniker**: "76-fun"

#### Directory Network (DN)
- **CometBFT Node ID**: `3029240e829e58e399bc7b6115bb6bc947cc24c7`
- **P2P Listen**: 16591
- **RPC**: 16592
- **libp2p**: 16593
- **API**: 16595
- **Connected Peers**: 1 (mainnet1)

#### BVN Cyclops
- **CometBFT Node ID**: `3029240e829e58e399bc7b6115bb6bc947cc24c7`
- **P2P Listen**: 16691
- **RPC**: 16692
- **libp2p**: 16693
- **API**: 16695
- **Connected Peers**: 0

**Configuration Notes**:
- Type: coreValidator
- Storage: levelDB
- Bootstrap peers: EMPTY (configured with no bootstrap peers)
- Healing: disabled
- Snapshots: disabled

---

### 2. mainnet1 (Secondary Node)
- **IP Address**: 144.76.105.23
- **Role**: Unknown (likely validator or follower)
- **CometBFT Version**: v0.38.0-rc3
- **Moniker**: "mainnet1"

#### Directory Network (DN)
- **CometBFT Node ID**: `ebb29bee942723271a39217bd0ed62f7827245de`
- **P2P Listen**: 16591
- **RPC**: 16592
- **API**: 16595
- **Connected Peers**: 1 (apollo-mainnet)

#### BVN Partition
- **Connected Peers**: 0 (no BVN peers connected)

**Configuration Notes**:
- Running CometBFT on standard ports
- Successfully connected to apollo-mainnet on DN partition
- BVN partition isolated (no peers)

---

### 3. Bootstrap Server
- **Hostname**: bootstrap.accumulate.defidevs.io
- **IP Address**: 3.138.61.111
- **Role**: libp2p Bootstrap/DHT Server
- **Status**: Running (accessible on port 16593)

**libp2p Peer IDs**:
- **Actual ID**: `12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx`
- **Stale ID** (cached somewhere): `12D3KooWGJTh4aeF7bFnwo9sAYRujCkuVU1Cq8wNeTNGpFgZgXdg`

**Services**:
- **libp2p DHT**: 16593 (active)
- **CometBFT**: NOT running on standard ports
- **Purpose**: Peer discovery for new P2P system, not consensus

**Configuration Notes**:
- Dedicated bootstrap server for libp2p peer discovery
- Does not participate in CometBFT consensus
- Peer ID mismatch indicates stale configuration in some bootstrap client code

---

## Port Allocation Standard

### Directory Network (DN)
- **16591**: CometBFT P2P
- **16592**: CometBFT RPC
- **16593**: libp2p (new P2P system)
- **16595**: Accumulate API (v2/v3)

### BVN Partitions (Cyclops +100 offset)
- **16691**: CometBFT P2P
- **16692**: CometBFT RPC
- **16693**: libp2p (new P2P system)
- **16695**: Accumulate API (v2/v3)

---

## P2P Architecture

### CometBFT P2P (Consensus Layer)
- **Protocol**: Tendermint P2P
- **Version**: CometBFT v0.38.0-rc3
- **Connection**: Direct peer-to-peer
- **Ports**: 16591 (DN), 16691 (BVN)
- **Status**: apollo ↔ mainnet1 connected on DN only

### libp2p (Application Layer)
- **Protocol**: libp2p with Kademlia DHT
- **Bootstrap Server**: 3.138.61.111:16593
- **Ports**: 16593 (DN), 16693 (BVN)
- **Status**: Operational for new P2P system
- **Discovery Mode**: DHT auto-server

**Important**: The bootstrap-peers configuration in `accumulate.toml` uses libp2p multiaddrs (port 16593/16693) for the NEW P2P system, NOT for CometBFT persistent peers (port 16591/16691).

---

## Follower Deployment Status

### Local Follower Node
- **Location**: /home/paul/accumulate-follower
- **Container**: accumulate-follower (Docker)
- **Image**: accumulated:follower-p2p-fix
- **Status**: Running (health: starting)
- **Configuration**: New-style config (accumulate.toml)

#### Current State
- ✅ Both partitions (DN + Cyclops) initialized from genesis
- ✅ Follower mode active (voting_power=0)
- ✅ ABCI, consensus, RPC services running
- ✅ Transient P2P key generated (follower mode)
- ⚠️ No CometBFT peers connected (protocol error)
- ⚠️ libp2p bootstrap peer ID mismatch

#### Issues Identified

**1. CometBFT P2P Connection Failure**
```
ERROR Error dialing peer err="auth failure: secret conn failed: proto: illegal wireType 7"
```
- **Cause**: CometBFT attempting to connect to libp2p ports (16593/16693) instead of CometBFT P2P ports (16591/16691)
- **Impact**: Follower cannot sync blocks from validators
- **Resolution**: Configure CometBFT persistent peers separately from libp2p bootstrap peers

**2. libp2p Bootstrap Peer ID Mismatch**
```
Unable to connect to bootstrap peer error="failed to dial 12D3KooWGJTh4aeF7bFnwo9sAYRujCkuVU1Cq8wNeTNGpFgZgXdg:
peer id mismatch: expected 12D3KooWGJTh4aeF7bFnwo9sAYRujCkuVU1Cq8wNeTNGpFgZgXdg,
but remote key matches 12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx"
```
- **Cause**: Stale peer ID cached somewhere in codebase or configuration
- **Actual ID**: `12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx`
- **Expected ID**: `12D3KooWGJTh4aeF7bFnwo9sAYRujCkuVU1Cq8wNeTNGpFgZgXdg`
- **Impact**: Cannot discover peers via bootstrap server
- **Resolution**: Update bootstrap peer ID in code or clear peer cache

---

## Genesis Information

**Genesis Date**: July 13, 2025

### Files Used for Follower
- **DN Genesis**: directory-genesis.snap (2.0 MB)
- **BVN Genesis**: cyclops-genesis.snap (2.1 GB)
- **Source**: /media/paul/Expansion/databases/2025-10-01-aws-mainnet-bvn0/

**Validator Key**: priv_validator_key.json (CometBFT validator key)
- **Type**: cometPrivValFile
- **Usage**: Non-validating (voting_power=0 assigned by CometBFT)

---

## Configuration Analysis

### Working: New-Style Config (accumulate.toml)
The follower successfully uses the new-style configuration system:
- Single `accumulate.toml` file in work directory
- Dual-mode configuration (DN + BVN in one config)
- Genesis files specified as relative paths
- Properly detected and loaded by `run-dual` command

### Issue: Bootstrap Peers vs Persistent Peers
The current configuration conflates two different P2P systems:

**libp2p Bootstrap Peers** (NEW P2P system):
```toml
dn-bootstrap-peers = ["/dns/apollo-mainnet.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWPs19932secARrxoRR5J8ZtBMt2vqwyHH1Q9p8thYP7cn"]
```
- Port: 16593 (libp2p)
- Format: multiaddr
- Purpose: Peer discovery for new P2P layer

**CometBFT Persistent Peers** (CONSENSUS layer):
- Port: 16591 (CometBFT P2P)
- Format: `<node-id>@<host>:<port>`
- Purpose: Block synchronization and consensus
- **Problem**: Currently being auto-generated from libp2p bootstrap peers with wrong ports

---

## Network Topology Diagram

```
[Bootstrap Server]          [apollo-mainnet]           [mainnet1]
3.138.61.111               23.22.212.106              144.76.105.23

libp2p DHT                 Validator "Cyclops"        Secondary Node
:16593                     :16591 (CometBFT DN)       :16591 (CometBFT DN)
                           :16691 (CometBFT BVN)
                                    ↕ (DN peers)
                                    ↕
                           [mainnet1 DN]              [apollo DN]
                           ebb29bee...                3029240e...


[Follower - Local]
localhost
:16591-16693

Status: Running
DN Peers: 0 (cannot connect)
BVN Peers: 0 (cannot connect)
Issue: Wrong port configuration
```

---

## Required Fixes

### 1. Separate CometBFT Persistent Peers Configuration
The `accumulate.toml` needs separate configuration for:
- libp2p bootstrap peers (application layer, ports 16593/16693)
- CometBFT persistent peers (consensus layer, ports 16591/16691)

### 2. Update Bootstrap Peer ID
Update hardcoded bootstrap peer ID from:
- Old: `12D3KooWGJTh4aeF7bFnwo9sAYRujCkuVU1Cq8wNeTNGpFgZgXdg`
- New: `12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx`

### 3. Correct Follower Bootstrap Configuration
For followers to sync properly, they need:
- **CometBFT Persistent Peers**: `3029240e829e58e399bc7b6115bb6bc947cc24c7@apollo-mainnet.accumulate.defidevs.io:16591` (DN)
- **CometBFT Persistent Peers**: `3029240e829e58e399bc7b6115bb6bc947cc24c7@apollo-mainnet.accumulate.defidevs.io:16691` (BVN)
- **libp2p Bootstrap Peers**: Keep current multiaddr configuration

---

## Next Steps

1. **Update Code**: Modify configuration loading to distinguish between libp2p and CometBFT peer lists
2. **Update accumulate.toml**: Add separate fields for CometBFT persistent peers
3. **Fix Bootstrap ID**: Update stale bootstrap server peer ID in codebase
4. **Test Connection**: Verify follower can connect to apollo on correct ports
5. **Monitor Sync**: Confirm block synchronization begins after connection

---

## References

- **Session Log**: follower_deployment_session_2025-11-16.md
- **Branch**: 3688-booting-a-follower (has proper follower type implementation)
- **Validator Config**: /var/lib/docker/volumes/acc_mainnet_bvn0/_data/accumulate.toml (on apollo)
- **Follower Config**: /home/paul/accumulate-follower/accumulate.toml
