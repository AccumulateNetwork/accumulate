# Issue #3905 Test Failure Debugging - Team Coordination Plan

## Overview

When the performance test orchestration runs unattended for 6-8 hours and tests fail, a coordinated team approach identifies root causes and applies fixes. This plan defines how to organize debugging work across different specialists.

---

## Team Structure

### Team Name: `v1.5.1-rc-debugging`

Six specialist agents, each handling a category of failures:

#### 1. **network-debugger**
- **Expertise**: Docker networking, container startup, port exposure, DNS
- **Handles**: Network startup failures, container won't boot, port conflicts
- **Tools**: docker ps, docker logs, docker inspect, docker network ls
- **Checklist**:
  - [ ] Docker daemon is running and healthy
  - [ ] Port mappings are correct (26660, 16593, etc.)
  - [ ] Network bridge (acc-network) created successfully
  - [ ] All containers can reach each other
  - [ ] External ports exposed for load test access
  - [ ] No port conflicts from previous tests

#### 2. **sharding-debugger**
- **Expertise**: BPT sharding, executor config, logging, database initialization
- **Handles**: Missing sharding logs, shard count verification fails, ShardedBPT issues
- **Tools**: grep for shard-count, check database/executor_config.go, examine logs
- **Checklist**:
  - [ ] Logs show `Executor initialized shard-count=64 sharding=ENABLED` for each validator
  - [ ] BPT sharding fix in cmd/accumulated/run/dagbft.go is applied
  - [ ] ExecutorConfig.ExecutorShardCount is 64 in database
  - [ ] No fallback to sequential execution logged
  - [ ] ShardedBPT instances created (not plain BPT)

#### 3. **load-test-debugger**
- **Expertise**: Load test tool, transaction submission, account creation, timing
- **Handles**: Load test won't submit, 0% success, account creation issues
- **Tools**: parallel-loadtest.go, curl to API endpoints, transaction logs
- **Checklist**:
  - [ ] Load test can connect to API endpoints
  - [ ] Account creation transactions execute successfully
  - [ ] Funding transactions are confirmed before sending from accounts
  - [ ] Worker accounts have sufficient balance
  - [ ] Transaction submission rate ramps correctly
  - [ ] Metrics are being collected

#### 4. **consensus-debugger**
- **Expertise**: DAG-BFT consensus, block production, mempool, throughput limits
- **Handles**: TPS plateaus, block production stalls, mempool congestion
- **Tools**: Docker logs for consensus messages, metrics analysis, block explorer
- **Checklist**:
  - [ ] Blocks are being produced (check "Block produced" logs)
  - [ ] Block production rate matches TPS / tx-per-block
  - [ ] Consensus rounds complete without timeouts
  - [ ] Mempool doesn't show stuck transactions
  - [ ] CPU available (not maxed out)
  - [ ] Memory available (not swapping)

#### 5. **memory-debugger**
- **Expertise**: Memory management, goroutines, resource leaks, GC tuning
- **Handles**: Memory runaway, OOM errors, goroutine leaks
- **Tools**: Docker stats, pprof profiles, goroutine dumps
- **Checklist**:
  - [ ] Memory growth is linear, not exponential
  - [ ] Goroutine count stabilizes (not growing unbounded)
  - [ ] No obvious leaks in code (especially batch handling, channel drains)
  - [ ] GC pause times are acceptable
  - [ ] No resource exhaustion errors in logs

#### 6. **cleanup-debugger**
- **Expertise**: Docker cleanup, volume management, resource reclamation
- **Handles**: Docker cleanup fails, lingering containers/volumes
- **Tools**: docker ps -a, docker volume ls, docker system df
- **Checklist**:
  - [ ] No accumulate containers remain after cleanup
  - [ ] No accumulate volumes remain after cleanup
  - [ ] No accumulate networks remain after cleanup
  - [ ] /tmp/accumulate-* directories deleted
  - [ ] Cleanup script completes without errors
  - [ ] Next test can start cleanly

---

## Failure Response Workflow

### 1. Test Fails (Automatic)

Orchestration script detects failure:
```
test A1 (3 validators, 1 BVN) FAILED
Error: "Network failed to start after 30s"
Captured logs: test/docker/performance-results/FAILED-A1-20260414-031542.json
```

