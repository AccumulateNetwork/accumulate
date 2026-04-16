# Performance Test Orchestration Plan - Issue #3905

## Objective

Validate RC v1.5.1-breaking readiness through automated, unattended performance testing of 6 validator/BVN configurations with incremental TPS load from 1K-15K, capturing comprehensive metrics for analysis and debugging.

---

## Design

### Modular Python Architecture

Instead of shell scripts, we use Python modules that each own their responsibility:

```
test_orchestrator.py        ← Entry point, orchestrates full suite
├─ test_config.py           ← Test definitions (6 configs, TPS sequence)
├─ docker_generator.py      ← Dynamic docker-compose generation
├─ docker_manager.py        ← Docker Compose operations + health checks
├─ load_test_runner.py      ← parallel-loadtest.go wrapper
├─ results_aggregator.py    ← CSV aggregation + report generation
├─ failure_reporter.py      ← Failure capture for debugging
├─ monitor.py               ← Per-node metrics collection
├─ metrics-server.py        ← Real-time dashboard + JSON API
└─ dashboard.html           ← Web UI (Chart.js, auto-refresh)
```

**Why Python?** Cleaner control flow, no bash escaping hell, testable units.

### Test Matrix

6 configurations × 8 TPS levels = 48 individual test steps

| Config | Validators | BVNs | Total Nodes | Duration |
|--------|-----------|------|-------------|----------|
| A1 | 3 | 1 | 4 | ~15 min |
| A2 | 4 | 1 | 5 | ~15 min |
| B1 | 3 | 2 | 7 | ~15 min |
| B2 | 4 | 2 | 8 | ~15 min |
| C1 | 3 | 3 | 10 | ~15 min |
| C2 | 4 | 3 | 11 | ~15 min |

TPS sequence per config:
```
[1000, 2000, 3000, 5000, 7000, 10000, 12000, 15000]
```

Duration: 112 seconds per TPS level (2 min per level, ~15 min per config)

Stop condition: Error rate > 5% (pushback detected) OR reach 15K TPS

---

## Components

### 1. Test Configuration (`test_config.py`)

Defines test matrix and execution parameters.

```python
TEST_CONFIGS = [
    TestConfig("A1", "Single-BVN-3-Validators", 3, 1),
    TestConfig("A2", "Single-BVN-4-Validators", 4, 1),
    # ... 4 more configs
]

TPS_SEQUENCE = [1000, 2000, 3000, 5000, 7000, 10000, 12000, 15000]
INCREMENT_DURATION_SECONDS = 112  # ~2 min per level
ERROR_THRESHOLD = 0.05  # 5% error triggers pushback
```

### 2. Docker Compose Generation (`docker_generator.py`)

Dynamically generates docker-compose files for each configuration.

**Input:** TestConfig(validators=3, bvns=1)

**Output:** 
```
docker-compose-3-val-1-bvn.yml
docker-network-3-val-1-bvn.yml
```

**Generated compose file:**
- Bootstrap service (peer discovery)
- Init service (network setup)
- Validator services (one per node)
- Real accumulated daemon execution (NOT stubs)

### 3. Docker Management (`docker_manager.py`)

Encapsulates Docker operations with proper error handling.

```python
# Methods:
run()              # Execute docker command, return (rc, stdout, stderr)
cleanup()          # Aggressive cleanup (containers, volumes, networks)
verify_clean()     # Ensure no lingering state
compose_up()       # Build and start network
compose_down()     # Stop network
wait_healthy()     # Wait for JSON-RPC responsiveness
get_logs()         # Fetch container logs
verify_shards()    # Confirm 64-shard execution
ps()               # Get docker compose ps output
```

**Key feature:** `wait_healthy()` checks JSON-RPC responses, not just container status.

### 4. Load Test Orchestration (`load_test_runner.py`)

Wrapper around `parallel-loadtest.go` with proper metrics parsing.

```python
# Runs: go run parallel-loadtest.go -start-tps 10000 -min-tps 10000 ...
# Parses metrics from stdout:
#   Submitted: 12345
#   Success: 12300
#   Failed: 45
#   Average TPS: 9876.5
```

**Metrics extracted:**
- submitted, success, failed, error_rate
- actual_tps, p50_latency, p99_latency (when available)

