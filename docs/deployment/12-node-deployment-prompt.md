# Prompt: Deploy 12-Node Test Network Using Run-Dual

## Objective

Deploy a proper 12-node Accumulate test network using `accumulated run-dual` for load testing with BPT sharding enabled.

## Requirements Understanding

Before starting, confirm you understand:

1. **Network Architecture**
   - 12 total nodes: 3 BVNs × 4 validators each
   - Each validator runs BOTH Directory Network (DN) and one BVN partition (dual mode)
   - Separate processes per node (NOT devnet single-process)
   - Each node exposes HTTP APIs on unique ports
   - Bootstrap server for P2P peer discovery

2. **Deployment Components**
   - Network configuration file (YAML)
   - Bootstrap node for P2P discovery
   - 12 validator node directories with configs
   - Genesis snapshots (DN + 3 BVN genesis files)
   - Load testing setup with accessible APIs

3. **Expected Outputs**
   - 12 running processes (can use tmux/screen for management)
   - HTTP APIs accessible on ports 20080-20095 (or specified range)
   - P2P network fully connected
   - BPT sharding enabled with depth=4
   - Load generator ready to submit transactions

## Step-by-Step Deployment Tasks

### Phase 1: Network Initialization

**Task 1.1: Create Network Configuration**

Create `network-config.yml` in `/tmp/accumulate-test-network/`:

```yaml
id: "LoadTestNet"
bvns:
  - id: "BVN0"
    nodes:
      - listenAddress: "127.0.0.1"
        peerAddress: "127.0.0.1"
        advertizeAddress: "127.0.0.1"
        basePort: 20000
        dnnType: "validator"
        bvnnType: "validator"
      - listenAddress: "127.0.0.1"
        peerAddress: "127.0.0.1"
        advertizeAddress: "127.0.0.1"
        basePort: 20100
        dnnType: "validator"
        bvnnType: "validator"
      - listenAddress: "127.0.0.1"
        peerAddress: "127.0.0.1"
        advertizeAddress: "127.0.0.1"
        basePort: 20200
        dnnType: "validator"
        bvnnType: "validator"
      - listenAddress: "127.0.0.1"
        peerAddress: "127.0.0.1"
        advertizeAddress: "127.0.0.1"
        basePort: 20300
        dnnType: "validator"
        bvnnType: "validator"
  - id: "BVN1"
    nodes:
      - listenAddress: "127.0.0.1"
        peerAddress: "127.0.0.1"
        advertizeAddress: "127.0.0.1"
        basePort: 20400
        dnnType: "validator"
        bvnnType: "validator"
      - listenAddress: "127.0.0.1"
        peerAddress: "127.0.0.1"
        advertizeAddress: "127.0.0.1"
        basePort: 20500
        dnnType: "validator"
        bvnnType: "validator"
      - listenAddress: "127.0.0.1"
        peerAddress: "127.0.0.1"
        advertizeAddress: "127.0.0.1"
        basePort: 20600
        dnnType: "validator"
        bvnnType: "validator"
      - listenAddress: "127.0.0.1"
        peerAddress: "127.0.0.1"
        advertizeAddress: "127.0.0.1"
        basePort: 20700
        dnnType: "validator"
        bvnnType: "validator"
  - id: "BVN2"
    nodes:
      - listenAddress: "127.0.0.1"
        peerAddress: "127.0.0.1"
        advertizeAddress: "127.0.0.1"
        basePort: 20800
        dnnType: "validator"
        bvnnType: "validator"
      - listenAddress: "127.0.0.1"
        peerAddress: "127.0.0.1"
        advertizeAddress: "127.0.0.1"
        basePort: 20900
        dnnType: "validator"
        bvnnType: "validator"
      - listenAddress: "127.0.0.1"
        peerAddress: "127.0.0.1"
        advertizeAddress: "127.0.0.1"
        basePort: 21000
        dnnType: "validator"
        bvnnType: "validator"
      - listenAddress: "127.0.0.1"
        peerAddress: "127.0.0.1"
        advertizeAddress: "127.0.0.1"
        basePort: 21100
        dnnType: "validator"
        bvnnType: "validator"
globals:
  executorVersion: "v2-jiuquan"
  majorBlockSchedule: "* * * * *"
```

**Task 1.2: Initialize Network**

