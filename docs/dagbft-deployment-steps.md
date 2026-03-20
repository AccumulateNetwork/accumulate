# DAG-BFT Deployment Testing Steps

Progressive validation from single node to multi-node network under load.

## IMPORTANT: Test Types

There are **two distinct types** of DAG-BFT tests:

### 1. Consensus Layer Tests (`cmd/consensus-testnet`)

These tests validate the **consensus algorithm in isolation** using mock executors:
- Mock executors produce blocks on a timer, independent of consensus output
- No actual Accumulate transaction processing
- No dual network (DN + BVN) configuration
- Useful for validating DAG-BFT protocol correctness

**These tests are NOT sufficient for issue #3823.**

### 2. Accumulated Integration Tests (Issue #3823)

Issue #3823 requires running **actual accumulated validators** with:
- Real `DAGBFTService` wired into accumulated
- Dual network configuration (Directory Network + Block Validator Networks)
- Each validator runs both DN and BVN partitions
- Real Accumulate transaction processing through the executor
- Proper genesis initialization via `accumulated init network`

**Status**: The consensus layer tests pass, but accumulated integration tests
are still needed. See "Accumulated Integration Test" section below.

---

## Part A: Consensus Layer Tests (Mock Executors)

These steps validate the DAG-BFT consensus protocol using `consensus-testnet`.

### Prerequisites

```bash
cd /home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate
git checkout dagbft-integration
go build -o consensus-testnet ./cmd/consensus-testnet
```

---

### Step 1: Single Node Startup

**Goal**: Verify a single DAG-BFT node starts and initializes correctly.

```bash
# Generate a seed for reproducibility
SEED1=$(printf '%064d' 1)

# Start single node
./consensus-testnet \
  --seed=$SEED1 \
  --listen=/ip4/127.0.0.1/tcp/9001 \
  --tx-rate=0 \
  --log-level=debug
```

**Validation**:
- [ ] Node starts without errors
- [ ] Shows "Starting consensus node" log
- [ ] Graceful shutdown with Ctrl+C

---

## Step 2: Single Node Block Production

**Goal**: Verify the node produces blocks with self-generated transactions.

```bash
./consensus-testnet \
  --seed=$SEED1 \
  --listen=/ip4/127.0.0.1/tcp/9001 \
  --tx-rate=10 \
  --block-interval=3s \
  --pprof-port=6060
```

**Validation**:
- [ ] Blocks produced every 3 seconds (check logs)
- [ ] Transaction count increases
- [ ] State hash changes with each block
- [ ] No memory leaks: `watch -n 5 'ps -o rss= -p $(pgrep consensus-testnet)'`

---

## Step 3: Two Node Communication

**Goal**: Verify peer discovery and message passing between two nodes.

```bash
# Get node 1's peer ID first by starting it briefly
SEED1=$(printf '%064d' 1)
SEED2=$(printf '%064d' 2)

# Terminal 1: Start node 1
./consensus-testnet \
  --seed=$SEED1 \
  --listen=/ip4/127.0.0.1/tcp/9001 \
  --tx-rate=10 \
  --log-level=info

# Note the peer ID from node 1's startup log, then in Terminal 2:
# Replace PEER_ID with actual ID from node 1
./consensus-testnet \
  --seed=$SEED2 \
  --listen=/ip4/127.0.0.1/tcp/9002 \
  --peers=/ip4/127.0.0.1/tcp/9001/p2p/PEER_ID \
  --tx-rate=10 \
  --log-level=info
```

**Validation**:
- [ ] Nodes discover each other (check logs for peer connection)
- [ ] GossipSub topics joined
- [ ] Both nodes produce blocks
- [ ] State hashes eventually converge

---

## Step 4: Four Node Consensus (BFT Quorum)

**Goal**: Achieve BFT consensus with f=1 fault tolerance (4 nodes, tolerates 1 failure).

