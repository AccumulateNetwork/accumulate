# Gap Recovery Testing for CrossChain Conductor

## Overview

This testing suite allows you to validate the CrossChain Conductor's gap recovery mechanism by selectively stalling BVNs and monitoring how the system detects and recovers from message gaps.

## Quick Start

```bash
# Start the gap recovery test with web dashboard
./devnet_load_test_enhanced.sh gap

# Or run directly
./gap_test.sh
```

Then open http://localhost:8081 in your browser.

## Features

### 1. BVN Control Panel
- **Selective Stalling**: Stall individual BVNs for 10s or 30s
- **Manual Control**: Instantly unstall any BVN
- **Visual Status**: Real-time status indicators (green=active, red=stalled)

### 2. Height Monitoring
- **BVN Pair Tracking**: Monitor source/destination heights for every BVN pair
- **Gap Detection**: Visual alerts when gaps are detected (red border)
- **Recovery Tracking**: Green highlighting when gaps are recovered

### 3. Performance Metrics
- **Active Gaps**: Current number of detected gaps
- **Recovered Gaps**: Total gaps successfully recovered
- **Average Recovery Time**: Mean time to recover from gaps
- **TPS Monitoring**: Real-time transaction throughput

### 4. Visualization
- **TPS History Chart**: 60-second rolling window of throughput
- **Recovery Time Chart**: Bar chart of last 10 gap recovery times
- **Event Log**: Real-time feed of gap detection and recovery events

## Test Scenarios

### Scenario 1: Basic Gap Creation
1. Click "Stall 10s" on BVN1
2. Watch gaps form in BVN0→BVN1 and BVN2→BVN1 pairs
3. Observe automatic recovery after 10 seconds
4. Check recovery time in metrics

### Scenario 2: Cascading Failures
1. Stall BVN0 for 30s
2. After 10s, stall BVN1 for 20s
3. Monitor cascading gap formation
4. Verify all gaps recover correctly

### Scenario 3: Stress Test
1. Rapidly stall and unstall different BVNs
2. Create multiple simultaneous gaps
3. Verify system handles concurrent recoveries
4. Check that no messages are lost

### Scenario 4: Partition Isolation
1. Stall BVN0 and BVN2 simultaneously
2. Verify BVN1 continues processing
3. Monitor isolated recovery of each partition

## Architecture

### Components

1. **Gap Test Monitor** (`gap_test_monitor.go`)
   - Web server on port 8081
   - BVN height monitoring
   - Gap detection logic
   - Stall control system

2. **Web Dashboard**
   - Real-time metrics updates (1s interval)
   - Interactive BVN controls
   - Chart.js visualizations
   - Event logging

3. **Test Traffic Generator**
   - Continuous crosschain transactions
   - Distributed across BVNs
   - Automatic load adjustment

### Gap Detection

Gaps are detected when:
- Source height > Destination height + threshold (default: 10 blocks)
- Messages fail to arrive at destination
- Sequence numbers are missing

### Recovery Mechanism

The system recovers by:
1. Detecting the gap through sequence tracking
2. Sending a gap request to the source partition
3. Source resets its sequence pointer to the gap start
4. Next transmission includes all messages from gap start
5. No retries needed - automatic inclusion in next collection

## Configuration

### Environment Variables

```bash
# BVN URLs (defaults)
BVN0_URL="http://localhost:27010"
BVN1_URL="http://localhost:27011"
BVN2_URL="http://localhost:27012"

# Web dashboard port
WEB_PORT=8081

# Gap detection threshold (blocks)
GAP_THRESHOLD=10
```

### Modifying Test Parameters

Edit `gap_test_monitor.go`:

```go
var (
    // Number of test accounts
    numWorkers = 10
    
    // Transaction generation rate
    txInterval = 100 * time.Millisecond
    
    // Gap detection threshold
    gapThreshold = uint64(10)
)
```

## Monitoring

### Key Metrics to Watch

1. **Gap Formation Rate**: How quickly gaps are detected
2. **Recovery Time**: Time from gap detection to recovery
3. **Message Delivery Rate**: Percentage of messages delivered
4. **TPS During Recovery**: Throughput during gap recovery

### Expected Behavior

- **Normal Operation**: No gaps, steady TPS
- **During Stall**: Gap formation, TPS may drop
- **Recovery Phase**: Burst of messages, TPS spike
- **Post-Recovery**: Return to normal, all messages delivered

## Troubleshooting

### Dashboard Not Loading
```bash
# Check if monitor is running
ps aux | grep gap_test_monitor

# Check logs
tail -f gap_monitor.log
```

### BVNs Not Responding
```bash
# Verify devnet is running
curl http://localhost:27004/status

# Restart devnet if needed
./devnet_manager.sh
```

### Gaps Not Recovering
- Check if source BVN is still stalled
- Verify network connectivity between BVNs
- Look for errors in gap_monitor.log

## Integration with CI/CD

```yaml
# Example GitHub Actions workflow
- name: Start DevNet
  run: ./devnet_manager.sh start
  
- name: Run Gap Recovery Test
  run: |
    ./gap_test.sh &
    sleep 30
    # Automated stall test
    curl -X POST http://localhost:8081/api/stall \
      -d '{"bvn":"BVN1","duration":10000000000}'
    sleep 15
    # Check recovery
    curl http://localhost:8081/api/metrics | \
      jq '.CrossChain.GapsRecovered'
```

## Advanced Usage

### Custom Gap Scenarios

Create custom test scenarios by calling the API:

```javascript
// Stall BVN for specific duration (nanoseconds)
fetch('/api/stall', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({
        bvn: 'BVN1',
        duration: 30000000000  // 30 seconds
    })
});

// Get current metrics
fetch('/api/metrics')
    .then(r => r.json())
    .then(metrics => console.log(metrics));

// Get BVN pair status
fetch('/api/bvn-pairs')
    .then(r => r.json())
    .then(pairs => console.log(pairs));
```

### Automated Testing

```bash
#!/bin/bash
# Automated gap recovery test

# Start monitor
./gap_test.sh &
MONITOR_PID=$!

# Wait for startup
sleep 5

# Test sequence
for i in {0..2}; do
    # Stall BVN
    curl -X POST http://localhost:8081/api/stall \
        -d "{\"bvn\":\"BVN$i\",\"duration\":10000000000}"
    
    # Wait for recovery
    sleep 15
    
    # Check metrics
    RECOVERED=$(curl -s http://localhost:8081/api/metrics | \
        jq '.CrossChain.GapsRecovered')
    echo "Gaps recovered: $RECOVERED"
done

# Cleanup
kill $MONITOR_PID
```

## Results Interpretation

### Success Criteria
- All gaps detected within 2 seconds
- All gaps recovered within 30 seconds
- No message loss during recovery
- TPS returns to baseline after recovery

### Performance Benchmarks
- Gap detection: < 2 seconds
- Recovery time: < 20 seconds average
- Message delivery: 100% eventual delivery
- TPS impact: < 50% reduction during recovery

## Contributing

To add new test scenarios:

1. Edit `gap_test_monitor.go` to add new stall patterns
2. Update the web dashboard for new visualizations
3. Add automated test cases in `gap_test.sh`
4. Document expected behavior in this README

## Related Documentation

- [Gap Recovery Design](../../internal/core/execute/v2/crosschain/GAP_RECOVERY_DESIGN.md)
- [CrossChain Conductor Review](../../internal/core/execute/v2/crosschain/CCC_REVIEW.md)
- [DevNet Load Test](./DEVNET_LOAD_TEST.md)