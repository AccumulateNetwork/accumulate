# Bootstrap Peers for Accumulate Networks

## Overview

Bootstrap peers are validators your follower node connects to for P2P block gossip. Configuring reliable bootstrap peers is **critical** for successful follower deployment.

**Best Practices:**
- Configure at least **2-3 bootstrap peers** per partition
- Use validators with high uptime
- Mix geographic locations for redundancy
- Verify peer IDs are current (validators may rotate keys)

---

## MainNet Bootstrap Peers

### BVN0 - Cyclops (Production Validator)

**Operator:** Accumulate Network
**IP:** `23.22.212.106`
**Status:** ✅ Active, High Uptime
**Location:** US East

**CometBFT Node ID:**
```
3029240e829e58e399bc7b6115bb6bc947cc24c7
```

**libp2p Peer IDs (converted):**
```
QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD
```

**Configuration:**
```toml
# Directory Network (DN)
dn-bootstrap-peers = [
  "/ip4/23.22.212.106/tcp/16591/p2p/QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD"
]

# Block Validator Network (BVN - Cyclops)
bvn-bootstrap-peers = [
  "/ip4/23.22.212.106/tcp/16691/p2p/QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD"
]
```

### Public Validator - mainnet1

**IP:** `144.76.105.23`
**Status:** ⚠️  Intermittent (verify before using)
**Location:** Europe

**CometBFT Node IDs:**
- DN: `ebb29bee942723271a39217bd0ed62f7827245de`
- BVN: `ba238200737bad88d4e9407fec6858fdc05d6dca`

**libp2p Peer IDs (converted):**
- DN: `QmeCiUsPegJhXHFrYh3uyGS94iLJXLvGNLfBzAiF3BgCLo`
- BVN: `QmasFwjwjj8CrkMRugDECTypUAjR6M6oxU7ncLkgiW5L15`

**Configuration:**
```toml
# Directory Network (DN)
dn-bootstrap-peers = [
  "/ip4/144.76.105.23/tcp/16591/p2p/QmeCiUsPegJhXHFrYh3uyGS94iLJXLvGNLfBzAiF3BgCLo"
]

# Block Validator Network (BVN - Cyclops)
bvn-bootstrap-peers = [
  "/ip4/144.76.105.23/tcp/16691/p2p/QmasFwjwjj8CrkMRugDECTypUAjR6M6oxU7ncLkgiW5L15"
]
```

**Note:** This peer has shown intermittent connectivity issues. Use BVN0 as primary.

### Recommended Multi-Peer Configuration

```toml
[[configurations]]
  type = "follower"
  mode = "dual"
  bvn = "Cyclops"

  # DN bootstrap peers (use multiple for redundancy)
  dn-bootstrap-peers = [
    "/ip4/23.22.212.106/tcp/16591/p2p/QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD",
    "/ip4/144.76.105.23/tcp/16591/p2p/QmeCiUsPegJhXHFrYh3uyGS94iLJXLvGNLfBzAiF3BgCLo"
  ]

  # BVN bootstrap peers (use multiple for redundancy)
  bvn-bootstrap-peers = [
    "/ip4/23.22.212.106/tcp/16691/p2p/QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD",
    "/ip4/144.76.105.23/tcp/16691/p2p/QmasFwjwjj8CrkMRugDECTypUAjR6M6oxU7ncLkgiW5L15"
  ]
```

---

## TestNet Bootstrap Peers

### Kermit TestNet

**Contact the Accumulate team for current Kermit bootstrap peer information.**

**General Configuration Format:**
```toml
network = "Kermit"

[[configurations]]
  type = "follower"
  mode = "dual"
  bvn = "Cyclops"

  dn-bootstrap-peers = [
    # Add Kermit DN validators here
  ]

  bvn-bootstrap-peers = [
    # Add Kermit BVN validators here
  ]
```

---

## DevNet Bootstrap Peers

For local development networks, bootstrap peers are typically localhost:

```toml
network = "DevNet"

[[configurations]]
  type = "follower"
  mode = "dual"
  bvn = "BVN0"

  dn-bootstrap-peers = [
    "/ip4/127.0.0.1/tcp/16591/p2p/<local-dn-peer-id>"
  ]

  bvn-bootstrap-peers = [
    "/ip4/127.0.0.1/tcp/16691/p2p/<local-bvn-peer-id>"
  ]
```

---

## How to Find Bootstrap Peers

### Method 1: Query Explorer