```bash
# Use the run-local.sh script or Docker
cd cmd/consensus-testnet

# Option A: Docker (recommended)
docker compose up --scale node=4

# Option B: Manual (4 terminals)
# See run-local.sh for the full command set
```

**Validation**:
- [ ] All 4 nodes start and connect
- [ ] Certificates reach quorum (3 of 4 votes)
- [ ] Blocks commit on all nodes
- [ ] State hashes match across nodes (check "state" in status logs)
- [ ] Kill one node (Ctrl+C) - consensus continues with 3
- [ ] Restart killed node - it syncs back up

---

## Step 5: Seven Node with Transactions

**Goal**: Full BFT network (f=2) processing transactions.

```bash
cd cmd/consensus-testnet

# Start 7-node network with Docker
docker compose up

# Watch logs for all nodes
docker compose logs -f

# Check specific node
docker compose logs -f node1
```

**Validation**:
- [ ] All 7 nodes connect and participate
- [ ] Status logs show matching state hashes
- [ ] Blocks produced every 3 seconds
- [ ] Transaction count increases consistently

---

## Step 6: Fault Tolerance Testing

**Goal**: Verify network handles node failures.

```bash
# With 7-node network running:

# Kill 1 node
docker compose stop node7
# Consensus should continue (6 nodes, need 5 for quorum)

# Kill another node
docker compose stop node6
# Consensus should continue (5 nodes = exactly quorum)

# Kill a third node
docker compose stop node5
# Consensus should STOP (4 nodes < 5 quorum)

# Restart nodes
docker compose start node5 node6 node7
# Consensus should resume
```

**Validation**:
- [ ] Network survives 2 node failures
- [ ] Network stalls with 3+ failures
- [ ] Network recovers when nodes rejoin

---

## Step 7: Multi-Node Network with Load

**Goal**: Stress test the network under sustained transaction load.

```bash
cd cmd/consensus-testnet

# Modify docker-compose.yml to increase tx-rate:
# Change TX_RATE from 100 to 500 or 1000

# Start with high load
docker compose up

# Monitor in another terminal
watch -n 1 'docker compose logs --tail=5 | grep -E "block|status"'

# Profile memory/CPU
docker stats
```

**With pprof enabled** (add `--pprof-port=6060` to node1):
```bash
# Memory profile
go tool pprof http://localhost:6060/debug/pprof/heap

# CPU profile (30 seconds)
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30
```

**Validation Criteria**:
- [ ] **Throughput**: Sustained 100+ tx/sec committed
- [ ] **Latency**: P99 < 5 seconds
- [ ] **Memory**: Stable (no unbounded growth)
- [ ] **CPU**: Reasonable usage (<80% sustained)
- [ ] **Block production**: Consistent interval
- [ ] **No data races**: Run with `-race` flag initially
- [ ] **Recovery**: Kill/restart nodes during load - system recovers

**Metrics to Monitor**:
```bash
# Key metrics endpoints
curl http://127.0.0.1:9090/metrics | grep -E "dagbft|consensus|block"

# Memory profile
go tool pprof http://127.0.0.1:6060/debug/pprof/heap

# CPU profile (30 seconds)
go tool pprof http://127.0.0.1:6060/debug/pprof/profile?seconds=30
```

**Load Test Results Template**:
```
Duration:        _____ minutes
Total TX:        _____
Committed TX:    _____
Failed TX:       _____
Throughput:      _____ tx/sec
Latency P50:     _____ ms
Latency P99:     _____ ms
Memory Start:    _____ MB
Memory End:      _____ MB
Nodes Failed:    _____
Recovery Time:   _____ sec
```

---

## Troubleshooting

### Common Issues

1. **Nodes don't connect**
   - Check firewall/ports
   - Verify bootstrap address
   - Check libp2p peer IDs match

2. **Consensus stalls**
   - Check if quorum is available
   - Look for certificate timeout errors
   - Verify network connectivity

3. **Memory growth**
   - Check DAG pruning is working
   - Verify batch cleanup after commit
   - Profile with pprof

