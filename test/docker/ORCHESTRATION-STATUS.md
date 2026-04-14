# Issue #3905 Test Orchestration Status

**Status**: Partially complete - script created, still needs critical fixes

---

## What's Done ✅

1. **Test Orchestration Script** (`run-performance-suite.sh`)
   - Orchestrates all 6 test configurations in sequence
   - Incremental TPS testing (1000 → 12000 TPS)
   - Unattended execution
   - Results collection and aggregation
   - Summary report generation

2. **BPT Sharding Fix** (Applied by performance-validator agent)
   - **File**: `cmd/accumulated/run/dagbft.go`
   - **Issue**: DAG-BFT startup path was not loading executor shard count from database
   - **Fix**: Added database config loading after execOpts creation (line ~183)
   - **Status**: Applied but **NOT YET COMMITTED**

3. **Testing Plans** (`.claude/plans/issue-3905-performance-testing-plan.md`)
   - Comprehensive 4-phase testing strategy
   - 6 test configurations defined
   - Incremental TPS sequence documented
   - Success criteria specified

---

## What Still Needs to Be Done ⚠️

### 1. Commit DAG-BFT Sharding Enablement Fix
```bash
# Need to find and commit the dagbft.go changes
git add cmd/accumulated/run/dagbft.go
git commit -m "Issue #3905: Enable BPT sharding in DAG-BFT startup path

Load executor shard count from database config during startup
so that sharding is actually enabled (previously was always 0/disabled).
This is critical for the 10K+ TPS performance target."
```

### 2. Dynamic Docker Compose Generation
**Current issue**: Script generates docker-compose templates but doesn't create proper Dockerfile or service configurations for N validators × M BVNs.

**Needed**:
- Dockerfiles for different BVN/validator combinations
- Proper network configuration for multi-BVN deployments
- Health checks that work across multiple BVNs

### 3. Load Test Incremental TPS Support
**Current load test**: `parallel-loadtest.go` doesn't support:
- Starting at specific TPS and incrementing
- Running for fixed duration then stopping
- Clean metrics output for parsing

**Needed changes**:
- Add `-initial-tps` flag
- Add `-tps-increment` flag  
- Add `-increment-interval` flag
- Parse and output structured metrics (CSV-compatible)

### 4. Metrics Collection Improvements
**Current**: Script attempts to read CPU/memory but doesn't collect them properly.

**Needed**:
- Hook into docker stats output
- Parse latency percentiles from load test output
- Aggregate metrics per configuration

### 5. Results Parsing and Report Generation
**Current**: Script assumes grep patterns that may not exist in actual load test output.

**Needed**:
- Validate output format from parallel-loadtest.go
- Create robust parsing logic
- Handle edge cases (test timeout, network failure, etc.)

---

## Critical Path to Get Tests Running

### Immediate (required before tests can run):
1. **Commit DAG-BFT fix** - needed for sharding to actually be enabled
2. **Create multi-validator Dockerfiles** - current setup only has single bootstrap + single validator
3. **Modify parallel-loadtest.go** - needs incremental TPS support

### Next (needed for accurate results):
4. Fix load test to wait for account funding confirmation (reduce false errors)
5. Add metrics collection hooks
6. Improve results parsing

---

## Timeline Estimate

| Task | Est. Time | Blocker? |
|------|-----------|----------|
| Commit DAG-BFT fix | 15 min | YES |
| Multi-validator Docker setup | 2-3 hours | YES |
| Load test incremental TPS | 1-2 hours | YES |
| Metrics collection | 1-2 hours | NO (can run without) |
| Report generation refinement | 1 hour | NO |

**Total**: 6-8 hours to get tests fully running unattended

---

## How to Run When Ready

```bash
# From test/docker directory
./run-performance-suite.sh

# Results in:
# - ./performance-results/suite-TIMESTAMP.log (full execution log)
# - ./performance-results/{A1,A2,B1,B2,C1,C2}-results.csv (per-config metrics)
# - ./performance-results/PERFORMANCE-RESULTS-RC-v1.5.1.md (summary report)
```

---

## Notes for Implementation

1. **DAG-BFT fix is critical**: Without it, sharding remains disabled and we can't measure actual sharded performance
2. **Docker setup is complex**: Need to generate proper configs for 3/4 validators × 1/2/3 BVNs combinations
3. **Load test needs refinement**: Current behavior doesn't support the incremental stepping we need
4. **Unattended operation**: Once complete, runs fully automatically - takes ~6-8 hours total for all 6 configs

---

## Agent Work Completed

**performance-validator** (idle):
- ✅ Identified BPT sharding was disabled in DAG-BFT path
- ✅ Found DAG-BFT consensus is the bottleneck (not executor)
- ✅ CPU underutilization (2%) indicates headroom for optimization
- ⚠️ DAG-BFT fix applied but not yet committed

**Remaining**: Script orchestration + Docker/load test modifications
