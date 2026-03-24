# How to Run Accumulate Load Tests

Complete guide for running high-throughput load tests on Accumulate DAG-BFT network.

---

## Quick Start

```bash
# 1. Build optimized accumulated binary
go build ./cmd/accumulated

# 2. Start 12-node test network (3 BVNs × 4 validators)
cd test/docker
docker compose up -d

# 3. Wait for nodes to be healthy (~30 seconds)
docker compose ps

# 4. Start monitoring + dashboard
python3 monitor.py /tmp/monitoring-results 3600 10 &
python3 metrics-server.py /tmp/loadtest.log /tmp/monitoring-results 8888 &

# 5. Build and run load test
cd /tmp/loadtest-workspace
go build -o loadtest parallel-10k-loadtest.go
./loadtest | tee loadtest.log

# 6. Open dashboard
# Visit: http://localhost:8888/
```

---

## Prerequisites

**Software:**
- Go 1.21+
- Docker + Docker Compose
- Python 3.8+

**System Requirements:**
- 24 CPU cores (or adjust memory limits)
- 32 GB RAM minimum
- 50 GB disk space
- Linux (tested on Ubuntu)

**Network:**
- All validator ports available (26660-26671)
- Bootstrap port available (16593)
- Dashboard port available (8888)

---

## Detailed Setup

### 1. Build Optimized Validator

The validators include performance optimizations:
- LRU batch eviction (62.5% CPU reduction)
- Bounded batch queue
- Vote spam protection

```bash
cd /home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate
go build ./cmd/accumulated

# Verify build
./cmd/accumulated/accumulated version
```

### 2. Start Test Network

**Option A: Standard 12-node network (recommended)**

```bash
cd test/docker
docker compose -f docker-compose.yml up -d
```

This starts:
- 1 bootstrap server (peer discovery)
- 12 validators (3 BVNs × 4 each)
- 2GB memory limit per validator

**Option B: Custom configuration**

Edit `test/docker/docker-network.yml`:
```yaml
network:
  id: test-network
  bvns: 3           # Number of BVNs
  validators: 4     # Validators per BVN

globals:
  oracle:
    price: 10000000  # 1000 * AcmeOraclePrecision
```

Then rebuild Docker images:
```bash
docker compose build
docker compose up -d
```

### 3. Verify Network Health

```bash
# Check all containers are running
docker compose ps

# Check bootstrap server
curl http://localhost:16593/status

# Check validator health
for port in {26660..26671}; do
  echo "Port $port:"
  curl -s http://localhost:$port/v3 | jq -r '.jsonrpc'
done
```

All should return `"2.0"` (JSON-RPC version).

### 4. Start Monitoring

**Monitoring Script:**
```bash
python3 test/docker/monitor.py <output_dir> <duration_seconds> <interval_seconds>
```

**Example: Monitor for 1 hour, sample every 10 seconds**
```bash
mkdir -p /tmp/monitoring-results
python3 test/docker/monitor.py /tmp/monitoring-results 3600 10 > /tmp/monitor.log 2>&1 &
MONITOR_PID=$!
echo "Monitor PID: $MONITOR_PID"
```

**Metrics collected:**
- Per-node CPU, memory, disk, network
- Per-node database size
- Cluster aggregates

### 5. Start Dashboard (Optional)

The real-time web dashboard provides live monitoring:

```bash
python3 test/docker/metrics-server.py \
  /tmp/loadtest.log \
  /tmp/monitoring-results \
  8888 > /tmp/dashboard.log 2>&1 &

DASHBOARD_PID=$!
echo "Dashboard PID: $DASHBOARD_PID"
echo "Dashboard: http://localhost:8888/"
```

### 6. Build Load Generator

**Copy the load test template:**
```bash
cp test/docker/parallel-loadtest.go /tmp/loadtest-workspace/
cd /tmp/loadtest-workspace
```

**Configure for your target TPS:**

Edit `parallel-loadtest.go`:
```go
const (
    totalWorkers = 48        // Adjust for target TPS
    targetTPS    = 12000     // Target transactions per second
    tpsPerWorker = targetTPS / totalWorkers
)
```

**Worker count guidelines:**
- 36 workers → ~8,800 TPS
- 48 workers → ~10,500 TPS
- 60 workers → ~12,500 TPS
- 72 workers → ~15,000 TPS

**Build:**
```bash
go build -o loadtest parallel-loadtest.go
```

### 7. Run Load Test

