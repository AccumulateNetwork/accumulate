# How to Run Visual Load Testing with JSON Logging

## Quick Start

### 1. Run Visual Partition Monitor (Interactive)

```bash
# Run the visual partition monitor with live updates
go run visual_partition_monitor.go

# To capture output for analysis:
go run visual_partition_monitor.go 2>&1 | tee partition_monitor.log
```

**Interactive Controls:**
- Press `1-4`: Toggle partition health (1=BVN0, 2=BVN1, 3=BVN2, 4=Directory)
- Press `c`: Cause cascading failure
- Press `r`: Recover all partitions
- Press `q`: Quit

### 2. Run Visual Lag Demo

```bash
# Run the lag demonstration
go run visual_lag_demo.go

# With logging:
go run visual_lag_demo.go 2>&1 | tee lag_demo.log
```

### 3. Run Full Test Suite

```bash
# Run all tests with logging
./run_full_test_suite.sh
```

## Capturing JSON Metrics

Since the existing visual monitors output to the terminal, here's how to capture metrics:

### Method 1: Parse Visual Output to JSON

```bash
# Run monitor and convert output to JSON
go run visual_partition_monitor.go 2>&1 | \
  awk '/Global Sequence:/ {seq=$NF} 
       /BVN[0-9]/ {
         partition=$1; 
         status=$2; 
         sent=$3; 
         acked=$4; 
         lag=$5;
         printf "{\"partition\":\"%s\",\"status\":\"%s\",\"sent\":%s,\"acked\":%s,\"lag\":%s,\"seq\":%s}\n", 
                partition, status, sent, acked, lag, seq
       }' > metrics.json
```

### Method 2: Run Tests and Extract Metrics

```bash
# Performance test
go run test_collection_proof_performance.go | \
  grep -E "Speedup|Savings|efficiency" > performance_metrics.txt

# Integration test  
go run test_batch_proof_integration.go | \
  grep -E "proof_savings|batch_size|efficiency" > integration_metrics.txt
```

## Monitoring in Real-Time

### Terminal 1: Run Visual Monitor
```bash
go run visual_partition_monitor.go
```

### Terminal 2: Watch Logs
```bash
# If outputting to file
tail -f partition_monitor.log | grep -E "Lag|Catch-up|Success Rate"
```

### Terminal 3: Extract Metrics
```bash
# Parse logs for key metrics
tail -f partition_monitor.log | \
  sed -n 's/.*Success Rate: \([0-9.]*\)%.*/{"success_rate": \1}/p'
```

## Load Testing Scenarios

### 1. Normal Load Test
```bash
# Run with standard load
go run visual_partition_monitor.go
# Let it run for 30 seconds, all partitions healthy
```

### 2. Partition Failure Test
```bash
# Run monitor
go run visual_partition_monitor.go
# Press '1' to fail BVN0
# Watch lag accumulate
# Press '1' again to recover
# Watch catch-up rate
```

### 3. Cascading Failure Test
```bash
# Run monitor  
go run visual_partition_monitor.go
# Press 'c' for cascading failure
# Observe all partitions failing
# Press 'r' to recover all
# Watch synchronized recovery
```

## Analyzing Results

### Extract Key Metrics from Logs

```bash
# Get average lag
grep "Lag" partition_monitor.log | \
  awk '{sum+=$NF; count++} END {print "Avg Lag:", sum/count}'

# Get success rate over time
grep "Success Rate" partition_monitor.log | \
  awk -F': ' '{print $NF}' | \
  awk '{sum+=$1; count++} END {print "Avg Success Rate:", sum/count "%"}'

# Count collection proof usage
grep -c "Collection proof" partition_monitor.log
```

### Performance Comparison

```bash
# Run performance test and extract speedup
go run test_collection_proof_performance.go | \
  grep "Average Speedup" | \
  awk '{print $3}'
```

## Example Session

```bash
# Terminal 1 - Run visual monitor
$ go run visual_partition_monitor.go

# Terminal 2 - Watch metrics (in another terminal)
$ watch -n 1 'tail -20 partition_monitor.log | grep -E "Lag|Success"'

# Terminal 3 - Run performance test
$ go run test_collection_proof_performance.go

# Terminal 4 - Check collection proof efficiency
$ go run test_batch_proof_integration.go | grep efficiency
```

## For AI Analysis

To generate data I can analyze, run:

```bash
# Generate comprehensive metrics
(
  echo "=== Performance Test ==="
  go run test_collection_proof_performance.go
  echo ""
  echo "=== Integration Test ==="
  go run test_batch_proof_integration.go
  echo ""
  echo "=== Visual Monitor (30 second run) ==="
  timeout 30 go run visual_partition_monitor.go
) > full_test_results.log 2>&1

# Then share the contents of full_test_results.log
cat full_test_results.log
```

This will give me the complete output to analyze the collection proof optimization performance!