### 2. Create Debugging Task

Create task in v1.5.1-rc-debugging team:

```
[User]: "Test A1 failed - network startup. Assign to network-debugger for investigation."
```

This creates:
```
- Task #1: Investigate FAILED-A1-20260414-031542
  - Owner: network-debugger
  - Category: network
  - Inputs: logs, docker ps output, timestamps
  - Expected output: Root cause analysis + fix recommendation
```

### 3. Agent Investigates

**network-debugger** analyzes failure:

```
1. Read captured logs
   - "Docker network creation failed: address already in use"
   
2. Determine root cause
   - Previous test's cleanup incomplete
   - Stale bridge interface lingering
   
3. Identify fix
   - Add more aggressive interface cleanup
   - Verify with `docker network ls` before creating
   
4. Report findings
   - Message team: "Found stale network from previous test. 
     Fix: Add explicit interface removal in docker_cleanup()"
```

### 4. Apply Fix

**network-debugger** implements fix:

```bash
# Option A: Fix in orchestration script
# Edit run-performance-suite.sh:
# Add: ip link del br-<id> (extract from docker network ls)

# Option B: Fix in Docker cleanup
# Improve docker_cleanup() function to handle stale bridges

git add test/docker/run-performance-suite.sh
git commit -m "Fix Issue #3905 failure: Remove stale Docker bridges in cleanup"
```

### 5. Verify Fix

**network-debugger** tests fix:

```bash
# Run just test A1 with the fix
./run-performance-suite.sh --single A1

# If successful:
# "Network started (3 validators healthy)"
# Report to team: "Fix verified, test A1 now passes"
```

### 6. Re-run Full Suite

If fix is successful and general (applies to all tests):

```bash
# Run full suite again with fix applied
./run-performance-suite.sh

# Monitor output for same failure mode
# If all tests now pass: mark as fixed
# If some still fail: investigate further
```

---

## Handling Multiple Failures

If multiple tests fail with different errors:

**Parallel Investigation** (use teams efficiently):

```
Task #1: Investigate FAILED-A1 (network)     → network-debugger
Task #2: Investigate FAILED-B2 (sharding)    → sharding-debugger  
Task #3: Investigate FAILED-C1 (consensus)   → consensus-debugger
```

Teams work in parallel, then coordiate:

```
network-debugger: "A1 fix is ready, apply to script"
sharding-debugger: "B2 needs code change, waiting on user review"
consensus-debugger: "C1 may require significant changes, blocked pending investigation"
```

**Coordination meeting** (via tasks/messages):
- What fixes are ready to apply?
- Which require code review?
- What's blocked and why?
- Apply fixes in dependency order
- Re-test full suite

---

## Common Debugging Scenarios

### Scenario 1: "Network Failed to Start"

**Likely owners**: network-debugger + cleanup-debugger

**Steps**:
1. Check if previous test's cleanup completed
   - `docker ps -a | wc -l` (should be 0-2)
   - `docker volume ls | wc -l` (should be minimal)
2. If lingering resources: improve cleanup
3. If cleanup is fine: check Docker daemon health
   - `docker info` 
   - `docker system df`
4. If daemon is fine: check network bridge/port availability
   - `docker network ls`
   - `netstat -tlnp | grep 26660`

**Fix options**:
- Option A: More aggressive cleanup (safest)
- Option B: Explicit port release between tests
- Option C: Use different port ranges per config

### Scenario 2: "No shard-count=64 in logs"

**Owner**: sharding-debugger

**Steps**:
1. Verify BPT sharding fix is applied to dagbft.go
2. Check if ExecutorConfig is being loaded in daemon startup
3. Look for error logs that might suppress the shard logs
4. Check if startup logs are being captured (not truncated)

**Fix options**:
- Option A: Apply BPT sharding fix (if missing)
- Option B: Check database initialization (shard count not persisted?)
- Option C: Add more detailed logging

### Scenario 3: "All transactions fail: Account.Main not found"

**Owner**: load-test-debugger

**Steps**:
1. Check if account funding completed successfully
2. Look at timing: are we sending too fast?
3. Check if funded accounts have balance
4. Look for any account creation errors in logs