### 5. Results Aggregation (`results_aggregator.py`)

Collects per-config metrics and generates reports.

**Outputs:**
- CSV per config: `{test_id}-results.csv`
- Summary report: `PERFORMANCE-RESULTS-RC-v1.5.1.md`

**Analysis includes:**
- Per-BVN scaling comparison
- Stable range identification (error < 1%)
- RC readiness assessment (8K+ TPS for single-BVN)

### 6. Failure Capture (`failure_reporter.py`)

Captures logs and state when tests fail for debugging team.

**Captures:**
- Docker Compose logs (last 500 lines)
- Container status (docker compose ps)
- Node states
- Recent error messages

**Output:** `failures/{test_id}-{stage}.txt`

### 7. Real-Time Dashboard System

Three-component system for live monitoring:

#### **7a. Node Monitor (`monitor.py`)**
- Runs continuously in background
- Samples Docker container stats every N seconds
- Writes CSV files per config
- Tracks CPU%, memory, database size per node

#### **7b. Metrics Server (`metrics-server.py`)**
- HTTP server on port 8888
- Reads load test log (real-time)
- Reads monitoring CSVs
- Calculates sliding-window TPS
- Serves JSON `/metrics` endpoint
- Hosts dashboard HTML

#### **7c. Dashboard Frontend (`dashboard.html`)**
- Chart.js charts
- Auto-refresh every 1 second
- Displays:
  - TPS (1-min, 5-min, 15-min, total)
  - Transaction counts (submitted, success, failed)
  - Per-node resources (CPU%, memory, database)
  - System utilization (cores, memory, growth rate)

**URL:** http://localhost:8888/

---

## Execution Flow

### Pre-Execution

1. Clean Docker state (aggressive wipe)
2. Create results directory

### Per Configuration Loop

```
For each config in [A1, A2, B1, B2, C1, C2]:

  1. Generate docker-compose-{V}val-{B}bvn.yml
  2. Generate docker-network-{V}val-{B}bvn.yml
  
  3. Wipe Docker state
  
  4. Start Docker Compose network
     - Build images
     - Start bootstrap, init, validators
  
  5. Wait for network health
     - Poll JSON-RPC until responsive
     - Timeout: 120 seconds
  
  6. Verify 64-shard execution
     - Check logs for: "shard-count=64 sharding=ENABLED"
     - Log warning if not found
  
  7. Run TPS sequence:
     For each tps in [1000, 2000, ..., 15000]:
         - Run load test at TPS for 112 seconds
         - Parse metrics (submitted, success, failed, error_rate)
         - Check for pushback (error_rate > 5%)
         - If pushback: STOP and record pushback_tps
         - Else: continue to next TPS level
  
  8. Generate CSV for config
     - Columns: TPS_TARGET, SUBMITTED, SUCCESS, FAILED, ERROR_RATE_PCT, ACTUAL_TPS, P50_LATENCY_MS, P99_LATENCY_MS, PUSHBACK
  
  9. Stop network
  
  10. Cleanup Docker (aggressive)
```

### Post-Execution

1. Generate summary report (PERFORMANCE-RESULTS-RC-v1.5.1.md)
2. Print test summary (passed/failed counts)
3. If failures: generate FAILURES-SUMMARY.txt
4. Cleanup resources

---

## Data Collection

### Load Test Output

Each load test outputs progress:
```
Accumulate Load Test
====================
Workers: 32 (4 nodes x 8 workers/node)
Generating worker accounts...
Progress: submitted=1234 success=1200 failure=34 elapsed=30s tps_1min=41.13 tps_total=41.13 target=1000
Progress: submitted=2468 success=2400 failure=68 elapsed=60s tps_1min=41.13 tps_total=41.13 target=1000
...
Final Results:
Submitted: 6240
Success: 6097
Failed: 143
Error rate: 0.0229% (143/6240)
Average TPS: 1100.0
```

### Dashboard Metrics

Real-time JSON response from `/metrics`:
```json
{
  "tps_1min": 1100,
  "tps_5min": 1050,
  "tps_15min": 1020,
  "tps_total": 1000,
  "total_submitted": 112000,
  "total_succeeded": 111800,
  "total_failed": 200,
  "error_rate": 0.0018,
  "cpu_cores": 12.5,
  "memory_gb": 2.3,
  "db_size_gb": 0.45,
  "nodes": [
    {"name": "acc-bvn1-val1", "cpu": 45.2, "memory_mb": 256, "db_size_mb": 42},
    ...
  ]
}
```

