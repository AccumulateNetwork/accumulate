# Issue #3905: Achieve 10K+ TPS Performance Target - Testing Plan

## Overview

Systematic performance testing to measure actual throughput limits across different validator/BVN configurations. Tests will increment from lower TPS rates upward until detecting network pushback (error rate spikes, mempool congestion, timeouts).

**Critical**: Each BVN has independent throughput limits. Total network TPS = sum of per-BVN rates.

---

## Phase 1: Fix Existing Issues

Before performance testing, resolve known problems:

### 1.1 BPT Sharding Integration Verification
- [ ] Confirm BPT sharding is actually enabled in Docker test environment
  - Verify `pkg/database/bpt/bpt.go` delegation logic works
  - Check that `internal/database/bpt.go:newBPT()` correctly enables sharding on root batches
  - Add logging to confirm ShardedBPT instances are created (not just plain BPT)
- [ ] Run unit tests for BPT sharding: `go test ./pkg/database/bpt/... -v`
- [ ] Run database tests: `go test ./internal/database/... -v` (ensure TestState passes)

### 1.2 Synthetic Message Routing Verification
- [ ] Confirm synthetic messages route through shards in ShardedBlock.Process()
- [ ] Run integration tests: `go test ./internal/core/execute/v2/block -v`
- [ ] Check logs for "Block execution is SHARDED" messages

### 1.3 Executor Configuration Loading
- [ ] Verify daemon startup loads shardCount from ExecutorConfig
- [ ] Confirm logs show "Executor initialized shard-count=N sharding=ENABLED"
- [ ] Check that configuration is persisted correctly

**Success criteria**: Build passes, all tests pass, sharding logs appear in Docker container startup.

---

## Phase 2: Systematic Performance Testing

### Test Strategy

**Increment approach**: Start at low TPS, increment gradually until detecting pushback
- Pushback indicators: Error rate spike >5%, timeouts, mempool lag, TPS plateau
- Each test runs 120-180 seconds to allow system to stabilize
- Monitor: TPS, error rate, latency, CPU/memory, per-shard load distribution

### Test Configurations

Test these combinations across different machines/scaling:

| Test ID | Validators | BVNs | Expected per-BVN Limit | Notes |
|---------|-----------|------|----------------------|-------|
| A1      | 3         | 1    | TBD (find limit)    | Single BVN baseline |
| A2      | 4         | 1    | TBD (find limit)    | 4-node cluster, 1 BVN |
| B1      | 3         | 2    | ~2x A1 per BVN      | Two BVNs on 3 validators |
| B2      | 4         | 2    | ~2x A2 per BVN      | Two BVNs on 4 validators |
| C1      | 3         | 3    | ~3x A1 per BVN      | Three BVNs on 3 validators |
| C2      | 4         | 3    | ~3x A2 per BVN      | Three BVNs on 4 validators |

### Test Execution per Configuration

For each configuration:

1. **Setup**: Start Docker network with X validators, Y BVNs
2. **Baseline**: Run at 1000 TPS (should be stable on any modern hardware)
3. **Increment sequence**:
   - 1000 TPS → 30s wait → check error rate
   - 2000 TPS → 30s wait → check error rate
   - 3000 TPS → 30s wait → check error rate
   - 5000 TPS → 30s wait → check error rate
   - 7000 TPS → 30s wait → check error rate
   - 10000 TPS → 30s wait → check error rate
   - 12000 TPS → 30s wait → check error rate (if stable)
   - Continue until error rate >5% or TPS plateaus

4. **Recording**: Capture for each TPS level:
   - Submitted transactions
   - Successful transactions
   - Failed transactions
   - Error rate %
   - Average TPS (actual sustained)
   - P50/P95/P99 latency
   - CPU usage per validator
   - Memory usage per validator
   - Per-shard transaction distribution (confirm balanced)

### Load Test Script Modifications

Modify `test/docker/parallel-loadtest.go` to support incremental testing:

```go
// Add flags:
-initial-tps 1000        // Start at this rate
-tps-increment 1000      // Increment by this amount each step
-increment-duration 30s  // Run each TPS level for this duration
-max-total-tps 12000     // Stop incrementing at this rate
-error-threshold 0.05    // Stop if error rate exceeds this
```

Script behavior:
1. Submit transactions at initial-tps for increment-duration
2. Measure error rate
3. If error_rate < error-threshold AND tps < max-total-tps: increment TPS, repeat
4. If error_rate >= error-threshold: stop and record "pushback detected at N TPS"

---

## Phase 3: Results Documentation

