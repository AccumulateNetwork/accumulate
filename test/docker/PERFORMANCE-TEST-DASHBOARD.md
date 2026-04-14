# Accumulate Performance Test Dashboard

## Overview

The performance test dashboard provides **real-time monitoring** of load test execution across all validator nodes in a DAG-BFT network. It displays live TPS (transactions per second), per-node resource utilization, transaction success/failure metrics, and database growth rates.

**Live Dashboard:** http://localhost:8888/

---

## Architecture

The dashboard is a distributed system with three components:

### 1. Metrics Server (`metrics-server.py`)

**Purpose:** Collects metrics from load tests and monitoring, serves them via JSON API and HTTP

**Responsibilities:**
- Parses real-time load test output from `parallel-loadtest.go`
- Reads per-node metrics from `monitor.py` CSV files
- Calculates sliding-window TPS (1-min, 5-min, 15-min, total)
- Serves metrics via `/metrics` JSON endpoint
- Serves dashboard HTML UI at `/`

**Startup:**
```bash
python3 metrics-server.py <loadtest_log> <monitoring_dir> <port>
```

Example:
```bash
python3 metrics-server.py /tmp/perf-test.log /tmp/monitoring-results 8888
```

**API Endpoints:**
- `GET /` — Returns dashboard HTML (dashboard.html)
- `GET /metrics` — Returns JSON metrics object
- `GET /reset` — Resets metrics state (for new test run)

### 2. Node Monitor (`monitor.py`)

**Purpose:** Collects per-node CPU, memory, disk, and database metrics

**Responsibilities:**
- Samples Docker container stats every `<interval>` seconds
- Measures database size per validator
- Calculates resource utilization percentages
- Writes CSV files to monitoring directory
- Runs continuously in background during tests

**Startup:**
```bash
python3 monitor.py <output_dir> <duration_seconds> <interval_seconds>
```

Example (monitor for 1 hour, sample every 10s):
```bash
python3 monitor.py /tmp/monitoring-results 3600 10 &
```

**Output Files (CSV):**
- `per-node-resources.csv` — CPU%, memory, network I/O per node
- `per-node-database.csv` — Database size per node
- `cluster-summary.csv` — Cluster-wide aggregates

### 3. Load Test Runner (`parallel-loadtest.go` + `load_test_runner.py`)

**Purpose:** Generates transaction load at target TPS

**Responsibilities:**
- Creates worker accounts and funds them
- Submits transactions at fixed target TPS
- Detects pushback (error rate > threshold)
- Outputs progress every ~1s: `Progress: submitted=X success=Y failure=Z target=TPS`
- Logs output to file for metrics-server to consume

**Run via orchestrator:**
```bash
python3 load_test_runner.py 10000 112  # 10K TPS for 112 seconds
```

---

## Dashboard Display

### Real-Time Metrics Cards

**Top row (cluster-wide):**
- **TPS (1 min)** — Last 60 seconds average
- **TPS (5 min)** — Last 300 seconds average  
- **TPS (15 min)** — Last 900 seconds average
- **TPS (Total)** — Overall average since test start

**Transaction metrics:**
- **Total Submitted** — Cumulative transaction count
- **Total Success** — Successful transactions
- **Total Failed** — Failed transactions (error count)
- **Target TPS** — Current target load

**Resource metrics:**
- **CPU Cores** — Total CPU cores in use (aggregate)
- **Memory GB** — Total memory used by all nodes
- **Database GB** — Total database size across cluster
- **Active Accounts** — Load test worker accounts

### Per-Node Grid

12 validator cards displayed in grid (3 BVNs × 4 validators):
- **Node name** — Container identifier (e.g., `acc-bvn1-val1`)
- **CPU %** — Per-node CPU utilization
- **Memory MB** — Per-node memory usage
- **DB MB** — Per-node database size

### Raw Data Section

Shows raw JSON metrics response for debugging (visible on dashboard)

---

## Data Flow

```
┌──────────────────────────────────────────────────────────────────┐
│ Load Test (parallel-loadtest.go)                                 │
│ Outputs: Progress: submitted=X success=Y failure=Z target=TPS   │
└──────────────┬───────────────────────────────────────────────────┘
               │
               ├─→ /tmp/perf-test.log (or user-specified log file)
               │
┌──────────────▼───────────────────────────────────────────────────┐
│ Monitor (monitor.py)                                             │
│ Samples Docker stats every <interval> seconds                    │
└──────────────┬───────────────────────────────────────────────────┘
               │
               ├─→ /tmp/monitoring-results/per-node-resources.csv
               ├─→ /tmp/monitoring-results/per-node-database.csv
               │
┌──────────────▼───────────────────────────────────────────────────┐
│ Metrics Server (metrics-server.py)                               │
│ Parses logs and CSVs, calculates sliding-window TPS              │
└──────────────┬───────────────────────────────────────────────────┘
               │
               ├─→ GET /metrics (JSON)
               │
┌──────────────▼───────────────────────────────────────────────────┐
│ Dashboard HTML (dashboard.html)                                  │
│ Browser-side: fetch /metrics every 1s, render charts            │
└──────────────────────────────────────────────────────────────────┘
               │
               └─→ http://localhost:8888/
```

