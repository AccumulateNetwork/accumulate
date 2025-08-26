# Testnet Upgrade Commands for accman

## Quick Upgrade to v1.5.0-experimental

Since you're using `accman` to manage your testnet servers, here are the commands to upgrade all nodes to v1.5.0-experimental:

## Prerequisites
```bash
# Ensure accman is configured with your testnet nodes
accman nodes list
```

## Upgrade Steps

### 1. Stop All Nodes
```bash
# Stop accumulate on all testnet nodes
accman exec all "systemctl stop accumulate || pkill accumulate"

# Verify stopped
accman exec all "pgrep accumulate || echo 'Stopped'"
```

### 2. Update Code on All Nodes
```bash
# Fetch and checkout the new version
accman exec all "cd /path/to/accumulate && git fetch origin"
accman exec all "cd /path/to/accumulate && git checkout 3653-add-a-crosschainconductor-process-for-coordinating-partitions"
accman exec all "cd /path/to/accumulate && git pull"
```

### 3. Build and Install
```bash
# Clean, build, and install new version
accman exec all "cd /path/to/accumulate && make clean && make build"
accman exec all "cd /path/to/accumulate && sudo make install"

# Verify version
accman exec all "accumulate version"
```

### 4. Start All Nodes Simultaneously
```bash
# Start all nodes at once
accman exec all "systemctl start accumulate"

# Wait for network to stabilize
sleep 30
```

### 5. Verify Upgrade
```bash
# Check network status
accman exec all "accumulate network status | head -10"

# Check ProofService metrics
accman exec node1 "accumulate metrics | grep -E 'collection_proofs|proof_savings'"

# Run basic test
accman exec node1 "accumulate account create acc://test-upgrade-$(date +%s)"
```

## One-Line Upgrade (Aggressive)
```bash
# Complete upgrade in one command (testnet only!)
accman exec all "systemctl stop accumulate && cd /path/to/accumulate && git fetch && git checkout 3653-add-a-crosschainconductor-process-for-coordinating-partitions && git pull && make clean && make build && sudo make install && systemctl start accumulate"
```

## Monitoring After Upgrade
```bash
# Watch network health
accman exec all "accumulate network status" --watch

# Monitor ProofService collection proofs
accman exec node1 "watch -n 5 'accumulate metrics | grep collection_proofs'"

# Check logs for errors
accman exec all "tail -f /var/log/accumulate/*.log | grep -i error"
```

## Rollback if Needed
```bash
# Emergency rollback to v1.4.3
accman exec all "systemctl stop accumulate"
accman exec all "cd /path/to/accumulate && git checkout main && git pull"
accman exec all "cd /path/to/accumulate && make clean && make build && sudo make install"
accman exec all "systemctl start accumulate"
```

## Load Testing After Upgrade
```bash
# Run the extended test suite (from node1)
accman exec node1 "cd /path/to/accumulate/test/load && ./run_complete_test_suite.sh"

# Run visual monitor (requires local terminal)
accman ssh node1
cd /path/to/accumulate/test/load
./visual_monitor.sh
```

## Expected Results

After successful upgrade, you should see:
1. All nodes running v1.5.0-experimental
2. Network consensus established
3. `collection_proofs_created` metric incrementing when batching occurs
4. `proof_savings` showing reduction in individual proofs
5. 13.2x performance improvement for batched transactions
6. 95% memory reduction for large batches

## Notes

- Replace `/path/to/accumulate` with actual path
- Update node names if different from node1, node2, etc.
- Since this is testnet with full control, no backup needed
- Total upgrade time: ~2-5 minutes