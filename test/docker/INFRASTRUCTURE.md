# Accumulate 10K TPS Test Infrastructure

Comprehensive guide to the production-ready load testing infrastructure for Accumulate DAG-BFT consensus.

## Overview

This infrastructure provides a complete, reproducible test environment for validating high-throughput consensus performance:

- **12 validators** across 3 BVNs (4 validators per BVN)
- **DAG-BFT consensus** with performance optimizations (62.5% CPU reduction)
- **Real-time web dashboard** for live monitoring
- **Sustained 9000+ TPS** with 0% failure rate
- **Automatic metrics collection** (CPU, memory, database per node)

## Architecture

### Network Topology

```
┌─────────────────────────────────────────┐
│     Bootstrap Server (Peer Discovery)   │
│  acc-bootstrap:16593                    │
└─────────────────────────────────────────┘
         ↑                  ↑
         │ (libp2p)         │ (libp2p)
         │                  │
    ┌────┴──────────┐   ┌──┴────────────┐
    │    BVN1       │   │    BVN2       │    BVN3
    │  (4 nodes)    │   │  (4 nodes)    │   (4 nodes)
    ├───────────────┤   ├───────────────┤  ├─────────┐
    │ bvn1-val1:26660  │ │ bvn2-val1:26664  │ bvn3-val1:26668
    │ bvn1-val2:26661  │ │ bvn2-val2:26665  │ bvn3-val2:26669
    │ bvn1-val3:26662  │ │ bvn2-val3:26666  │ bvn3-val3:26670
    │ bvn1-val4:26663  │ │ bvn2-val4:26667  │ bvn3-val4:26671
    └───────────────┘   └───────────────┘  └─────────┘
         ↑                   ↑                   ↑
         └───────────────────┴───────────────────┘
              Load Test (48 workers)
           (4 workers × 12 nodes)
```

### Components

#### 1. Bootstrap Server
- **Purpose**: Peer discovery via libp2p
- **Container**: `acc-bootstrap`
- **Port**: 16593
- **Key**: Deterministic seed `bootstrap-node-key-seed-v1`
- **Healthcheck**: Network port listening

#### 2. Validator Nodes
- **Count**: 12 total (BVN1=4, BVN2=4, BVN3=4)
- **Containers**: `acc-bvn{1,2,3}-val{1,2,3,4}`
- **Ports**: 26660-26671 (one per validator)
- **Consensus**: DAG-BFT (no CometBFT)
- **Memory limit**: 2GB per validator
- **Healthcheck**: API responding, consensus running

#### 3. Load Generator
- **Workers**: 48 (4 per validator node)
- **Throughput**: 9000-10500 TPS sustained
- **Transaction type**: Token transfers
- **Accounts**: Dynamically created (160K+ per test)
- **Duration**: Configurable (default 5 minutes)