```bash
cd /tmp/accumulate-test-network
accumulated init network network-config.yml -w .nodes

# Verify generated structure:
# .nodes/
# ├── bootstrap/          # Bootstrap node for P2P discovery
# ├── bvn0-0/            # BVN0 validator 0
# │   ├── dnn/           # Directory Network partition
# │   ├── bvnn/          # BVN0 partition
# │   └── accumulate.toml
# ├── bvn0-1/            # BVN0 validator 1
# ... (12 total node directories)
# ├── directory-genesis.snap
# ├── bvn0-genesis.snap
# ├── bvn1-genesis.snap
# └── bvn2-genesis.snap
```

**Task 1.3: Enable BPT Sharding**

For EACH of the 12 node directories, add BPT configuration to `accumulate.toml`:

```bash
for node in .nodes/bvn*-*/; do
  cat >> "$node/accumulate.toml" <<EOF

# BPT Sharding Configuration
[bpt]
sharding_enabled = true
sharding_depth = 4
EOF
done
```

Or set environment variables when running nodes:
```bash
export ACC_BPT_SHARDING_ENABLED=true
export ACC_BPT_SHARDING_DEPTH=4
```

### Phase 2: Start Bootstrap Node

**Task 2.1: Start Bootstrap**

```bash
cd /tmp/accumulate-test-network/.nodes/bootstrap

# Start in background or separate tmux window
accumulated run -w . > /tmp/bootstrap.log 2>&1 &
BOOTSTRAP_PID=$!
echo $BOOTSTRAP_PID > /tmp/bootstrap.pid

# Wait for startup
sleep 5

# Verify bootstrap is running
ps -p $BOOTSTRAP_PID
curl -s http://localhost:16592/status
```

**Task 2.2: Verify Bootstrap P2P**

```bash
# Get bootstrap peer ID for other nodes
grep "peer.*ID" /tmp/bootstrap.log

# Should show bootstrap listening on P2P port
# Example: /ip4/127.0.0.1/tcp/16593/p2p/12D3KooW...
```

### Phase 3: Start All Validator Nodes

**Task 3.1: Create Node Startup Script**

Create `/tmp/accumulate-test-network/start-all-nodes.sh`:

```bash
#!/bin/bash

set -e

NODES_DIR="/tmp/accumulate-test-network/.nodes"
LOG_DIR="/tmp/accumulate-test-network/logs"
mkdir -p "$LOG_DIR"

# BPT Sharding environment variables
export ACC_BPT_SHARDING_ENABLED=true
export ACC_BPT_SHARDING_DEPTH=4

# Array of node directories
nodes=(
  "bvn0-0" "bvn0-1" "bvn0-2" "bvn0-3"
  "bvn1-0" "bvn1-1" "bvn1-2" "bvn1-3"
  "bvn2-0" "bvn2-1" "bvn2-2" "bvn2-3"
)

echo "Starting 12 validator nodes..."

for node in "${nodes[@]}"; do
  echo "Starting $node..."

  cd "$NODES_DIR/$node"

  # Run dual mode (DN + BVN partitions)
  accumulated run-dual dnn bvnn > "$LOG_DIR/$node.log" 2>&1 &

  pid=$!
  echo $pid > "$LOG_DIR/$node.pid"
  echo "  Started $node (PID: $pid)"

  # Small delay between starts
  sleep 2
done

echo ""
echo "All 12 nodes started!"
echo "PIDs saved to $LOG_DIR/*.pid"
echo "Logs available at $LOG_DIR/*.log"
echo ""
echo "Check status with: ./check-nodes.sh"
```

**Task 3.2: Create Node Status Checker**

Create `/tmp/accumulate-test-network/check-nodes.sh`:

