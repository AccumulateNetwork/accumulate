# convert-node-id Tool

## Purpose

Converts CometBFT node IDs (20-byte hex format) to libp2p peer IDs (base58-encoded multihash format).

This tool is **essential for configuring bootstrap peers** in follower node deployments.

## Why This Tool Is Needed

Accumulate uses two different peer ID formats:

1. **CometBFT Format** (used in CometBFT configs):
   - 20-byte hex string
   - Example: `3029240e829e58e399bc7b6115bb6bc947cc24c7`
   - Found in: `node_key.json`, CometBFT logs, validator lists

2. **libp2p Multiaddr Format** (used in Accumulate configs):
   - Base58-encoded multihash
   - Example: `QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD`
   - Required for: `accumulate.toml` bootstrap peer configuration

**Without this tool,** you cannot properly configure bootstrap peers in your follower's `accumulate.toml`.

## Usage

### Basic Usage

```bash
go run ./tools/cmd/convert-node-id <cometbft-node-id>
```

### Example

**Input:** CometBFT node ID from BVN0 validator
```bash
go run ./tools/cmd/convert-node-id 3029240e829e58e399bc7b6115bb6bc947cc24c7
```

**Output:** libp2p peer ID
```
QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD
```

### Using in Configuration

**Step 1:** Get CometBFT node ID from validator
```bash
# From a validator's node_key.json
ssh validator-host 'cat /path/to/node_key.json' | jq -r '.id'
# Output: 3029240e829e58e399bc7b6115bb6bc947cc24c7
```

**Step 2:** Convert to libp2p format
```bash
go run ./tools/cmd/convert-node-id 3029240e829e58e399bc7b6115bb6bc947cc24c7
# Output: QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD
```

**Step 3:** Use in accumulate.toml
```toml
[[configurations]]
  type = "follower"

  # DN bootstrap peer (port 16591)
  dn-bootstrap-peers = [
    "/ip4/23.22.212.106/tcp/16591/p2p/QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD"
  ]

  # BVN bootstrap peer (port 16691)
  bvn-bootstrap-peers = [
    "/ip4/23.22.212.106/tcp/16691/p2p/QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD"
  ]
```

## Installation

### Build Binary

```bash
cd ~/accumulate
go build -o convert-node-id ./tools/cmd/convert-node-id
sudo mv convert-node-id /usr/local/bin/
```

### Use Directly

```bash
cd ~/accumulate
go run ./tools/cmd/convert-node-id <node-id>
```

## Finding CometBFT Node IDs

### From Validator's node_key.json

```bash
# On validator host
cat /var/lib/accumulate/dnn/config/node_key.json | jq -r '.id'
```

### From CometBFT Status

```bash
# Query validator's RPC endpoint
curl -s http://validator-ip:16592/status | jq -r '.result.node_info.id'
```

### From CometBFT Logs

Look for log entries like:
```
Starting node with ID=3029240e829e58e399bc7b6115bb6bc947cc24c7
```

### From Address Book

```bash
# Check validator's addrbook.json
cat /var/lib/accumulate/dnn/config/addrbook.json | jq '.addrs'
```

## Technical Details

### Conversion Algorithm

1. **Input:** 20-byte hex string (40 hex characters)
   ```
   3029240e829e58e399bc7b6115bb6bc947cc24c7
   ```

2. **Decode hex to bytes:**
   ```
   [30 29 24 0e 82 9e 58 e3 99 bc 7b 61 15 bb 6b c9 47 cc 24 c7]
   ```

3. **Pad to 32 bytes (SHA256 size):**
   ```
   [30 29 24 0e ... c7 00 00 00 00 00 00 00 00 00 00 00 00]
   ```

4. **Create SHA2_256 multihash:**
   ```
   Encodes: <hash-type-code><hash-length><hash-bytes>
   ```

5. **Convert to libp2p peer ID:**
   ```
   Base58 encode the multihash
   ```

6. **Output:** libp2p peer ID
   ```
   QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD
   ```

### Why Padding Is Needed

- CometBFT uses truncated 20-byte node IDs
- libp2p expects full 32-byte SHA256 hashes
- Padding maintains compatibility between systems

## Error Handling

### Invalid Node ID Length

**Error:**
```
Node ID must be 20 bytes (40 hex chars), got X bytes
```

**Cause:** Input is not exactly 40 hex characters

**Solution:** Verify you copied the full node ID:
```bash
# Correct: 40 characters
3029240e829e58e399bc7b6115bb6bc947cc24c7

# Incorrect: Too short
3029240e829e58e399bc7b6115bb6bc9

# Incorrect: Too long
3029240e829e58e399bc7b6115bb6bc947cc24c7abcdef
```

### Invalid Hex Characters

**Error:**
```
Error decoding node ID: encoding/hex: invalid byte
```

**Cause:** Non-hexadecimal characters in input

**Solution:** Ensure input contains only `0-9` and `a-f`:
```bash
# Correct
3029240e829e58e399bc7b6115bb6bc947cc24c7

# Incorrect (contains 'g')
3029240e829e58e399bc7b6115bb6bg947cc24c7
```

## Common Use Cases

### 1. Configuring Follower Bootstrap Peers

Most common use case. See [deploying-follower.md](../../docs/operations/deploying-follower.md).

### 2. Building Multi-Node Test Networks

Convert node IDs when setting up local test networks.

### 3. Debugging P2P Connectivity

Verify peer IDs match between CometBFT and libp2p layers.

### 4. Network Analysis

Convert validator node IDs when analyzing network topology.

## Related Documentation

- **Follower Deployment:** [docs/operations/deploying-follower.md](../../docs/operations/deploying-follower.md)
- **Bootstrap Peers:** [docs/operations/bootstrap-peers.md](../../docs/operations/bootstrap-peers.md)
- **Follower Configuration:** [cmd/accumulated/run/FOLLOWER-TYPE-README.md](../../cmd/accumulated/run/FOLLOWER-TYPE-README.md)

## Troubleshooting

### Problem: Follower Won't Connect to Peers

**Symptom:** `n_peers = 0` despite configured bootstrap peers

**Check:**
```bash
# Verify peer ID is correct
go run ./tools/cmd/convert-node-id <original-node-id>

# Should match what's in your accumulate.toml
```

**Common mistake:** Using CometBFT hex ID directly in config instead of libp2p format.

### Problem: Wrong Peer ID Format

**Symptom:** Config validation errors or connection failures

**Solution:**
- libp2p peer IDs start with `Qm` (typically)
- Should be ~46 characters long
- If your peer ID doesn't match this, re-convert

---

*Last Updated: 2025-10-13*