Create `test/docker/PERFORMANCE-RESULTS-RC-v1.5.1.md` with:

### Per-Configuration Results

For each test (A1-C2):

```markdown
## Test ID: A1 (3 validators, 1 BVN)

| TPS Target | Submitted | Success | Failed | Error % | Actual TPS | P50 Latency | P99 Latency | CPU% | Memory% | Status |
|-----------|-----------|---------|--------|---------|-----------|-------------|-------------|------|---------|--------|
| 1000      | 30000     | 30000   | 0      | 0.0%    | 1000      | 45ms       | 120ms      | 12%  | 35%     | ✓      |
| 2000      | 60000     | 60000   | 0      | 0.0%    | 2000      | 48ms       | 125ms      | 22%  | 40%     | ✓      |
| ...       | ...       | ...     | ...    | ...     | ...       | ...        | ...        | ...  | ...     | ...    |
| 10000     | 300000    | 285000  | 15000  | 5.0%    | 9500      | 180ms      | 850ms      | 78%  | 65%     | ⚠ pushback |

**Per-BVN Limit**: ~9500 TPS (detected at 10000 TPS target)
**Stable Range**: 1000-8000 TPS (<1% error)
**CPU Ceiling**: 78% at limit
**Memory Ceiling**: 65% at limit
```

### Summary Table

```markdown
## Summary: Per-BVN Throughput Limits

| Config | Validators | BVNs | Per-BVN Limit | Total Network TPS | Notes |
|--------|-----------|------|-------------|-----------------|-------|
| A1     | 3         | 1    | ~9500 TPS   | 9500 TPS        | Baseline single BVN |
| A2     | 4         | 1    | ~10000 TPS  | 10000 TPS       | 4-node improves on 3 |
| B1     | 3         | 2    | ~4500 TPS   | 9000 TPS        | BVN contention on 3 validators |
| B2     | 4         | 2    | ~5000 TPS   | 10000 TPS       | Better on 4 validators |
| C1     | 3         | 3    | ~3000 TPS   | 9000 TPS        | Severe contention |
| C2     | 4         | 3    | ~3500 TPS   | 10500 TPS       | 4 validators handle 3 BVNs better |
```

### Analysis

Document findings:

1. **Bottleneck identification**:
   - Is limit CPU-bound? (CPU hits 95%+ before TPS plateaus)
   - Is limit memory-bound? (Memory hits 90%+ before TPS plateaus)
   - Is limit network I/O? (latency spike before error rate)
   - Is limit lock contention? (shard distribution unbalanced)

2. **Per-BVN scaling**:
   - How does per-BVN throughput change with validator count?
   - Does adding validators improve single-BVN performance?
   - Do multiple BVNs on same hardware interfere?

3. **Sharding effectiveness**:
   - Measure actual shard load distribution (logs or metrics)
   - Confirm all 64 shards are being used
   - Check if sharding is actually improving throughput vs. sequential

4. **RC Readiness Assessment**:
   - If 1 BVN reaches 10K TPS on 4 validators → RC ready for single-partition deployment
   - If 3 BVNs x 3.5K TPS = 10.5K total → RC ready for 3-partition deployment
   - If neither: identify what's blocking and add to Phase 4 work

---

## Phase 4: Test Failure Debugging & Fixes

Tests will run unattended for 6-8 hours. Failures can occur at various stages. Use team coordination to diagnose and fix issues.

### Expected Failure Modes

| Failure Type | Symptoms | Root Cause | Fix Owner | Time to Fix |
|---|---|---|---|---|
| **Network startup failure** | "Network failed to start" after 30s | Docker network misconfiguration, port conflicts, container won't boot | network-debugger | 30 min - 2 hours |
| **Missing sharding logs** | No `shard-count=64` in Docker logs | BPT sharding not enabled in DAG-BFT startup path | sharding-debugger | 30 min - 1 hour |
| **Load test won't submit** | 0% success rate, connection refused | Network not exposed/accessible, API port wrong, firewall | network-debugger | 15 min - 1 hour |
| **All transactions fail with "Account.Main not found"** | 100% error rate, but network is healthy | Load test submitting too fast before funding completes | load-test-debugger | 15 min - 30 min |
| **TPS plateaus at low level (< 1K)** | TPS stuck despite low CPU/memory | Block production stalled, consensus stuck, mempool full | consensus-debugger | 1 - 3 hours |
| **Memory leak or runaway memory** | Memory grows 1-5% per minute until OOM | Goroutine leak, unbounded cache, batch not discarded | memory-debugger | 1 - 4 hours |
| **Docker cleanup fails** | Lingering containers/volumes block next test | Resource cleanup incomplete, Docker daemon issues | cleanup-debugger | 15 min - 1 hour |

