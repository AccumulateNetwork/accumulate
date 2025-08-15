# Instructions for Running Visual Load Tests

## For You to Run in Your Terminal

### Option 1: Visual Partition Monitor with Logging

**In Terminal 1 (Main Visual Interface):**
```bash
cd /home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/test/load
go run visual_partition_monitor.go
```

**In Terminal 2 (Capture logs for AI analysis):**
```bash
cd /home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/test/load
# This captures the output to a file I can read
script -c "go run visual_partition_monitor.go" monitor_session.log
```

**Interactive Controls While Running:**
- Press `1` - Toggle BVN0 health
- Press `2` - Toggle BVN1 health  
- Press `3` - Toggle BVN2 health
- Press `4` - Toggle Directory health
- Press `c` - Cause cascading failure
- Press `r` - Recover all partitions
- Press `q` - Quit

### Option 2: Visual Lag Demo

```bash
cd /home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/test/load
go run visual_lag_demo.go | tee lag_output.log
```

### Option 3: Run with JSON Output Capture

**Run this command to see visual output AND create JSON logs:**
```bash
cd /home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/test/load

# This runs the visual monitor and extracts metrics to JSON
go run visual_partition_monitor.go 2>&1 | tee >(
  grep -E "Sequence:|BVN|Directory|Success Rate|Lag" | \
  awk '
    /Sequence:/ {seq=$NF}
    /Success Rate:/ {rate=$NF}
    /BVN|Directory/ {
      printf "{\"time\":\"%s\",\"seq\":%s,\"partition\":\"%s\",\"sent\":%s,\"acked\":%s,\"lag\":%s}\n",
             strftime("%Y-%m-%d %H:%M:%S"), seq, $1, $3, $4, $5
    }
  ' > monitor_metrics.jsonl
)
```

### Option 4: Full Test Suite with Visual Output

```bash
cd /home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/test/load

# First run the performance tests
echo "=== PERFORMANCE TESTS ===" | tee test_output.log
go run test_collection_proof_performance.go | tee -a test_output.log

# Then run integration tests
echo "=== INTEGRATION TESTS ===" | tee -a test_output.log  
go run test_batch_proof_integration.go | tee -a test_output.log

# Finally run the visual monitor (you interact with this)
echo "=== VISUAL MONITOR ===" | tee -a test_output.log
go run visual_partition_monitor.go | tee -a test_output.log
```

## After You Run the Tests

**Share the log files with me by running:**
```bash
# Show me the last 100 lines of the output
tail -100 test_output.log

# Or show specific metrics
grep -E "Speedup|efficiency|Success Rate|Lag" test_output.log

# If you captured JSON metrics
cat monitor_metrics.jsonl | tail -20
```

## Test Scenarios to Try

### Scenario 1: Normal Operation
1. Run `go run visual_partition_monitor.go`
2. Let it run for 30 seconds without pressing anything
3. Press `q` to quit
4. Share the output with me

### Scenario 2: Partition Failure and Recovery
1. Run `go run visual_partition_monitor.go`
2. After 10 seconds, press `1` (fail BVN0)
3. Watch lag accumulate for 20 seconds
4. Press `1` again (recover BVN0)
5. Watch it catch up
6. Press `q` to quit
7. Share the output with me

### Scenario 3: Cascading Failure
1. Run `go run visual_partition_monitor.go`
2. After 10 seconds, press `c` (cascading failure)
3. Watch all partitions fail
4. After 15 seconds, press `r` (recover all)
5. Watch recovery
6. Press `q` to quit
7. Share the output with me

## To Share Results with Me

After running, you can share the results by:
```bash
# Option 1: Show the log file
cat test_output.log

# Option 2: Show just the summary
tail -50 test_output.log

# Option 3: Show metrics only
grep -E "efficiency|Speedup|Success Rate" test_output.log
```

Then I can analyze the performance of the collection proof optimizations!