**Fix options**:
- Option A: Increase wait-for-funding duration (easiest)
- Option B: Make load test poll for account confirmation
- Option C: Improve account setup in test infrastructure

### Scenario 4: "TPS plateaus at 2K"

**Owner**: consensus-debugger

**Steps**:
1. Check blocks are being produced (grep for "Block committed")
2. Calculate actual throughput: blocks/min × tx/block = TPS
3. Check if block size is limiting (too few tx per block)
4. Check CPU/memory (is there headroom?)
5. Check if consensus is stalling (timeout logs)

**Fix options**:
- Option A: Increase block size limit
- Option B: Increase consensus round speed
- Option C: Investigate why consensus is slow (lock contention? CPU throttle?)

---

## Communication Protocol

### Agent Reports Status

When investigating a failure:

```
[network-debugger]: Investigating FAILED-A1
- Root cause found: stale Docker bridge from previous test
- Fix: Add 'docker network prune -f' to cleanup
- Status: Implementing fix
- ETA: 15 minutes
```

### Report Findings

When investigation complete:

```
[network-debugger]: FAILED-A1 Investigation Complete

ROOT CAUSE:
- Previous test cleanup didn't fully remove Docker network
- Bridge interface remained, port 26660 still bound

RECOMMENDATION:
- Apply explicit: docker network prune -f --filter "label!=keep"
- Add 5-second wait after prune for OS to release ports

RISK: Low (only affects cleanup, not core logic)

NEXT STEPS:
- Apply fix to run-performance-suite.sh
- Re-run test A1 for verification
- If passes: include in full suite re-run
```

### Request Help If Blocked

If investigation stalls:

```
[memory-debugger]: Blocked on FAILED-C2 (memory runaway)
- Memory growing 2% per minute in validator 1
- Goroutines stable, no obvious leaks in logs
- Need help: Can someone profile the heap during load test?
- Blocking: Can't fix without data

REQUESTING: 
- Quick memory profile during failed test
- Or: Enable pprof endpoint in accumulated binary
```

---

## Team Operations

### Before Tests Start

1. Create team: `teams create v1.5.1-rc-debugging`
2. Brief team on failure modes and responsibilities
3. Ensure all agents have:
   - Read access to test/docker directory
   - Write access to performance-results/
   - Ability to read git history and make commits
   - Access to Docker (docker ps, docker logs, etc.)

### During Tests (Unattended)

1. Orchestration script runs
2. On failure: captures logs automatically
3. Users monitors progress (optional)

### On First Failure

1. User reviews captured logs
2. Assigns to appropriate specialist agent
3. Agent investigates and reports findings
4. User applies fix or coordinates further investigation

### After All Tests Complete

1. Review all failures and fixes
2. Commit fixes to issue-3906-v1.5.1-breaking branch
3. Plan re-test with all fixes applied
4. Re-run full suite to validate

---

## Success Criteria

✅ Debugging process successful if:

- [ ] All test failures have identified root causes
- [ ] Root causes are documented
- [ ] Fixes have been identified for each failure
- [ ] Fixes have been tested and verified
- [ ] No new failures introduced by fixes
- [ ] Full test suite passes with all fixes applied
- [ ] Team coordination was efficient (no blockers)

---

## Failure Documentation Template

For each failure, document:

```markdown
## Failure: Test A1 - Network Startup

**Date/Time**: 2026-04-14 03:15:42
**Test Config**: 3 validators, 1 BVN
**Error Message**: "Network failed to start after 30s"

**Investigating Agent**: network-debugger
**Root Cause**: Stale Docker bridge from previous test cleanup
**Severity**: High (blocks test execution)

**Fix Applied**: 
- File: test/docker/run-performance-suite.sh
- Change: Add `docker network prune -f` in docker_cleanup()
- Commit: abc123def456

**Verification**:
- [x] Fix tested on A1: PASSES
- [x] Fix applied to full suite
- [x] Full suite re-run: 6/6 tests pass
- [ ] No regressions detected

**Lessons Learned**:
- Docker cleanup needs explicit network prune
- Should verify network clean state before starting test
```

This creates a log of all failures and how they were resolved.