### Debugging Process

For each failed test:

1. **Automated capture** (done by orchestration script):
   - Full Docker logs for all containers
   - Test metrics/output
   - Docker resource usage at failure time
   - Timestamp of failure

2. **Team investigation**:
   - Analyze logs and metrics for root cause
   - Identify which category of failure
   - Assign to appropriate specialist
   - Create sub-task for fix

3. **Fix & verify**:
   - Implement fix in isolated branch
   - Test the fix on single configuration
   - If successful: apply to main branch, re-run full suite
   - If unsuccessful: escalate to next tier specialist

### Team Structure for Debugging

Create v1.5.1-rc-debugging team with specialists:

- **network-debugger**: Docker networking, container startup, port exposure
- **sharding-debugger**: BPT sharding enablement, executor config, logging
- **load-test-debugger**: Load generator issues, transaction submission, account setup
- **consensus-debugger**: Block production, mempool, consensus throughput
- **memory-debugger**: Memory leaks, goroutine leaks, resource bounds
- **cleanup-debugger**: Docker resource cleanup, volume removal

### How to Trigger Debugging

If test orchestration detects failure:

```bash
# Script will create: test/docker/performance-results/FAILED-TEST-<test-id>.json
# Contains: logs, metrics, timestamp, test configuration

# Manually trigger investigation:
teams create v1.5.1-rc-debugging
teams assign network-debugger FAILED-TEST-A1.json
# Agent analyzes and reports findings
```

### Common Fixes & Workarounds

**Issue**: All tests fail at Docker startup
- **Quick fix**: Run `docker system prune -f` and `docker volume prune -f -a`
- **Better fix**: Improve cleanup script in handler

**Issue**: Sharding not enabled
- **Fix**: Ensure `cmd/accumulated/run/dagbft.go` loads executor config (already done by performance-validator)
- **Verify**: Check logs for `shard-count=64 sharding=ENABLED`

**Issue**: Load test stuck at 0% success with "Account.Main not found"
- **Cause**: Funding transactions not committed before sending from accounts
- **Fix**: Increase `wait-for-funding` duration in parallel-loadtest.go
- **Verify**: Logs show accounts created, then wait period, then sending begins

**Issue**: TPS plateaus < 2K despite low CPU
- **Cause**: Block production rate too slow (only 10 tx/block, not 100 tx/block)
- **Investigate**: Check how many tx per block in logs
- **Fix**: Increase block size limit or production rate in consensus config

---

## Phase 5: Re-testing & Verification

If failures occurred and fixes were applied:

1. Clean all data: `docker system prune -f && docker volume prune -f -a`
2. Rebuild binary with fixes: `go build ./cmd/accumulated`
3. Re-run full test suite: `./run-performance-suite.sh`
4. Compare results with Phase 2 baseline
5. Document what failed and how it was fixed

---

## Success Criteria

✅ RC v1.5.1-breaking is performance-ready if:

- [ ] Single 4-node cluster (1 BVN) sustains ≥8K TPS with <1% error
- [ ] Multiple BVN configurations don't degrade below per-BVN baseline
- [ ] All sharding logs present and shard distribution balanced
- [ ] No critical errors or crashes under load
- [ ] Performance results documented in `PERFORMANCE-RESULTS-RC-v1.5.1.md`
- [ ] All test failures debugged and root causes documented
- [ ] Any necessary fixes committed and re-tested

---

## Timeline

- **Phase 1** (Fix issues): 2-4 hours
- **Phase 2** (Testing): ~1.5-2 hours (runs unattended, 6 configs × 15 min each)
  - 6 configurations × ~15 minutes per config = ~90 minutes
  - Plus 30 min setup/cleanup = ~2 hours total
- **Phase 3** (Documentation): 1-2 hours
- **Phase 4** (Debugging & fixes): 2-8 hours (only if failures occur)
- **Phase 5** (Re-test): 1.5-2 hours (only if Phase 4 fixes applied)

**Total**: 7-12 hours minimum, up to 18-24 hours if major issues found

---

## Testing Mindset

1. **Expect failures**: With 6 configurations × 8 TPS levels = 48 individual test steps, some may fail
2. **Document everything**: Each failure is a learning opportunity for improving the code/tests
3. **Fix systematically**: Don't patch around issues; find and fix root causes
4. **Re-validate**: After fixes, always re-run the full suite to ensure fixes don't break other tests
5. **Build confidence**: Only mark RC as ready after all tests pass with documented shard verification