Visit the [Accumulate Explorer](https://explorer.accumulatenetwork.io/) to view active validators.

### Method 2: Query Existing Validator

```bash
# Get validator's peer information
curl -s http://validator-ip:16592/net_info | jq '.result.peers[]'
```

### Method 3: Check Address Book

If you have access to a running node:
```bash
cat /var/lib/accumulate/dnn/config/addrbook.json | jq '.addrs'
```

### Method 4: Community Resources

- Discord: Ask in the #validators channel
- Forum: Check pinned posts for current peer lists
- GitLab: Check network-map repository

---

## Converting Node IDs

When you find a CometBFT node ID, convert it to libp2p format:

```bash
# Example: Convert BVN0's node ID
cd ~/accumulate
go run ./tools/cmd/convert-node-id 3029240e829e58e399bc7b6115bb6bc947cc24c7

# Output: QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD
```

See [convert-node-id README](../../tools/cmd/convert-node-id/README.md) for details.

---

## Verifying Bootstrap Peer Connectivity

### Before Deployment

Test connectivity to bootstrap peers:

```bash
# Check port reachability
nc -zv 23.22.212.106 16591  # DN
nc -zv 23.22.212.106 16691  # BVN

# Expected output: "Connection to 23.22.212.106 16591 port [tcp/*] succeeded!"
```

### After Deployment

Check if your follower connected successfully:

```bash
# Check peer count
curl -s http://localhost:16592/net_info | jq '.result.n_peers'
# Expected: > 0

# View connected peers
curl -s http://localhost:16592/net_info | jq '.result.peers[] | {
  moniker: .node_info.moniker,
  remote_ip: .remote_ip,
  id: .node_info.id
}'
```

**Expected Result:**
```json
{
  "moniker": "76-fun",
  "remote_ip": "23.22.212.106",
  "id": "3029240e829e58e399bc7b6115bb6bc947cc24c7"
}
```

---

## Port Reference

### Directory Network (DN)

| Port | Protocol | Purpose |
|------|----------|---------|
| 16591 | TCP | P2P Gossip (bootstrap peer port) |
| 16592 | TCP | CometBFT RPC |
| 16593 | TCP | Accumulate API |

### Block Validator Network (BVN)

| Port | Protocol | Purpose |
|------|----------|---------|
| 16691 | TCP | P2P Gossip (bootstrap peer port) |
| 16692 | TCP | CometBFT RPC |
| 16693 | TCP | Accumulate API |

**Note:** Bootstrap peer configuration uses **P2P ports** (16591/16691), not RPC ports.

---

## Multiaddr Format Explained

Bootstrap peers use libp2p multiaddr format:

```
/ip4/<ip-address>/tcp/<port>/p2p/<peer-id>
```

**Components:**
- `/ip4/` - IPv4 address (IPv6 would be `/ip6/`)
- `<ip-address>` - Validator's IP (e.g., 23.22.212.106)
- `/tcp/` - TCP protocol
- `<port>` - P2P port (16591 for DN, 16691 for BVN)
- `/p2p/` - libp2p protocol
- `<peer-id>` - Base58-encoded multihash peer ID

**Example:**
```
/ip4/23.22.212.106/tcp/16591/p2p/QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD
└─┬─┘└──────┬──────┘└─┬─┘└─┬──┘└─┬─┘└──────────────────┬──────────────────┘
  │         │         │    │     │                      │
  │      IP Addr    Proto Port  │                   Peer ID
  │                              │
 IPv4                          P2P
```

---

## Troubleshooting Bootstrap Peers

### Problem: No Peers Connected

**Diagnosis:**
```bash
curl -s http://localhost:16592/net_info | jq '.result.n_peers'
# Output: "0"
```

**Causes & Solutions:**

**1. Wrong Peer ID Format**
```bash
# Verify you used libp2p format (starts with Qm...)
# NOT CometBFT hex format
```
Solution: Re-convert with `convert-node-id` tool

**2. Port Not Reachable**
```bash
nc -zv <peer-ip> <peer-port>
```
Solution: Check firewall, verify IP/port are correct

**3. Peer Offline**
```bash
curl -s http://<peer-ip>:16592/status
```
Solution: Use different bootstrap peer

**4. Wrong Network**
```bash
# MainNet peer won't work for TestNet
```
Solution: Use peers from correct network

### Problem: Peer Connected But Not Syncing

**Diagnosis:**
```bash
curl -s http://localhost:16592/net_info | jq '.result.peers[] | {
  moniker,
  height: .node_info.other.height
}'
```

**If height is null:**
- Peer is also a follower (not a validator)
- Peer is not reporting height
- Solution: Add validator bootstrap peers

**If height is behind:**
- Peer is syncing itself
- Solution: Wait or use different peer

### Problem: Only One Peer Connects

**Configuration:**
```toml
dn-bootstrap-peers = [
  "/ip4/23.22.212.106/tcp/16591/p2p/QmRaef...",
  "/ip4/144.76.105.23/tcp/16591/p2p/QmeCiUs..."
]
```

**Check which peer connected:**
```bash
curl -s http://localhost:16592/net_info | jq '.result.peers[].remote_ip'
```

**If only one connects:**
- Other peer may be down
- Network filtering
- Peer may have connection limits

**Action:** As long as ONE peer connects and is syncing, follower will work. Add more peers for redundancy.

---

## Contributing Bootstrap Peers

If you operate a validator and want to be listed as a public bootstrap peer:

1. Verify your validator has high uptime
2. Confirm P2P ports (16591/16691) are publicly accessible
3. Provide:
   - IP address
   - CometBFT node IDs (DN and BVN)
   - Converted libp2p peer IDs
   - Geographic location
   - Contact information

4. Submit a merge request updating this document

---

## Security Considerations

### Bootstrap Peer Trust

- Bootstrap peers can influence which chain your follower tracks
- Use well-known validators operated by trusted entities
- Multiple bootstrap peers reduce single-point-of-trust
- Verify peer IDs from multiple sources

### Running Your Own Validator as Bootstrap

Most secure option: Run your own validator and use it as bootstrap peer for your followers.

### Public vs Private Bootstrap Peers

**Public:**
- Anyone can connect
- Higher bandwidth usage
- Contributes to network decentralization

**Private:**
- Firewall limits connections
- Lower resource usage
- Less network contribution

---

## Additional Resources

- **Follower Deployment:** [deploying-follower.md](deploying-follower.md)
- **convert-node-id Tool:** [../../tools/cmd/convert-node-id/README.md](../../tools/cmd/convert-node-id/README.md)
- **Follower Configuration:** [../../cmd/accumulated/run/FOLLOWER-TYPE-README.md](../../cmd/accumulated/run/FOLLOWER-TYPE-README.md)
- **Network Explorer:** https://explorer.accumulatenetwork.io/
- **Community Discord:** [Accumulate Discord]

---

*Last Updated: 2025-10-13*

**Note:** Bootstrap peer information may change as validators are added/removed. Check this document regularly for updates.
