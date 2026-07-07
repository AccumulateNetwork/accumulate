# Stability Test Plan - DAG-BFT Network

## Network Configuration

**Topology**: 3 BVNs (Block Validator Networks)  
**Total Nodes**: 12 validators (4 per BVN)

The network runs in **docker** (`test/docker/docker-compose.yml`: bootstrap +
init + 12 validators). `test/run-dagbft-network.sh up` builds, starts, and
waits for block production; node APIs are on host ports 26660-26671. The
loopback addresses below describe the container-internal layout.

### BVN1 - 4 Validator Nodes
- Node 1: 127.0.1.1:26656
- Node 2: 127.0.1.2:26656
- Node 3: 127.0.1.3:26656
- Node 4: 127.0.1.4:26656

### BVN2 - 4 Validator Nodes
- Node 1: 127.0.2.1:26656
- Node 2: 127.0.2.2:26656
- Node 3: 127.0.2.3:26656
- Node 4: 127.0.2.4:26656

### BVN3 - 4 Validator Nodes
- Node 1: 127.0.3.1:26656
- Node 2: 127.0.3.2:26656
- Node 3: 127.0.3.3:26656
- Node 4: 127.0.3.4:26656

## Test Phases

### Phase 1: Short Duration Tests (30 minutes each)

#### Test 1.1: Low Load (10 TPS)
```bash
# Start the network
cd test
./run-dagbft-network.sh

# In another terminal, run load test
cd test/cmd/load
go run . -s http://127.0.0.1:26660/v2 -t 10 -d 1800 -dashboard

# Monitor for:
- Block production consistency
- Transaction success rate (>99%)
- Memory stability (no leaks)
- CPU usage (<50% average)
- Goroutine count stability
```

**Success Criteria**:
- ✅ Network runs for 30 minutes without crashes
- ✅ All 12 nodes stay synchronized
- ✅ Transaction success rate > 99%
- ✅ No memory leaks (stable RSS)
- ✅ Block production consistent across BVNs

#### Test 1.2: Medium Load (100 TPS)
```bash
# Same setup, higher TPS
go run . -s http://127.0.0.1:26660/v2 -t 100 -d 1800 -dashboard
```

**Success Criteria**:
- ✅ Network handles 100 TPS sustained load
- ✅ Transaction success rate > 95%
- ✅ Latency < 2 seconds (p95)
- ✅ No node failures
- ✅ Memory usage stable

### Phase 2: Extended Soak Test (24 hours)

#### Test 2.1: Sustained Load (50 TPS)
```bash
# 24-hour test
go run . -s http://127.0.0.1:26660/v2 -t 50 -d 86400 -dashboard
```

**Success Criteria**:
- ✅ 24-hour uptime
- ✅ Total transactions > 4.3 million
- ✅ Transaction success rate > 98%
- ✅ No crashes or restarts
- ✅ Memory stable (no growth)
- ✅ CPU usage stable
- ✅ Block production consistent

**Monitoring During Test**:
```bash
# Monitor resources
watch -n 10 'ps aux | grep accumulated-dagbft | awk "{print \$2, \$3, \$4, \$6}"'

# Monitor logs
tail -f /tmp/dagbft-*.log | grep -E "ERROR|FATAL|panic"

# Check block heights
curl http://127.0.0.1:26660/v2/metrics | grep block_height
```

### Phase 3: Stress Tests

#### Test 3.1: Peak Load (200 TPS)
```bash
# 30-minute peak load test
go run . -s http://127.0.0.1:26660/v2 -t 200 -d 1800 -dashboard
```

**Success Criteria**:
- ✅ Network handles peak load
- ✅ Transaction success rate > 90%
- ✅ Graceful degradation if overloaded
- ✅ Recovery after load reduction

#### Test 3.2: Node Failure Recovery
```bash
# Kill a node during operation
kill $(ps aux | grep "accumulated-dagbft.*127.0.1.1" | awk '{print $2}')

# Wait 5 minutes, then restart
./start-node.sh BVN1 1

# Verify:
# - Network continues operating (11 nodes)
# - Node rejoins and syncs
# - No data loss
```

