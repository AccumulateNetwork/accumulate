# Issue #3905 Performance Test Orchestration

Python-based orchestration for running all 6 performance test configurations unattended.

## Architecture

Clean, modular Python implementation:

- **test_config.py** - Test configurations (3/4 validators × 1/2/3 BVNs, TPS sequences)
- **docker_manager.py** - Docker operations (start, stop, cleanup, health checks, shard verification)
- **failure_reporter.py** - Structured failure capture with detailed diagnostics for debugging team
- **test_orchestrator.py** - Main orchestration engine (ties everything together)

## Quick Start

```bash
cd test/docker

# Run the full test suite (unattended, ~6-8 hours)
python3 test_orchestrator.py

# Results saved to: performance-results/
```

## What It Does

1. **Pre-suite cleanup**: Wipe all Docker state
2. **For each of 6 test configurations**:
   - Clean Docker state
   - Start network (X validators, Y BVNs)
   - Verify all containers healthy
   - Verify 64-shard execution enabled
   - Run TPS incremental sequence (1K → 12K TPS)
   - Capture metrics at each TPS level
   - Clean up network
3. **Post-suite cleanup**: Leave system clean
4. **Generate summary report** with results and any failures

## Failure Handling

If a test fails:

1. Detailed failure report is captured automatically:
   - Docker container state
   - Docker Compose logs (last 300 lines)
   - System state (disk, memory, load)
   - Timestamp and error message

2. Failure report saved to: `performance-results/FAILED-<test-id>-TIMESTAMP.txt`

3. Summary printed at end showing:
   - Which tests failed
   - Failure categories (network, sharding, consensus, etc.)
   - Where to find detailed reports
   - Instructions for debugging team

## Debugging Team Integration

When tests fail, debugging team can investigate:

```bash
# Create debugging team
teams create v1.5.1-rc-debugging

# Assign failures to specialists
teams assign network-debugger FAILED-A1-20260414-031542.txt

# Each specialist uses the detailed report to:
# - Identify root cause
# - Implement fix
# - Re-test to verify
# - Report back with findings
```

See `.claude/plans/issue-3905-debugging-team-coordination.md` for full team process.

## Results

After tests complete:

```
performance-results/
├── suite-YYYYMMDD-HHMMSS.log     # Full execution log
├── A1-results.csv                # Metrics for each TPS level
├── A2-results.csv
├── B1-results.csv
├── B2-results.csv
├── C1-results.csv
├── C2-results.csv
├── PERFORMANCE-RESULTS-RC-v1.5.1.md  # Summary report
├── FAILED-A1-20260414-031542.txt     # Failure reports (if any)
├── FAILED-B2-20260414-032015.txt
└── FAILURES-SUMMARY.txt          # Summary of all failures
```

## Key Features

✅ **Modular design** - Easy to test, extend, and debug each component
✅ **Structured failure capture** - Comprehensive diagnostics for debugging team
✅ **Aggressive Docker cleanup** - No state bleed between tests
✅ **Shard verification** - Confirms 64-shard execution enabled
✅ **Unattended execution** - Runs completely automatically for 6-8 hours
✅ **Clear logging** - Progress visible in console and logs

## Dependencies

- Python 3.7+
- Docker
- Docker Compose
- Go build tools (to build accumulated binary)

## Configuration

Edit `test_config.py` to:
- Add/remove test configurations
- Change TPS sequence
- Adjust test duration or error thresholds

## Troubleshooting

**"No module named docker_manager"**
- Make sure you're running from test/docker directory
- Python path should include current directory

**"docker: command not found"**
- Ensure Docker is installed and in PATH
- Test: `docker --version`

**"Permission denied: test_orchestrator.py"**
- Already fixed: `chmod +x test_orchestrator.py`

## Next Steps

Phase 1: Implement load test integration
- Modify to call parallel-loadtest.go with TPS targets
- Parse metrics output
- Detect pushback (error rate > 5%)

Phase 2: Results aggregation
- Aggregate metrics across test configs
- Generate summary report with per-BVN throughput limits
- Create charts/graphs

Phase 3: Debugging team support
- Save failure reports in JSON format
- Create task list for debugging team
- Track which agents are investigating which failures

---

**Status**: Phase 0 (framework ready, load test integration pending)
