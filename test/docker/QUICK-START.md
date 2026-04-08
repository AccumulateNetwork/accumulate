# 10K TPS Test Network - Quick Start

Get the full test network with dashboard running in 5 minutes.

## Prerequisites

- Go 1.21+
- Docker + Docker Compose
- Python 3.8+
- ~24 CPU cores (8 minimum)
- 32 GB RAM (16 GB minimum)
- 50 GB disk space

## One-Command Start

```bash
cd test/docker
./run-full-test.sh
```

This starts:
1. ✅ Docker network (1 bootstrap + 12 validators)
2. ✅ Real-time dashboard (http://localhost:8888)
3. ✅ Live load test (9000+ TPS)
4. ✅ Metrics collection

**Access dashboard:** http://localhost:8888/

## Manual Setup (5 steps)

### 1. Build the binary
```bash
cd /path/to/accumulate
go build ./cmd/accumulated
```

### 2. Start the network
```bash
cd test/docker
docker compose build
docker compose up -d
sleep 30  # Wait for initialization
docker compose ps
```

### 3. Start monitoring
```bash
mkdir -p /tmp/monitoring-results
python3 monitor.py /tmp/monitoring-results 3600 10 > /tmp/monitor.log 2>&1 &
```

### 4. Start dashboard
```bash
python3 metrics-server.py /tmp/loadtest.log /tmp/monitoring-results 8888 &
```

### 5. Run load test
```bash
go run parallel-loadtest.go -start-tps 10000 -end-tps 10000 -duration 300s 2>&1 | tee /tmp/loadtest.log
```

**Then open dashboard:** http://localhost:8888/

## What You'll See

- **Dashboard**: Real-time graphs of TPS, CPU, memory per validator
- **Console**: Progress updates every 10 seconds
- **Metrics API**: http://localhost:8888/metrics (JSON)

Expected results:
- **TPS**: 9000-10500 sustained
- **Success rate**: 99.99%+
- **Failure rate**: 0%
- **Per-node CPU**: 1-3 cores
- **Per-node Memory**: 200-300 MB

## Stop Everything

```bash
# Stop load test
killall go

# Stop dashboard
pkill -f metrics-server.py

# Stop monitoring
pkill -f monitor.py

# Stop network
docker compose down -v
```

## Troubleshooting

**Dashboard shows no data?**
- Make sure load test is running: `ps aux | grep parallel-loadtest`
- Check logs: `tail /tmp/metrics-server.log`

**Low TPS (< 1000)?**
- Verify all validators are healthy: `docker compose ps`
- Increase workers: Edit `parallel-loadtest.go`, change `workersPerNode`

**Network won't start?**
- Check Docker: `docker ps -a`
- Clean and restart: `docker compose down -v && docker compose up -d`
- Check logs: `docker compose logs -f bootstrap`

## Configuration

### Worker Count (TPS target)
Edit `parallel-loadtest.go`:
```go
const (
    workersPerNode = 4   // Adjust this
    totalNodes     = 12
)
```
- `1 worker/node` → ~1000 TPS
- `4 workers/node` → ~9000 TPS (default)
- `6 workers/node` → ~15000 TPS

### Test Duration
```bash
go run parallel-loadtest.go -duration 300s  # 5 minutes (default)
go run parallel-loadtest.go -duration 0     # Run until Ctrl+C
```

### Target TPS
```bash
go run parallel-loadtest.go -start-tps 5000 -end-tps 10000  # Ramp up
```

## Files

| File | Purpose |
|------|---------|
| `docker-compose.yml` | Container orchestration |
| `docker-network.yml` | Network genesis config |
| `parallel-loadtest.go` | Load generator |
| `dashboard.html` | Web dashboard UI |
| `metrics-server.py` | API backend |
| `monitor.py` | Metrics collector |
| `run-full-test.sh` | One-command setup |
| `QUICK-START.md` | This file |
| `README-DAGBFT.md` | Network architecture |

## Performance Tips

1. **Maximize TPS**: Increase `workersPerNode` to 6+
2. **Reduce CPU**: Use fewer workers or increase major block interval
3. **Monitor in real-time**: Watch dashboard while load test runs

## Next Steps

- Check `README-DAGBFT.md` for architecture details
- See `HOW-TO-RUN-LOAD-TEST.md` for advanced configuration
- Review `optimization-reports/` for performance analysis

---

**Issue**: #3892
**Branch**: `issue-3892-10k-tps-infrastructure`
**Status**: Production-ready
