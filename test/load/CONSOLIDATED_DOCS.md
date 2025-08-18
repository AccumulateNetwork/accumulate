# Consolidated Load Testing & CrossChain Documentation

> **Note**: This document consolidates 27 separate markdown files into a single, manageable reference guide.

## Table of Contents

1. [Quick Start](#quick-start)
2. [Load Testing Guide](#load-testing-guide)
3. [CrossChain Conductor](#crosschain-conductor)
4. [DevNet Configuration](#devnet-configuration)
5. [Performance Results](#performance-results)
6. [Troubleshooting](#troubleshooting)
7. [API & Connection Management](#api--connection-management)
8. [Development Guides](#development-guides)

---

## Quick Start

### Running Load Tests

```bash
# Start DevNet
go run ./cmd/accumulated run devnet -w .devnet

# Run 50k transactions at 100 TPS
go test -v -run TestSimple50K -timeout 20m

# Run 100k transactions at 200 TPS  
go test -v -run TestSimple100K -timeout 45m

# Run with custom parameters
go test -v -run TestStreamlinedLoad -args \
  -txs 100000 -tps 200 -k 40 -a 40 -timeout 15m

# Visual monitoring
./run_visual_test.sh | tee test.log
```

### Quick Diagnostics

```bash
# Check DevNet status
ps aux | grep accumulated
ss -tlnp | grep 266
curl http://127.0.0.1:26660/metrics

# Smart endpoint discovery
go test -run TestDevnetDiscovery

# Verify connectivity
curl http://127.0.0.1:26660/v3
```

---

## Load Testing Guide

### Available Tests

#### TestStreamlinedLoad (Primary)
- **Purpose**: Configurable load test with full flag support
- **Location**: `test/load/sl_test.go`
- **Flags**:
  - `-txs`: Number of transactions (default: 1000)
  - `-tps`: Target TPS, 0=unlimited (default: 0)
  - `-k`: Sender accounts (default: 10)
  - `-a`: Receiver accounts (default: 10)
  - `-timeout`: Settlement timeout (default: auto)
  - `-verbose`: Enable verbose logging

#### TestSimple50K / TestSimple100K
- **Purpose**: Pre-configured tests for common scenarios
- **Defaults**: Check constants in test files - labeled as `// DEFAULT:`
- **Note**: Edit targetTPS constant to change rate

### Performance Baselines

| TPS Target | Transactions | Success Rate | Duration | Status |
|------------|-------------|--------------|----------|--------|
| 50         | 100,000     | 100%         | 2000s    | ✅ |
| 100        | 50,000      | 99.3%        | 500s     | ✅ |
| 200        | 100,000     | 100%         | 500s     | ✅ |
| 500        | 10,000      | 100%         | 20s      | ✅ |
| 1000       | 10,000      | 100%         | 10s      | ✅ |
| 2000       | 50,000      | 100%         | 25s      | ✅ |
| 3000       | 50,000      | 100%         | 16.7s    | ✅ |

**Key Finding**: No breaking point found up to 3000 TPS

### Test Configuration

```go
// Edit these DEFAULTS in simple_*.go files:
const (
    numSenders   = 40      // DEFAULT: Number of sender accounts
    numReceivers = 40      // DEFAULT: Number of receiver accounts  
    totalTxs     = 100000  // DEFAULT: Total transactions
    targetTPS    = 200     // DEFAULT: Target TPS
    txAmount     = int64(0.001 * 1e8) // DEFAULT: 0.001 ACME per tx
)
```

---

## CrossChain Conductor

### Architecture Overview

The CrossChain Conductor orchestrates cross-partition blockchain transactions with:
- Async processing with per-destination queues
- Automatic retry with exponential backoff  
- Missing transaction recovery
- Collection proof batching

### Key Components

```yaml
Location: internal/core/execute/v2/crosschain/
Files:
  conductor.go         # Main conductor logic
  types.go            # Data structures
  recovery.go         # Missing transaction recovery
  proof_service.go    # Proof construction/validation
```

### Core Functions

```go
// Constructor - line 108
NewCrossChainConductor(dispatcher, logger)

// Submit transactions - line 192
SubmitSynthetic(ctx, messages, dest)

// Process requests - line 162
processSyntheticRequest(req)

// Monitor errors - line 205
monitorTransmissionErrors()

// Handle retries - line 273
processRetries()
```

### Configuration

```go
config: ConductorConfig{
    ForceCollectionProofs:  true,  // Always use collection proofs
    CollectionMaxBatchSize: 100,   // Max per batch
}
maxRetries: 3                      // Retry attempts
retryDelay: 2 * time.Second        // Between retries
syntheticChan: buffer(100)         // Async queue
retryChan: buffer(50)              // Retry queue
```

### Enabling CrossChain Conductor

```go
// File: internal/core/execute/v2/block/block_end.go:578
if x.crosschainConductor != nil {
    err = x.crosschainConductor.SubmitSynthetic(ctx, messages, dest)
} else {
    err = dispatcher.Submit(ctx, dest, &messaging.Envelope{Messages: messages})
}

// File: internal/node/daemon/run.go:407
EnableCrosschainCoordinator: true
```

---

## DevNet Configuration

### Fixed Configuration

```go
// File: cmd/accumulated/run/devnet.go
const devNetDefaultHost = "/ip4/127.0.0.1"  // Changed from 127.0.1.1
```

### Network Topology

- **Nodes**: 12 total (default configuration)
- **Partitions**: 3 (BVN0, BVN1, Directory)
- **Ports**: 26656-26660
- **API Endpoint**: `http://127.0.0.1:26660/v3`

### Smart Discovery System

```go
// Automatic endpoint detection
endpoint, err := FindDevnetEndpoint()
if err != nil {
    // Handle error - devnet not running
}

// Discovery checks multiple sources:
// 1. Default ports (26656-26660)
// 2. Process detection
// 3. Network interfaces
// 4. Config files
```

---

## Performance Results

### System Capabilities

- **Maximum Tested TPS**: 3000 (100% success)
- **Breaking Point**: Not found (beyond 3000 TPS)
- **Optimal Range**: 200-500 TPS for production
- **Memory Usage**: ~200 bytes per transaction
- **Retry Success Rate**: 95%+ for transient errors

### Resource Usage

```yaml
Per_Transaction: 200 bytes
Goroutines: 3 per conductor
Connection_Pool: 100 clients max
Client_TTL: 5 minutes
Queue_Sizes:
  synthetic: 100
  retry: 50
```

---

## Troubleshooting

### Common Issues & Solutions

#### Connection Refused
```bash
# Check if devnet is running
ps aux | grep accumulated

# Start devnet
go run ./cmd/accumulated run devnet -w .devnet

# Use smart discovery
endpoint, err := FindDevnetEndpoint()
```

#### Low Success Rate
```bash
# Reduce TPS
-tps 50  # Instead of 200

# Increase senders/receivers
-k 40 -a 40  # Better distribution

# Check metrics
curl http://127.0.0.1:26660/metrics
```

#### Transaction Failures
```go
// Check conductor metrics
sent, errors, retried, txErrors := conductor.GetMetrics()

// Increase retries
conductor.maxRetries = 5
conductor.retryDelay = 5 * time.Second
```

#### Missing Transactions
```go
// Use recovery manager
rm := conductor.recoveryManager
missing := rm.DetectMissingTransactions()
rm.RequestMissingTransactions(req)
```

---

## API & Connection Management

### V3 Connection Pool

**Problem**: Connection exhaustion with direct client creation
**Solution**: Connection pooling with automatic cleanup

```go
// OLD - Creates new connection each time
client := jsonrpc.NewClient(url)

// NEW - Uses pooled connection
client := GetPooledClient(url)
```

**Benefits**:
- 41% latency reduction
- Prevents "too many open files" errors
- Automatic TTL and cleanup

### API Error Handling

```go
// Retry with exponential backoff
err := QueryWithRetry(ctx, client, operation)

// Connection pool cleanup
defer CleanupClientPool()
```

---

## Development Guides

### Adding New Tests

1. **Use existing framework** - Don't create new test files
2. **Configure via flags** - Use `-args` for parameters
3. **Label defaults** - Mark with `// DEFAULT:` comment
4. **Run verification** - Ensure transactions are real, not mocked

### Modifying CrossChain Conductor

1. **Check if enabled** - Verify `EnableCrosschainCoordinator: true`
2. **Update both paths** - Handle conductor and direct dispatcher
3. **Add metrics** - Track new operations
4. **Test recovery** - Verify retry logic works

### Performance Testing Workflow

```bash
# 1. Start fresh devnet
./scripts/clean_restart.sh

# 2. Run baseline test
go test -v -run TestSimple50K -timeout 20m

# 3. Increase load progressively
for tps in 100 200 500 1000; do
    go test -v -run TestStreamlinedLoad -args -txs 10000 -tps $tps
done

# 4. Monitor metrics
watch -n 1 'curl -s http://127.0.0.1:26660/metrics'

# 5. Analyze results
grep "Test PASSED" *.log
```

---

## File Consolidation Summary

This document consolidates:
- **Load Test Guides**: sl_*.md, TEST_USAGE.md, LOAD_TEST_GUIDE.md
- **CrossChain Docs**: CrossChainConductor_*.md, PROOF_*.md
- **DevNet Config**: DEVNET_CONFIGURATION.md, DISCOVERY_DEMO.md
- **Performance**: TPS_PERFORMANCE_REPORT.md, FINAL_REVIEW_SUMMARY.md
- **API Fixes**: v3_connection_fixes.md, APPLY_V3_FIXES.md
- **Development**: AI_ASSISTANT_GUIDE.md, CODE_REVIEW_*.md

**Total Reduction**: 27 files → 1 comprehensive guide

---

## Quick Reference

### Essential Commands
```bash
# DevNet
go run ./cmd/accumulated run devnet -w .devnet

# Tests
go test -v -run TestSimple50K         # 50k @ 100 TPS
go test -v -run TestSimple100K        # 100k @ 200 TPS
go test -v -run TestStreamlinedLoad   # Configurable

# Monitoring
curl http://127.0.0.1:26660/metrics
tail -f .devnet/*/node.log | grep ERROR
```

### Key Files
```
test/load/
  sl_test.go              # Main streamlined test
  simple_50k_test.go      # 50k preset
  simple_100k_test.go     # 100k preset
  devnet_endpoint.go      # Discovery system

internal/core/execute/v2/crosschain/
  conductor.go            # Main conductor
  types.go               # Data structures
  recovery.go            # Recovery system
  proof_service.go       # Proof handling
```

### Configuration Points
```go
// Conductor config
ForceCollectionProofs: true
CollectionMaxBatchSize: 100
maxRetries: 3
retryDelay: 2*time.Second

// Test defaults (edit in test files)
numSenders: 40
numReceivers: 40
totalTxs: 100000
targetTPS: 200
```

---

*Last Updated: 2025-08-18 | Consolidated from 27 documentation files*