#### Test 3.3: Network Partition Simulation
```bash
# Block traffic between BVN1 and BVN2
sudo iptables -A INPUT -s 127.0.2.0/24 -j DROP
sudo iptables -A OUTPUT -d 127.0.2.0/24 -j DROP

# Wait 5 minutes, then restore
sudo iptables -D INPUT -s 127.0.2.0/24 -j DROP
sudo iptables -D OUTPUT -d 127.0.2.0/24 -j DROP

# Verify:
# - Consensus continues in unpartitioned sections
# - Network recovers after partition heals
# - State reconciliation works
```

## Metrics to Collect

### Performance Metrics
- Transactions per second (actual vs target)
- Transaction success rate
- Latency (p50, p95, p99)
- Block production rate
- Block size

### Resource Metrics
- CPU usage per node
- Memory usage (RSS, virtual)
- Goroutine count
- File descriptors
- Network bandwidth

### Consensus Metrics
- Certificate production rate
- DAG depth
- Commit latency
- Fork rate (should be 0)

### Health Metrics
- Node uptime
- Sync status
- Error rates
- Panic count (should be 0)

## Test Tools

### Load Generator
```bash
cd test/cmd/load
go run . --help

# Options:
# -s: Server URL
# -t: Target TPS
# -d: Duration (seconds)
# -r: Transactions per routine
# -dashboard: Enable real-time dashboard
```

### Performance Monitor
```bash
cd tools/cmd/debug
go run . perfmon --duration 60s --interval 1s --output report.json
```

### Network Monitor
```bash
cd test/cmd/monitor
go run . --config ../dagbft-network.yml --interval 10s
```

## Expected Results

### Minimum Acceptable Performance
- **Throughput**: 50 TPS sustained
- **Latency**: < 2s (p95)
- **Success Rate**: > 98%
- **Uptime**: 99.9% (< 1 minute downtime per day)
- **Sync Time**: < 5 minutes after disconnect

### Target Performance
- **Throughput**: 100 TPS sustained, 200 TPS peak
- **Latency**: < 1s (p95)
- **Success Rate**: > 99.5%
- **Uptime**: 99.99%
- **Sync Time**: < 2 minutes after disconnect

## Failure Criteria (Stop and Debug)

- ❌ Any node crashes/panics
- ❌ Transaction success rate drops below 95%
- ❌ Memory leak detected (>10% growth per hour)
- ❌ Network partition doesn't recover within 10 minutes
- ❌ Block production stops for > 1 minute
- ❌ State divergence between nodes

## After Test Completion

1. **Collect Logs**:
   ```bash
   tar -czf stability-test-logs.tar.gz /tmp/dagbft-*.log
   ```

2. **Generate Reports**:
   ```bash
   cd tools/cmd/debug
   go run . analyze-logs --input /tmp/dagbft-*.log --output report.html
   ```

3. **Performance Analysis**:
   ```bash
   go run . perfmon analyze --baseline baseline.json --current test.json
   ```

4. **Document Results**:
   - Transaction throughput achieved
   - Resource usage patterns
   - Any errors or warnings
   - Recommendations for optimization

## Configuration File

Network topology defined in: `test/dagbft-network.yml`

Total configuration:
- 3 BVNs
- 4 validators per BVN
- 12 total validator nodes
- Local loopback networking
- Base port: 26656 per node

## Next Steps After Successful Testing

1. ✅ Phase 1 complete → Proceed to Phase 2
2. ✅ Phase 2 complete → Proceed to Phase 3
3. ✅ Phase 3 complete → Production readiness review
4. ✅ All phases complete → Deploy to staging environment
5. 🚀 Staging validated → Production launch

---

**Configuration Updated**: 2026-03-23  
**Test Plan Version**: 1.0  
**Status**: Ready for execution