**Option A: Fixed duration (default 5 minutes)**
```bash
./loadtest 2>&1 | tee loadtest.log
```

**Option B: Continuous (until Ctrl+C)**

Edit the code to remove the 5-minute timer, then:
```bash
./loadtest 2>&1 | tee loadtest.log
```

**Option C: Background with monitoring**
```bash
./loadtest > loadtest.log 2>&1 &
LOADTEST_PID=$!

# Monitor progress
tail -f loadtest.log

# Stop when done
kill -SIGINT $LOADTEST_PID
```

### 8. Monitor Progress

**Load test output:**
```
Progress: submitted=100000 success=99998 failure=2 elapsed=10s actual_tps=10000.0
```

**Dashboard:** http://localhost:8888/
- Real-time TPS graph
- Per-node metrics
- Success rate
- Resource usage

**Manual checks:**
```bash
# CPU usage
docker stats --no-stream

# Database size
docker exec acc-bvn1-val1 du -sh /root/.accumulate

# Transaction count
curl -s http://localhost:26660/v3 | jq '.result.totalTransactions'
```

### 9. Stop Test

```bash
# Stop load test (if running in background)
kill -SIGINT $LOADTEST_PID

# Stop monitoring
kill -SIGTERM $MONITOR_PID

# Stop dashboard
kill -SIGTERM $DASHBOARD_PID

# Stop network
cd test/docker
docker compose down -v
```

### 10. Analyze Results

**Monitoring data:**
```bash
ls /tmp/monitoring-results/
# per-node-resources.csv
# per-node-database.csv
# cluster-summary.csv
# summary-report.txt

cat /tmp/monitoring-results/summary-report.txt
```

**Load test log:**
```bash
grep "Final Results" /tmp/loadtest.log -A 10
```

---

## Configuration Options

### Load Test Configuration

**In `parallel-loadtest.go`:**

```go
const (
    // Worker configuration
    totalWorkers      = 48    // Total concurrent workers
    workersPerNode    = 4     // Workers per validator

    // Performance tuning
    targetTPS         = 12000 // Target transactions per second
    accountsPerWorker = 10    // Pre-allocated accounts per worker

    // HTTP client settings (in main())
    MaxIdleConns:        200  // Total idle connections
    MaxIdleConnsPerHost: 20   // Idle connections per host
)
```

### Network Configuration

**In `test/docker/docker-network.yml`:**

```yaml
globals:
  executorVersion: "v2"

  oracle:
    price: 10000000  # Credit price (1000 * AcmeOraclePrecision)

  globals:
    majorBlockSchedule: "0 */12 * * *"  # Cron format

network:
  id: test-network
  bvns: 3           # Number of BVNs
  validators: 4     # Validators per BVN
```

### Memory Limits

**In `test/docker/docker-compose.yml`:**

```yaml
services:
  bvn1-val1:
    mem_limit: 2g    # Adjust per validator
```

For 10K TPS: 2GB is sufficient
For 15K+ TPS: Consider 4GB

---

## Troubleshooting

### Issue: Nodes won't start

**Symptoms:** Containers exit immediately or show errors

**Solutions:**
```bash
# Check logs
docker compose logs bootstrap
docker compose logs bvn1-val1

# Common causes:
# 1. Port conflicts - check if ports are in use
netstat -tuln | grep 26660

# 2. Memory limits - increase if system has insufficient RAM
docker compose down
# Edit docker-compose.yml, increase mem_limit
docker compose up -d

# 3. Stale data - clean everything
docker compose down -v
docker system prune -f
docker compose up -d
```

### Issue: Low TPS

**Symptoms:** Achieving < 8,000 TPS

**Solutions:**
```bash
# 1. Check if nodes are healthy
docker stats

# 2. Increase workers
# Edit parallel-loadtest.go, increase totalWorkers to 60-72

# 3. Check network latency
ping localhost

# 4. Verify HTTP connection pooling
# In load test code, check MaxIdleConns settings
```

### Issue: High memory usage

**Symptoms:** Nodes using > 80% of memory limit

**Solutions:**
```bash
# 1. Check for memory leaks
docker stats --no-stream

# 2. Increase memory limit
# Edit docker-compose.yml
mem_limit: 4g

# 3. Reduce load
# Decrease totalWorkers in load test
```

### Issue: Dashboard shows zeros

**Symptoms:** Per-node metrics show 0 MB/0%

