# Accumulate Network Port Reference

This document provides the definitive port configuration for all Accumulate networks.

## Port Calculation Formula

All Accumulate networks use the same port offset system:

```
Actual Port = Base Port + Port Offset
```

### Port Offsets (Constants)
- **Tendermint P2P**: 0 (base port)
- **Tendermint RPC**: 1
- **Accumulate P2P**: 2  
- **Prometheus**: 3
- **Accumulate API (JSON-RPC)**: 4

## Mainnet Port Configuration

**Base Port**: 16591 (Directory Network), 16691 (BVN)

### Directory Network (DN) Ports
| Service | Port | Description |
|---------|------|-------------|
| Tendermint P2P | 16591 | Peer-to-peer communication |
| Tendermint RPC | 16592 | Tendermint RPC interface |
| Accumulate P2P | 16593 | Accumulate P2P protocol |
| Prometheus | 16594 | Metrics collection |
| **Accumulate API** | **16595** | **JSON-RPC API interface** |

### Block Validator Network (BVN) Ports
| Service | Port | Description |
|---------|------|-------------|
| Tendermint P2P | 16691 | Peer-to-peer communication |
| Tendermint RPC | 16692 | Tendermint RPC interface |
| Accumulate P2P | 16693 | Accumulate P2P protocol |
| Prometheus | 16694 | Metrics collection |
| **Accumulate API** | **16695** | **JSON-RPC API interface** |

### Management Ports
| Service | Port | Description |
|---------|------|-------------|
| AccMan | 16666 | Accumulate Manager |
| SSL Client | 6695 | HTTPS access |

## Kermit Testnet Port Configuration

**Base Port**: 16591 (same as mainnet)

### Directory Network (DN) Ports
| Service | Port | Description |
|---------|------|-------------|
| Tendermint P2P | 16591 | Peer-to-peer communication |
| Tendermint RPC | 16592 | Tendermint RPC interface |
| Accumulate P2P | 16593 | Accumulate P2P protocol |
| Prometheus | 16594 | Metrics collection |
| **Accumulate API** | **16595** | **JSON-RPC API interface** |

### Block Validator Network (BVN) Ports
| Service | Port | Description |
|---------|------|-------------|
| Tendermint P2P | 16691 | Peer-to-peer communication |
| Tendermint RPC | 16692 | Tendermint RPC interface |
| Accumulate P2P | 16693 | Accumulate P2P protocol |
| Prometheus | 16694 | Metrics collection |
| **Accumulate API** | **16695** | **JSON-RPC API interface** |

## Development/Local Networks

**Base Port**: 26656 (default for local development)

### Directory Network (DN) Ports
| Service | Port | Description |
|---------|------|-------------|
| Tendermint P2P | 26656 | Peer-to-peer communication |
| Tendermint RPC | 26657 | Tendermint RPC interface |
| Accumulate P2P | 26658 | Accumulate P2P protocol |
| Prometheus | 26659 | Metrics collection |
| **Accumulate API** | **26660** | **JSON-RPC API interface** |

### Block Validator Network (BVN) Ports
BVN ports use base port + 10000 * partition_index:
- **BVN1**: 36656-36660
- **BVN2**: 46656-46660
- etc.

## Network Endpoints

### Mainnet
- **DN API**: `http://apollo-mainnet.accumulate.defidevs.io:16595`
- **BVN API**: `http://<bvn-node>:16695`

### Kermit Testnet
- **DN API**: `https://kermit-dn.accumulatenetwork.io/v2` (proxied)
- **BVN API**: `https://kermit-bvn.accumulatenetwork.io/v2` (proxied)
- **Direct DN API**: `http://kermit-dn.accumulate.defidevs.io:16595`
- **Direct BVN API**: `http://kermit-bvn.accumulate.defidevs.io:16695`

### Local Development
- **DN API**: `http://127.0.0.1:26660`
- **BVN API**: `http://127.0.0.1:36660` (BVN1)

## Important Notes

1. **API Port is Key**: The Accumulate API port (base + 4) is the primary interface for JSON-RPC v2 and v3 APIs.

2. **Network Consistency**: Both Mainnet and Kermit use the same base port structure (16591/16691).

3. **Development vs Production**: Local development uses 26656 base, while production networks use 16591/16691.

4. **HTTPS Proxying**: Production networks often proxy HTTPS (443) to the API ports (16595/16695).

5. **Bootstrap Configuration**: P2P bootstrap peers use the Accumulate P2P port (base + 2).

## References

- Port offsets defined in: `internal/node/config/enums_gen.go`
- Mainnet configuration: `docs/network/accumulate-mainnet-reference.md`
- Kermit configuration: `docs/configuration/accumulate-kermit.toml`
- Bootstrap files: `~/.accumulate/cache/kermit-bootstrap.json`
