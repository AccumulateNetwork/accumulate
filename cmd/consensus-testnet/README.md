# Consensus Testnet

A standalone testnet for testing the DAG-based Bullshark consensus implementation.

## Overview

This testnet runs multiple consensus nodes that:
1. Generate transactions at a configurable rate
2. Order transactions via Bullshark DAG consensus
3. Produce blocks at regular intervals
4. Allow runtime parameter changes via special transactions

## Quick Start with Docker

```bash
cd cmd/consensus-testnet

# Build and start 7 nodes
docker compose build
docker compose up

# View logs
docker compose logs -f

# View specific node
docker compose logs -f node1

# Stop
docker compose down
```

## Configuration

Each node accepts these flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--seed` | (random) | 32-byte hex seed for key generation |
| `--listen` | `/ip4/0.0.0.0/tcp/9000` | Multiaddr to listen on |
| `--peers` | (none) | Comma-separated peer multiaddrs |
| `--validators` | (self) | Comma-separated validator public keys |
| `--block-interval` | `3s` | Block production interval |
| `--tx-rate` | `100` | Transactions per second to generate |
| `--tx-size` | `256` | Size of transaction payloads |
| `--log-level` | `info` | Log level (debug, info, warn, error) |

## Transaction Types

### DataTx
Simple data transaction for throughput testing. Any node can submit.

### SetBlockTimeTx
Changes the block production interval. Only validators can submit.

```go
// Example: Change block time to 5 seconds
tx := NewSetBlockTimeTx(validatorPubKey, 5*time.Second, nonce)
tx.Sign(validatorPrivKey)
```

### SetTxRateTx
Changes the transaction generation rate. Only validators can submit.

```go
// Example: Change tx rate to 200/sec
tx := NewSetTxRateTx(validatorPubKey, 200, nonce)
tx.Sign(validatorPrivKey)
```

## Validator Keys (7-node testnet)

Generated from deterministic seeds for reproducibility:

| Node | Seed | Public Key |
|------|------|------------|
| 1 | `...01` | `4cb5abf6ad79fbf5abbccafcc269d85cd2651ed4b885b5869f241aedf0a5ba29` |
| 2 | `...02` | `7422b9887598068e32c4448a949adb290d0f4e35b9e01b0ee5f1a1e600fe2674` |
| 3 | `...03` | `f381626e41e7027ea431bfe3009e94bdd25a746beec468948d6c3c7c5dc9a54b` |
| 4 | `...04` | `fd50b8e3b144ea244fbf7737f550bc8dd0c2650bbc1aada833ca17ff8dbf329b` |
| 5 | `...05` | `fde4fba030ad002f7c2f7d4c331f49d13fb0ec747eceebec634f1ff4cbca9def` |
| 6 | `...06` | `b4c92afb3ba57f3ab959ffe6d319c98484a2155a0f4c65b2c37011ffd197b075` |
| 7 | `...07` | `3ee2a8a7283cb2fd728943daa127ef09e483071a8b4bc699ba4522f09b14cfde` |

## Generate Keys

To generate keys for a different number of nodes:

```bash
go run ./genkeys -n 10
```

## Architecture

```
┌─────────────────────────────────────────────────┐
│                 Consensus Node                  │
├─────────────────────────────────────────────────┤
│  Transaction Generator                          │
│  - Creates DataTx at configurable rate          │
│  - Submits to local worker                      │
├─────────────────────────────────────────────────┤
│  DAG Consensus (pkg/consensus/)                 │
│  - Worker: batches transactions                 │
│  - Primary: creates headers & certificates      │
│  - Bullshark: orders certificates               │
├─────────────────────────────────────────────────┤
│  Executor                                       │
│  - Processes ordered transactions               │
│  - Handles SetBlockTime, SetTxRate              │
│  - Produces blocks                              │
├─────────────────────────────────────────────────┤
│  Block Builder                                  │
│  - Creates blocks every N seconds               │
│  - Chains blocks via prev_hash                  │
│  - Computes state hash                          │
└─────────────────────────────────────────────────┘
```

## Metrics

Each node logs:
- Block production (height, tx count, hash)
- Status every 10 seconds (blocks, processed txns, state hash, round)
- Parameter changes (block interval, tx rate)

## Verifying Consensus

All honest nodes should:
1. Produce the same sequence of state hashes
2. Have blocks with matching transaction hashes
3. Advance rounds together

To verify, compare the `state` field in the status logs across nodes.

## Running Locally (without Docker)

```bash
# Build
go build -o consensus-testnet ./cmd/consensus-testnet

# Terminal 1: Node 1
./consensus-testnet --seed=0...01 --listen=/ip4/127.0.0.1/tcp/9001

# Terminal 2: Node 2
./consensus-testnet --seed=0...02 --listen=/ip4/127.0.0.1/tcp/9002 \
  --peers=/ip4/127.0.0.1/tcp/9001/p2p/12D3KooWEyoppNCUx8Yx66oV9fJnriXwCcXwDDUA2kj6vnc6iDEp

# ... etc
```

## BFT Properties

With 7 nodes (f=2), the network can tolerate:
- Up to 2 Byzantine (malicious) nodes
- Up to 2 crashed nodes

Quorum requires 5 nodes (2f+1 = 5).