**Solutions:**
```bash
# 1. Restart metrics server
pkill -f metrics-server.py
python3 test/docker/metrics-server.py \
  /tmp/loadtest.log \
  /tmp/monitoring-results \
  8888 &

# 2. Check monitoring is running
ps aux | grep monitor.py

# 3. Verify CSV files have data
tail /tmp/monitoring-results/per-node-resources.csv
```

### Issue: Test crashes

**Symptoms:** Load test exits with panic or error

**Solutions:**
```bash
# 1. Check logs for panic
tail -50 /tmp/loadtest.log

# 2. Verify node connectivity
for port in {26660..26671}; do
  curl -s http://localhost:$port/v3 > /dev/null && echo "Port $port: OK"
done

# 3. Reduce load and retry
# Decrease totalWorkers to 36
```

---

## Performance Tuning

### To Maximize TPS

1. **Increase workers:** 48 → 60 → 72 (linear scaling)
2. **HTTP/2:** Enable in load generator (ForceAttemptHTTP2: true)
3. **Connection pooling:** Increase MaxIdleConnsPerHost
4. **Batch submissions:** Send multiple txns per request (major improvement)

### To Reduce Resource Usage

1. **Optimize batch size:** Smaller batches = less memory
2. **Reduce vote limits:** Lower VotesPerHeaderMultiplier
3. **Increase major block interval:** Less frequent anchoring

### To Improve Stability

1. **Gradual ramp-up:** Start at low TPS, increase slowly
2. **Health checks:** Monitor node status before load
3. **Backpressure:** Add rate limiting in load generator

---

## Test Scenarios

### Scenario 1: Steady-State Load Test

**Goal:** Validate sustained throughput

```bash
# Configure
totalWorkers = 48
targetTPS = 10000
duration = 1 hour

# Run
./loadtest  # Let run for full duration
```

### Scenario 2: Ramp-Up Test

**Goal:** Find maximum TPS

```bash
# Start low
totalWorkers = 24
# Observe TPS

# Gradually increase
totalWorkers = 36  # Rebuild
totalWorkers = 48  # Rebuild
totalWorkers = 60  # Rebuild

# Stop when TPS plateaus
```

### Scenario 3: Stress Test

**Goal:** Find failure point

```bash
# Aggressive configuration
totalWorkers = 96
targetTPS = 20000

# Run until failures occur
# Analyze failure modes
```

### Scenario 4: Endurance Test

**Goal:** Validate stability over time

```bash
# Moderate load
totalWorkers = 48
targetTPS = 10000
duration = 24 hours

# Monitor for:
# - Memory leaks
# - Database growth
# - Performance degradation
```

---

## Expected Results

### At 10K TPS (48 workers):
- **Actual TPS:** 10,500±500
- **CPU usage:** 12-14 cores total
- **Memory usage:** 3-4 GB total
- **Success rate:** > 99.99%
- **Database:** ~1.3 MB per node per hour

### At 12K TPS (60 workers):
- **Actual TPS:** 12,000±700
- **CPU usage:** 15-17 cores total
- **Memory usage:** 4-5 GB total
- **Success rate:** > 99.99%
- **Database:** ~1.6 MB per node per hour

### Bottlenecks by TPS:
- **< 10K:** Transaction generation (HTTP latency)
- **10-15K:** Network I/O (connection limits)
- **15-20K:** CPU (validator processing)
- **20K+:** Consensus protocol overhead

---

## Files and Locations

### Source Code
- `cmd/accumulated` - Validator binary
- `test/docker/parallel-loadtest.go` - Load generator template
- `test/docker/monitor.py` - Monitoring script
- `test/docker/metrics-server.py` - Dashboard backend
- `test/docker/dashboard.html` - Dashboard frontend

### Configuration
- `test/docker/docker-compose.yml` - Container orchestration
- `test/docker/docker-network.yml` - Network configuration

### Output
- `/tmp/loadtest-workspace/loadtest.log` - Load test output
- `/tmp/monitoring-results/*.csv` - Monitoring data
- `/tmp/monitoring-results/summary-report.txt` - Summary

### Documentation
- `test/docker/HOW-TO-RUN-LOAD-TEST.md` - This file
- `test/docker/optimization-reports/FINAL-12K-TEST-RESULTS.md` - Results
- `test/docker/README-DAGBFT.md` - Network setup guide

---

## Support

**Issues:** https://github.com/anthropics/accumulate/issues
**Documentation:** test/docker/optimization-reports/

---

**Last updated:** March 24, 2026
**Tested configuration:** 12 validators, 48 workers, 10.5K TPS sustained