```bash
#!/bin/bash

LOG_DIR="/tmp/accumulate-test-network/logs"

echo "=== Node Status Check ==="
echo ""

# Check all PIDs
for pidfile in "$LOG_DIR"/*.pid; do
  if [ -f "$pidfile" ]; then
    node=$(basename "$pidfile" .pid)
    pid=$(cat "$pidfile")

    if ps -p $pid > /dev/null 2>&1; then
      echo "✓ $node (PID: $pid) - RUNNING"
    else
      echo "✗ $node (PID: $pid) - STOPPED"
    fi
  fi
done

echo ""
echo "=== API Endpoint Check ==="
echo ""

# Check HTTP APIs (assuming standard port offsets)
# Base ports: 20000, 20100, 20200, etc.
# API offset: typically +24 or +80 depending on config

base_ports=(20000 20100 20200 20300 20400 20500 20600 20700 20800 20900 21000 21100)
node_names=(
  "bvn0-0" "bvn0-1" "bvn0-2" "bvn0-3"
  "bvn1-0" "bvn1-1" "bvn1-2" "bvn1-3"
  "bvn2-0" "bvn2-1" "bvn2-2" "bvn2-3"
)

for i in "${!base_ports[@]}"; do
  port=$((${base_ports[$i]} + 80))  # API typically at base + 80
  node="${node_names[$i]}"

  if curl -s -m 2 "http://localhost:$port/v3/describe" > /dev/null 2>&1; then
    echo "✓ $node API (port $port) - RESPONDING"
  else
    echo "✗ $node API (port $port) - NOT RESPONDING"
  fi
done
```

**Task 3.3: Make Scripts Executable and Run**

```bash
chmod +x /tmp/accumulate-test-network/start-all-nodes.sh
chmod +x /tmp/accumulate-test-network/check-nodes.sh

# Start all nodes
/tmp/accumulate-test-network/start-all-nodes.sh

# Wait for startup (60 seconds)
sleep 60

# Check status
/tmp/accumulate-test-network/check-nodes.sh
```

### Phase 4: Verify Network Health

**Task 4.1: Check Consensus**

```bash
# Check each node's logs for consensus messages
for log in /tmp/accumulate-test-network/logs/*.log; do
  echo "=== $(basename $log) ==="
  grep -i "consensus\|started\|running" "$log" | tail -5
  echo ""
done
```

**Task 4.2: Check P2P Connectivity**

```bash
# Each node should connect to bootstrap and other peers
for log in /tmp/accumulate-test-network/logs/*.log; do
  echo "=== $(basename $log) peers ==="
  grep -i "peer\|connected" "$log" | tail -3
  echo ""
done
```

**Task 4.3: Verify BPT Sharding Active**

```bash
# Look for BPT sharding messages
grep -r "BPT\|shard" /tmp/accumulate-test-network/logs/*.log | head -20
```

**Task 4.4: Test API Endpoints**

```bash
# Test a few API endpoints
curl -s http://localhost:20080/v3/describe | jq '.data.network.id'
curl -s http://localhost:20180/v3/describe | jq '.data.network.id'
curl -s http://localhost:20480/v3/describe | jq '.data.network.id'
```

### Phase 5: Load Testing Setup

**Task 5.1: Identify Faucet Account**

```bash
# The network should have a pre-funded faucet account
# Check genesis or initialization output for faucet details

grep -r "faucet" /tmp/accumulate-test-network/.nodes/
```

**Task 5.2: Prepare Load Generator**

```bash
# Get API endpoints
api_endpoints=(
  "http://localhost:20080/v3"
  "http://localhost:20180/v3"
  "http://localhost:20280/v3"
  "http://localhost:20380/v3"
)

# Create load test configuration
cat > /tmp/accumulate-test-network/loadtest-config.json <<EOF
{
  "endpoints": [
    "http://localhost:20080/v3",
    "http://localhost:20180/v3",
    "http://localhost:20280/v3",
    "http://localhost:20380/v3",
    "http://localhost:20480/v3",
    "http://localhost:20580/v3",
    "http://localhost:20680/v3",
    "http://localhost:20780/v3",
    "http://localhost:20880/v3",
    "http://localhost:20980/v3",
    "http://localhost:21080/v3",
    "http://localhost:21180/v3"
  ],
  "tps": 100,
  "duration": "30m",
  "accounts": 1000
}
EOF
```

**Task 5.3: Run Load Test**

```bash
# Using the load generator tool
cd /home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate

# Build if needed
go build ./test/cmd/load-generator

# Run load test
./load-generator \
  --config /tmp/accumulate-test-network/loadtest-config.json \
  --report /tmp/accumulate-test-network/loadtest-results.json

# Monitor during test
watch -n 5 '/tmp/accumulate-test-network/check-nodes.sh'
```

### Phase 6: Monitoring and Validation

**Task 6.1: Monitor Resource Usage**

