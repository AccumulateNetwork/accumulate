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

## Phase 4: Issues & Fixes (if needed)

If performance doesn't reach 10K per-BVN, create sub-issues:

- **#3905.1**: Profile bottleneck (CPU? Memory? Lock contention?)
- **#3905.2**: Fix identified bottleneck
- **#3905.3**: Re-test and confirm improvement

---

## Success Criteria

✅ RC v1.5.1-breaking is performance-ready if:

- [ ] Single 4-node cluster (1 BVN) sustains ≥8K TPS with <1% error
- [ ] Multiple BVN configurations don't degrade below per-BVN baseline
- [ ] All sharding logs present and shard distribution balanced
- [ ] No critical errors or crashes under load
- [ ] Performance results documented in `PERFORMANCE-RESULTS-RC-v1.5.1.md`

---

## Timeline

- **Phase 1** (Fix issues): 2-4 hours
- **Phase 2** (Testing): 6-8 hours (6 configurations × ~1 hour per config including setup/cooldown)
- **Phase 3** (Documentation): 1-2 hours
- **Phase 4** (If fixes needed): TBD based on findings

**Total**: 10-15 hours for complete performance validation