---

## Typical Workflow

### 1. Start Network
```bash
cd test/docker
docker compose -f docker-compose.yml up -d
docker compose ps  # Verify all containers running
```

### 2. Start Monitoring (separate terminal)
```bash
mkdir -p /tmp/monitoring-results
python3 monitor.py /tmp/monitoring-results 3600 10 > /tmp/monitor.log 2>&1 &
```

### 3. Start Metrics Server (separate terminal)
```bash
python3 metrics-server.py /tmp/perf-test.log /tmp/monitoring-results 8888 > /tmp/metrics-server.log 2>&1 &
```

### 4. Open Dashboard
```
http://localhost:8888/
```

### 5. Run Load Test
```bash
# In test/docker or workspace with parallel-loadtest.go
go run parallel-loadtest.go > loadtest.log 2>&1 &
```

### 6. Watch Dashboard
- Refresh http://localhost:8888/
- Observe real-time TPS, CPU, memory, success rate
- Monitor for pushback (error rate spike)

---

## Metrics Calculations

### TPS (Transactions Per Second)

Calculated using **sliding-window** approach:

```
TPS_1min = (submitted_now - submitted_60s_ago) / 60 seconds
TPS_5min = (submitted_now - submitted_300s_ago) / 300 seconds
TPS_15min = (submitted_now - submitted_900s_ago) / 900 seconds
TPS_total = total_submitted / elapsed_seconds
```

**Why sliding windows?** 
- Smooths spiky per-second measurements
- Better reflects sustained throughput
- 1-min gives instant feedback; 15-min shows stability

### Resource Utilization

**Per-node CPU %:**
```
CPU_Percent = (docker stats CPU field) / (1.0) × 100
```

**Per-node Memory:**
```
Memory_MB = (docker stats memory usage in bytes) / 1024 / 1024
Memory_Percent = (Memory_MB / container_limit_MB) × 100
```

**Database Growth:**
```
DB_Growth_MB_per_min = (current_db_size - initial_db_size) / elapsed_minutes
```

---

## Performance Baselines

For reference (from prior 12-node 10.5K TPS test):

| Metric | Value |
|--------|-------|
| Sustained TPS | 10,500 |
| Success rate | 99.9999% |
| Total CPU used | 13.3 cores |
| Total memory | 3.3 GB |
| Per-node database | 1.32 MB |
| Per-node memory limit | 2.0 GB |

**Interpretation:**
- 87.5% of 12K target is achievable
- Bottleneck is network latency (HTTP), not compute
- Adding more workers (currently 48) can increase TPS further

---

## Troubleshooting

### Dashboard shows zeros
**Cause:** Monitoring not started or CSV files not yet written
**Fix:**
1. Verify monitor.py is running: `ps aux | grep monitor.py`
2. Check monitoring directory exists: `ls /tmp/monitoring-results`
3. Wait 30 seconds for first CSV sample to be written

### Dashboard disconnected (red status)
**Cause:** Metrics server not running or metrics endpoint unreachable
**Fix:**
```bash
# Check if server is running
ps aux | grep metrics-server

# Restart if needed
python3 metrics-server.py /tmp/perf-test.log /tmp/monitoring-results 8888 &

# Verify endpoint
curl http://localhost:8888/metrics
```

### TPS stuck at 0
**Cause:** Load test not running or not writing to log file
**Fix:**
1. Verify load test is running: `ps aux | grep loadtest`
2. Check log file exists and has recent entries: `tail -f /tmp/perf-test.log`
3. Ensure log path matches argument passed to metrics-server

### Memory usage seems low
**Cause:** Dashboard calculates total across all nodes; individual nodes shown in grid
**Context:** 
- Per-node limit is 2GB (in docker-compose)
- 12 nodes = 24GB cluster limit
- 3.3GB total = 13.8% utilization (plenty of headroom)

---

## Integration with Orchestrator

The performance test orchestrator (`test_orchestrator.py`) uses the dashboard system automatically:

```python
# Start metrics collection
monitor = subprocess.Popen([
    'python3', 'monitor.py', 
    '/tmp/monitoring-results', '7200', '10'
])

# Start dashboard
metrics_server = subprocess.Popen([
    'python3', 'metrics-server.py',
    '/tmp/perf-test.log', '/tmp/monitoring-results', '8888'
])

# Run load tests (output to /tmp/perf-test.log)
# Dashboard automatically reads and displays metrics
```

---

## Files

| File | Purpose |
|------|---------|
| `dashboard.html` | Frontend UI (Chart.js, auto-refresh every 1s) |
| `metrics-server.py` | Backend metrics API server |
| `monitor.py` | Node monitoring agent (Docker stats → CSV) |
| `parallel-loadtest.go` | Transaction generator |
| `load_test_runner.py` | Orchestrator for load test execution |
| `test_orchestrator.py` | Runs all 6 configurations sequentially |

---

## Last Updated

April 14, 2026 — Performance test orchestration for RC v1.5.1-breaking
