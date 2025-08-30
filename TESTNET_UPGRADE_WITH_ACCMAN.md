# Testnet Upgrade to v1.5.0-experimental Using accman

## Overview
This guide provides step-by-step instructions for upgrading your testnet to v1.5.0-experimental using the `accman` management tool.

## Pre-Upgrade Checklist
```bash
# Verify accman is working and nodes are accessible
accman status

# Check current version on all nodes
accman version

# Verify network health
accman health
```

## Upgrade Procedure

### Step 1: Prepare for Upgrade
```bash
# Check if any transactions are pending (optional for testnet)
accman exec all "accumulate mempool status"

# Note current block height for verification later
accman exec all "accumulate network status | grep Height"
```

### Step 2: Stop All Nodes
```bash
# Stop all accumulate nodes
accman stop all

# Verify all nodes are stopped
accman status
# Should show all nodes as "stopped"
```

### Step 3: Update to v1.5.0-experimental
```bash
# Update accumulate on all nodes to the experimental branch
accman update --branch 3653-add-a-crosschainconductor-process-for-coordinating-partitions

# Or if accman supports version tags:
accman update --version v1.5.0-experimental
```

### Step 4: Build and Deploy
```bash
# Build the new version on all nodes
accman build

# Deploy the new binaries
accman deploy
```

### Step 5: Start All Nodes
```bash
# Start all nodes simultaneously
accman start all

# Wait for network to stabilize
sleep 30
```

### Step 6: Verify Upgrade
```bash
# Check version on all nodes
accman version
# Should show: v1.5.0-experimental

# Check network status
accman status
# All nodes should be "running" and "healthy"

# Verify consensus
accman consensus
```

### Step 7: Validate New Features
```bash
# Check ProofService is active
accman exec node1 "accumulate metrics | grep collection_proofs"

# Test cross-partition transaction
accman test cross-partition

# Monitor for collection proof creation
accman monitor --metric collection_proofs_created --duration 60
```

## Quick Upgrade Script for accman

If accman supports scripting, create this upgrade script:

```yaml
# accman-upgrade-v1.5.0.yaml
name: upgrade-to-v1.5.0-experimental
steps:
  - name: stop-nodes
    command: stop all
    wait: true
    
  - name: update-code
    command: update --branch 3653-add-a-crosschainconductor-process-for-coordinating-partitions
    wait: true
    
  - name: build
    command: build
    parallel: true
    
  - name: deploy
    command: deploy
    wait: true
    
  - name: start-nodes
    command: start all
    wait: true
    
  - name: wait-stabilize
    command: sleep 30
    
  - name: verify
    command: health
    expect: all_healthy
```

Run with:
```bash
accman run accman-upgrade-v1.5.0.yaml
```

## Monitoring Post-Upgrade

### Real-time Monitoring
```bash
# Monitor network health
accman monitor

# Watch ProofService metrics
accman metrics --filter "proof|collection" --watch

# Monitor partition lag
accman partition-lag --watch
```

### Performance Validation
```bash
# Run load test to verify performance improvements
accman test load --duration 120 --rate 50

# Expected improvements:
# - 13.2x faster proof generation for batches
# - 95% memory reduction for large batches
# - Collection proofs created when 2+ transactions go to same destination
```

## Rollback Procedure (If Needed)

```bash
# Stop all nodes
accman stop all

# Rollback to previous version
accman rollback
# or
accman update --branch main
accman build
accman deploy

# Start nodes
accman start all

# Verify rollback
accman version
```

## Expected Metrics After Upgrade

Monitor these metrics to confirm successful upgrade:

| Metric | Expected Value | Description |
|--------|---------------|-------------|
| `collection_proofs_created` | Increasing | Collection proofs being generated |
| `proof_savings` | > 0 | Individual proofs saved by batching |
| `transactions_in_collections` | Increasing | Transactions using collection proofs |
| `proof_validation_success` | ~100% | Proofs validating correctly |

## Troubleshooting

### If nodes don't start:
```bash
# Check logs
accman logs --tail 100 --follow

# Check for port conflicts
accman exec all "ss -tlpn | grep 26656"
```

### If consensus fails:
```bash
# Check peer connections
accman peers

# Restart specific node
accman restart node1
```

### If ProofService metrics don't appear:
```bash
# Verify ProofService is enabled
accman exec all "grep -i proof /etc/accumulate/config.toml"

# Check for errors
accman logs --grep "ProofService|proof_service" --tail 50
```

## Load Testing After Upgrade

```bash
# Run the comprehensive test suite
accman exec node1 "cd /path/to/accumulate/test/load && ./run_complete_test_suite.sh"

# Or use accman's built-in load testing
accman test load --profile extended
accman test load --profile sustained --duration 120
```

## Summary

The upgrade process with accman should take approximately 5-10 minutes:
1. Stop nodes (30 seconds)
2. Update code (2 minutes)
3. Build and deploy (3 minutes)
4. Start nodes (30 seconds)
5. Verification (2 minutes)

Since this is a testnet with full control, the process can be aggressive without concerns about data loss or downtime impact.