### Aggregated Results

CSV per configuration:
```
TPS_TARGET,SUBMITTED,SUCCESS,FAILED,ERROR_RATE_PCT,ACTUAL_TPS,P50_LATENCY_MS,P99_LATENCY_MS,PUSHBACK
1000,6240,6097,143,0.23,1100.0,12.3,45.2,FALSE
2000,12480,12194,286,0.23,2200.0,12.5,48.1,FALSE
...
15000,93600,91400,2200,2.35,16500.0,85.2,450.0,TRUE
```

---

## Success Criteria

### For Each Configuration

- ✅ All 8 TPS levels tested
- ✅ Metrics collected at each level
- ✅ Pushback point identified (if any)
- ✅ CSV report generated

### For Test Suite

- ✅ All 6 configurations complete
- ✅ Summary report generated
- ✅ No Docker state leakage between tests
- ✅ Performance degradation tracked

### For RC Readiness

**Single-BVN deployment ready if:**
- A1 or A2: Max sustained TPS ≥ 8000

**Multi-BVN deployment ready if:**
- C1 or C2: Per-BVN TPS ≥ 3000 (total 9000+ TPS)

---

## Dashboard Integration

**Startup sequence:**
```bash
# Terminal 1: Start orchestrator
python3 test_orchestrator.py

# Terminal 2: Start monitoring (inside test_orchestrator)
python3 monitor.py /tmp/monitoring-results 7200 10

# Terminal 3: Start metrics server (inside test_orchestrator)
python3 metrics-server.py /tmp/perf-test.log /tmp/monitoring-results 8888

# Browser: Open dashboard
# http://localhost:8888/
```

Dashboard auto-updates every second, showing:
- Current TPS across all levels
- Per-node CPU/memory during test
- Success rate and error trends
- Database growth rate

---

## Error Handling

### Network Startup Failures

**Captured:** Docker build errors, service health timeouts, container crashes

**Action:** Log failure, capture logs, skip to next config, continue suite

**Output:** `failures/{config_id}-{stage}.txt`

### Load Test Failures

**Captured:** Parallel-loadtest.go exit code, missing metrics, parse errors

**Action:** Log error, continue to next TPS level (don't fail entire config)

**Output:** Missing metrics in CSV (marked as error in log)

### Docker State Issues

**Prevention:** Aggressive cleanup before each test
- Stop containers with `docker stop`
- Remove containers with `docker rm -f`
- Prune volumes, networks, dangling images
- Clear temp directories

**Verification:** `verify_clean()` confirms no lingering state

---

## Files Generated

### Per-Configuration
- `docker-compose-{V}val-{B}bvn.yml` — Docker Compose file
- `docker-network-{V}val-{B}bvn.yml` — Network config
- `{test_id}-results.csv` — TPS and metrics per level

### Suite-Level
- `PERFORMANCE-RESULTS-RC-v1.5.1.md` — Summary report
- `FAILURES-SUMMARY.txt` — Failure details (if any)
- `suite-YYYYMMDD-HHMMSS.log` — Full execution log

### Monitoring (Real-Time)
- `/tmp/monitoring-results/per-node-resources.csv` — CPU, memory per node
- `/tmp/monitoring-results/per-node-database.csv` — Database size per node
- `/tmp/monitoring-results/cluster-summary.csv` — Cluster-wide stats

---

## Validation

### Pre-Test
- ✓ All Python modules importable
- ✓ parallel-loadtest.go compiles
- ✓ Docker Compose available
- ✓ Ports 8888, 16593, 26660-26671 available

### Post-Test
- ✓ All 6 configs completed (or failed with reason)
- ✓ Results directory populated
- ✓ Summary report generated
- ✓ Docker cleaned (no orphan containers)

---

## Timelines

**Typical run: ~90 minutes total**
- Pre-suite cleanup: 5 min
- Per config: ~15 min × 6 = 90 min
- Post-suite cleanup + report: 5 min

**Total with overhead: ~100 minutes unattended**

---

## Last Updated

April 14, 2026 — Initial orchestration design for RC v1.5.1-breaking