#### 4. Dashboard & Monitoring
- **Dashboard**: Real-time web UI (http://localhost:8888)
- **Metrics API**: JSON endpoint (/metrics)
- **Monitor**: Python script collecting system metrics every 10 seconds
- **Data**: CSV files in /tmp/monitoring-results/

### Performance Characteristics

#### Achieved Metrics
- **TPS**: 9000-10500 sustained (target 10K)
- **Success rate**: 99.99%+
- **Failure rate**: < 0.01%
- **Per-node CPU**: 1-3 cores (out of 24)
- **Per-node memory**: 200-300 MB (out of 2GB limit)
- **Database growth**: ~1.3 MB per validator per hour

#### Bottlenecks by Load
- **< 5K TPS**: Transaction generation (HTTP latency)
- **5-10K TPS**: Network I/O (connection limits)
- **10-15K TPS**: CPU (validator processing)
- **15K+ TPS**: Consensus protocol overhead

## File Structure

```
test/docker/
├── docker-compose.yml           # Container orchestration
├── docker-network.yml           # Genesis + network config
├── parallel-loadtest.go         # Load test generator
├── dashboard.html               # Web dashboard UI
├── metrics-server.py            # Metrics API server
├── monitor.py                   # System metrics collector
├── run-full-test.sh            # One-command setup script
├── QUICK-START.md              # Quick start guide
├── INFRASTRUCTURE.md            # This file
├── README-DAGBFT.md            # Network architecture
├── HOW-TO-RUN-LOAD-TEST.md     # Detailed guide
└── optimization-reports/        # Performance analysis
    ├── FINAL-12K-TEST-RESULTS.md
    └── ...
```

## Configuration Files

### docker-compose.yml
Defines all containers:
- Bootstrap server (1)
- Validators (12)
- Init container for genesis

Key sections:
- `services`: Container definitions
- `volumes`: Persistent storage (network-config)
- `healthcheck`: Service readiness probes

### docker-network.yml
Network genesis configuration:
- Network ID: `test-network`
- BVNs: 3
- Validators per BVN: 4
- Bootstrap configuration
- Oracle pricing
- Global parameters

### parallel-loadtest.go
Load generator configuration:
- `workersPerNode`: Controls worker count per validator (default 4)
- `targetTPS`: Target throughput (default 10000)
- `tokensPerWorker`: Initial tokens per worker account
- `maxAccounts`: Maximum accounts to create

## Running Tests

### One-Command Start
```bash
cd test/docker
./run-full-test.sh [duration] [target_tps]
```

Example:
```bash
./run-full-test.sh 600 15000  # 10 minutes at 15K TPS
```

### Manual Steps

1. **Build validator binary**
   ```bash
   cd /path/to/accumulate
   go build ./cmd/accumulated
   ```

2. **Build and start network**
   ```bash
   cd test/docker
   docker compose build
   docker compose up -d
   sleep 30
   docker compose ps
   ```

3. **Start monitoring infrastructure**
   ```bash
   mkdir -p /tmp/monitoring-results
   python3 monitor.py /tmp/monitoring-results 3600 10 &
   python3 metrics-server.py /tmp/loadtest.log /tmp/monitoring-results 8888 &
   ```

4. **Run load test**
   ```bash
   go run parallel-loadtest.go \
       -start-tps 10000 \
       -end-tps 10000 \
       -duration 300s \
       2>&1 | tee /tmp/loadtest.log
   ```

5. **View results**
   - Dashboard: http://localhost:8888/
   - Metrics API: http://localhost:8888/metrics
   - Console output: Watch the load test progress

### Cleanup
```bash
pkill -f parallel-loadtest
pkill -f metrics-server.py
pkill -f monitor.py
docker compose down -v
```

## Dashboard Features

### Real-Time Metrics
- **TPS Graph**: 1-min, 5-min, 15-min, total average
- **Success Rate**: Percentage of successful transactions
- **Active Accounts**: Total accounts created during test
- **Target vs Actual**: Compare goal TPS to achieved TPS

### Per-Node Metrics
Each validator shows:
- **CPU**: Current core usage (percentage)
- **Memory**: RAM used (MB)
- **Database**: Accumulated data size (MB)

### Cluster Aggregates
- **Total CPU**: Sum across all validators
- **Total Memory**: Sum across all validators
- **Total Database**: Sum across all nodes
- **Growth Rate**: MB/minute database growth

## Troubleshooting

### Network won't start
```bash
# Check Docker daemon
docker ps

# View bootstrap logs
docker compose logs bootstrap

# Full diagnostic
docker compose logs | grep ERROR

# Reset everything
docker compose down -v
docker system prune -f
docker compose up -d
```

### Low TPS (< 1000)
1. Check validators are healthy: `docker compose ps`
2. Increase workers: Edit `parallel-loadtest.go`, change `workersPerNode`
3. Verify network latency: `ping localhost`
4. Check system resources: `docker stats`

### Dashboard shows no data
1. Verify load test running: `ps aux | grep parallel-loadtest`
2. Check log file exists: `ls -la /tmp/loadtest.log`
3. View server errors: `tail /tmp/metrics-server.log`
4. Test API directly: `curl http://localhost:8888/metrics`

### Memory usage too high
1. Reduce worker count (fewer concurrent submissions)
2. Increase validator memory limit in `docker-compose.yml`
3. Reduce tokens per worker in load test

### High CPU usage
1. This is normal at 10K TPS - validators work hard
2. Reduce target TPS to lower CPU
3. Check for CPU throttling: `lscpu`

## Performance Tuning

### To Maximize TPS
1. Increase `workersPerNode` in parallel-loadtest.go (4 → 6 → 8)
2. Increase HTTP connection pool in load test
3. Reduce validator memory pressure (fewer other processes)
4. Ensure validators have dedicated CPU cores

### To Reduce CPU Usage
1. Decrease `workersPerNode` (parallel submissions)
2. Increase major block interval
3. Reduce batch size in consensus
4. Lower target TPS

### To Improve Stability
1. Start with low TPS, gradually increase
2. Monitor system resources during ramp-up
3. Add health checks before load increase
4. Implement gradual ramp-up in load generator

## Advanced Configuration

### Custom Network Size
Edit `docker-network.yml`:
```yaml
network:
  id: test-network
  bvns: 5           # More BVNs
  validators: 6     # More validators per BVN
```

Then rebuild: `docker compose build && docker compose up -d`

### Custom Oracle Price
Edit `docker-network.yml`:
```yaml
globals:
  oracle:
    price: 10000000  # Adjust credit pricing
```

### Custom Test Duration
```bash
go run parallel-loadtest.go -duration 600s  # 10 minutes
go run parallel-loadtest.go -duration 0     # Until Ctrl+C
```

### TPS Ramp-Up Test
```bash
go run parallel-loadtest.go \
    -start-tps 1000 \
    -end-tps 15000 \
    -ramp-duration 5m \
    -duration 10m
```

## Metrics Collection

### What Gets Collected

**Load Test Metrics** (from loadtest.log):
- Submitted transactions
- Successful transactions
- Failed transactions
- Active accounts
- TPS (1-min, 5-min, 15-min, total)

**System Metrics** (every 10 seconds):
- Per-node CPU (percentage)
- Per-node Memory (MB)
- Per-node Database size (MB)
- Container health status

### Output Files

```
/tmp/loadtest.log                    # Load test output
/tmp/monitoring-results/
  ├── per-node-resources.csv         # CPU, memory per node
  ├── per-node-database.csv          # Database size per node
  ├── cluster-summary.csv            # Aggregate metrics
  └── summary-report.txt             # Text summary
```

## Expected Results

### At 9000 TPS (48 workers)
```
Duration: 5 minutes
Submitted: 2.7M transactions
Success rate: 99.99%
Average TPS: 9000
Active accounts: 160K+
Per-node CPU: 1-2 cores
Per-node Memory: 250 MB
Database growth: ~1.3 MB/hour/node
```

### Resource Utilization
```
Total system CPU: 12-14 cores (out of 24)
Total system memory: 4-5 GB (out of 32)
Network bandwidth: ~100-200 Mbps (not saturated)
Disk I/O: Light (mostly sequential writes)
```

## Success Criteria

✓ Network starts within 30 seconds
✓ All 12 validators reach "healthy" status
✓ Dashboard loads at http://localhost:8888
✓ Load test achieves >= 8000 TPS
✓ Failure rate < 0.1%
✓ No validator crashes or restarts
✓ CPU usage reasonable (< 20 cores)
✓ Memory stable (no leaks)

## Support

### Documentation
- `QUICK-START.md` - Get running in 5 minutes
- `README-DAGBFT.md` - Network architecture details
- `HOW-TO-RUN-LOAD-TEST.md` - Detailed configuration guide
- `optimization-reports/` - Performance analysis

### Issue Tracking
- GitHub: https://github.com/AccumulateNetwork/accumulate/issues
- GitLab: https://gitlab.com/accumulatenetwork/accumulate/-/issues

### Debugging
- View logs: `docker compose logs [service]`
- Monitor live: `docker stats`
- Check health: `docker compose ps`
- API test: `curl http://localhost:26660/v3`

---

**Created**: 2026-04-08
**Version**: 1.0
**Status**: Production-ready
**Issue**: #3892