```bash
# CPU and memory per process
for pidfile in /tmp/accumulate-test-network/logs/*.pid; do
  node=$(basename "$pidfile" .pid)
  pid=$(cat "$pidfile")
  echo "=== $node (PID: $pid) ==="
  ps -p $pid -o %cpu,%mem,rss,vsz,cmd | tail -1
  echo ""
done
```

**Task 6.2: Monitor Transaction Throughput**

```bash
# Check logs for transaction processing
for log in /tmp/accumulate-test-network/logs/bvn0-*.log; do
  echo "=== $(basename $log) ==="
  grep -i "transaction\|executed" "$log" | tail -5
  echo ""
done
```

**Task 6.3: Verify BPT Sharding Under Load**

```bash
# Look for BPT operations during load
tail -f /tmp/accumulate-test-network/logs/bvn0-0.log | grep -i "bpt\|shard"
```

### Phase 7: Cleanup

**Task 7.1: Stop All Nodes**

Create `/tmp/accumulate-test-network/stop-all-nodes.sh`:

```bash
#!/bin/bash

LOG_DIR="/tmp/accumulate-test-network/logs"

echo "Stopping all nodes..."

for pidfile in "$LOG_DIR"/*.pid; do
  if [ -f "$pidfile" ]; then
    node=$(basename "$pidfile" .pid)
    pid=$(cat "$pidfile")

    if ps -p $pid > /dev/null 2>&1; then
      echo "Stopping $node (PID: $pid)..."
      kill $pid
      sleep 1

      # Force kill if still running
      if ps -p $pid > /dev/null 2>&1; then
        echo "  Force killing $node..."
        kill -9 $pid
      fi
    else
      echo "$node already stopped"
    fi

    rm "$pidfile"
  fi
done

# Stop bootstrap
if [ -f /tmp/bootstrap.pid ]; then
  pid=$(cat /tmp/bootstrap.pid)
  if ps -p $pid > /dev/null 2>&1; then
    echo "Stopping bootstrap (PID: $pid)..."
    kill $pid
  fi
  rm /tmp/bootstrap.pid
fi

echo "All nodes stopped!"
```

**Task 7.2: Archive Results**

```bash
# Create results archive
tar -czf /tmp/accumulate-test-results-$(date +%Y%m%d-%H%M%S).tar.gz \
  /tmp/accumulate-test-network/logs/ \
  /tmp/accumulate-test-network/loadtest-results.json
```

## Success Criteria

✅ **Deployment successful if:**
1. All 12 validator processes running
2. Bootstrap server operational
3. All HTTP APIs responding (12 endpoints)
4. P2P network fully connected (12+ peers per node)
5. Consensus producing blocks
6. BPT sharding active in logs
7. Load test completes successfully
8. Transaction success rate > 99%
9. No crashes or restarts during test

## Troubleshooting

### Nodes won't start
- Check logs: `tail -f /tmp/accumulate-test-network/logs/bvn0-0.log`
- Verify ports not in use: `netstat -tuln | grep 20000`
- Check bootstrap is running: `curl http://localhost:16592/status`

### APIs not responding
- Verify port offsets in configuration
- Check if HTTP service is enabled in accumulate.toml
- Look for "Listening" messages in logs

### Nodes not connecting
- Verify bootstrap peer ID is correct
- Check firewall/network settings
- Ensure all nodes have bootstrap in peer list

### BPT sharding not active
- Verify environment variables set: `echo $ACC_BPT_SHARDING_ENABLED`
- Check accumulate.toml has BPT configuration
- Look for errors in logs: `grep -i "bpt.*error" logs/*.log`

## Expected Timeline

- Network initialization: 2-3 minutes
- Node startup: 5-10 minutes
- Network stabilization: 2-3 minutes
- Load test (30 min): 30 minutes
- **Total**: ~45 minutes for complete deployment and testing

## Files Created

- `/tmp/accumulate-test-network/network-config.yml`
- `/tmp/accumulate-test-network/.nodes/` (12 node directories + bootstrap)
- `/tmp/accumulate-test-network/start-all-nodes.sh`
- `/tmp/accumulate-test-network/stop-all-nodes.sh`
- `/tmp/accumulate-test-network/check-nodes.sh`
- `/tmp/accumulate-test-network/logs/*.log` (13 log files)
- `/tmp/accumulate-test-network/logs/*.pid` (13 PID files)
- `/tmp/accumulate-test-network/loadtest-config.json`
- `/tmp/accumulate-test-network/loadtest-results.json`