4. **High latency**
   - Check network latency between nodes
   - Look for GossipSub congestion
   - Verify batch sizes are reasonable

### Debug Commands

```bash
# Check node status
./accumulated dagbft status --node http://127.0.0.1:8080

# View DAG state
./accumulated dagbft dag --node http://127.0.0.1:8080

# List peers
./accumulated dagbft peers --node http://127.0.0.1:8080

# Force leader rotation (testing)
./accumulated dagbft rotate --node http://127.0.0.1:8080
```

---

---

## Part B: Accumulated Integration Tests (Issue #3823)

These tests run actual `accumulated` validators with DAG-BFT consensus.

### Prerequisites

```bash
cd /home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate
git checkout dagbft-integration
go build ./cmd/accumulated
```

### Network Configuration

Create a network configuration file (`network.yml`) for 7 validators:

```yaml
id: "DAGBFTTestNet"
bvns:
  - id: "BVN1"
    nodes:
      - listen_address: "127.0.1.1"
      - listen_address: "127.0.1.2"
      - listen_address: "127.0.1.3"
  - id: "BVN2"
    nodes:
      - listen_address: "127.0.2.1"
      - listen_address: "127.0.2.2"
      - listen_address: "127.0.2.3"
      - listen_address: "127.0.2.4"
globals:
  executor_version: v2
```

Each node runs:
- **Directory Network (DN)** partition with DAG-BFT
- **Block Validator Network (BVN)** partition with DAG-BFT

### Initialize Network

```bash
# Generate genesis and node configurations
./accumulated init network network.yml -w .nodes

# This creates:
# .nodes/bvn1-1/accumulate.toml  (node config with DAGBFTService)
# .nodes/bvn1-1/directory-genesis.snap
# .nodes/bvn1-1/bvn1-genesis.snap
# ... for all 7 nodes
```

### Start Validators

```bash
# Terminal 1-7: Start each validator
./accumulated run -w .nodes/bvn1-1
./accumulated run -w .nodes/bvn1-2
./accumulated run -w .nodes/bvn1-3
./accumulated run -w .nodes/bvn2-1
./accumulated run -w .nodes/bvn2-2
./accumulated run -w .nodes/bvn2-3
./accumulated run -w .nodes/bvn2-4
```

### Validation Criteria for Issue #3823

The test must run for **30 minutes** and verify:

- [ ] All 7 validators start with DAG-BFT (check logs for "Running DAG-BFT")
- [ ] Both DN and BVN partitions use DAGBFTService
- [ ] Blocks are produced via DAG-BFT certificates (not timer-based)
- [ ] `certificatesCommitted > 0` in metrics
- [ ] State hashes converge across validators (at least 5/7 agreement)
- [ ] No CometBFT/Tendermint code paths executed
- [ ] Network handles transaction submission via API
- [ ] Cross-partition anchoring works (DN <-> BVN)

### Key Differences from Consensus Layer Tests

| Aspect | consensus-testnet | accumulated |
|--------|------------------|-------------|
| Executor | Mock (timer-based blocks) | Real Accumulate executor |
| Partitions | Single test partition | DN + BVN per validator |
| Transactions | Test transactions | Accumulate protocol |
| Block production | Timer triggers blocks | Certificates trigger blocks |
| State | Simple hash | Full BPT state |

### Current Blocker

The `DAGBFTService` in `cmd/accumulated/run/dagbft.go` needs verification that:
1. Blocks are produced from committed certificates (not independently)
2. The executor processes transactions from certificate payloads
3. Cross-partition communication works via the conductor

---

## Next Steps After Validation

1. [ ] Run 24-hour stability test
2. [ ] Test with network latency injection (tc netem)
3. [ ] Test with Byzantine node (malicious behavior)
4. [ ] Verify cross-partition anchoring with DAG-BFT
5. [ ] Performance comparison: CometBFT vs DAG-BFT
