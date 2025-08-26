# DevNet Load Test Documentation

## Quick Start

The devnet load test is located in `test/load/` directory and consists of several scripts for testing the Accumulate devnet.

## Important Note on Port Configuration
The devnet actually runs on **port 27004** (not 26660 as some defaults suggest). The devnet manager script automatically configures this correctly.

## Main Scripts

### 1. DevNet Load Test (`test/load/devnet_load_test.sh`)
**Purpose:** Performs load testing on a running devnet by sending concurrent JSON-RPC requests.

**Usage:**
```bash
cd test/load
./devnet_load_test.sh [DEVNET_URL] [NUM_REQUESTS] [CONCURRENT_WORKERS]
```

**Parameters:**
- `DEVNET_URL`: Target devnet endpoint (default: `http://127.0.0.1:26660/v2`, but actual devnet runs on `http://127.0.0.1:27004/v2`)
- `NUM_REQUESTS`: Total number of requests to send (default: 50)
- `CONCURRENT_WORKERS`: Number of concurrent workers (default: 5)

**Example:**
```bash
# Run with correct port
./devnet_load_test.sh http://127.0.0.1:27004/v2 50 5

# More intensive test
./devnet_load_test.sh http://127.0.0.1:27004/v2 500 20
```

**What it does:**
- Tests devnet connectivity first
- Sends alternating query and describe requests
- Measures response times and success rates
- Calculates requests per second (TPS)
- Reports success/failure statistics

### 2. DevNet Manager (`test/load/devnet_manager.sh`)
**Purpose:** Manages the devnet lifecycle - kills existing devnet, compiles new version, launches fresh devnet, and runs basic tests.

**Usage:**
```bash
cd test/load
./devnet_manager.sh
```

**What it actually does:**
1. Kills any existing devnet processes (by process name and port)
2. Cleans the `.devnet-test` directory
3. Compiles the accumulate binary using `go install ./cmd/accumulated`
4. Starts devnet using: `go run ./cmd/accumulated run devnet -w .devnet-test --port 27000`
5. Waits for devnet to be ready (checks port 27004)
6. Attempts to run load tests automatically
7. Creates log files: `devnet.log` and `devnet_manager.log`

**Important Details:**
- The devnet runs on port 27004 for API access (even though started with --port 27000)
- Process runs in background with PID saved to `devnet.pid`
- Logs are written to `test/load/devnet.log`

### 3. Quick Test (`test/load/quick_test.sh`)
**Purpose:** Runs a quick smoke test on the devnet.

### 4. Full Test Suite (`test/load/run_full_test_suite.sh`)
**Purpose:** Runs comprehensive test suite including crosschain tests.

### 5. CrossChain Test (`test/load/run_crosschain_test.sh`)
**Purpose:** Specifically tests crosschain functionality.

## Other Related Scripts

- `consolidated_load_test.go`: Go-based load test implementation
- `devnet_config.sh`: Configuration settings for devnet
- `load_test_runner.sh`: General load test runner
- `partition_manager.sh`: Manages partition testing
- `run_visual_monitor.sh`: Visual monitoring of devnet
- `test_partition_control.sh`: Tests partition control functionality

## Running a Complete Test

### Method 1: All-in-One (Recommended)
```bash
cd test/load
./devnet_manager.sh  # This starts devnet AND runs tests
```

### Method 2: Manual Steps
1. **Start DevNet Only:**
```bash
cd /home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate2/accumulate
go run ./cmd/accumulated run devnet -w .devnet-test --port 27000 &
```

2. **Wait for DevNet to be ready** (usually 10-15 seconds)
   - Check if ready: `curl -s http://127.0.0.1:27004/v2`

3. **Run Load Test:**
```bash
cd test/load
./devnet_load_test.sh http://127.0.0.1:27004/v2 50 5
```

4. **For more intensive testing:**
```bash
# 500 requests with 20 concurrent workers
./devnet_load_test.sh http://127.0.0.1:27004/v2 500 20
```

## Expected Output

Successful load test output:
```
DevNet Load Test
================
Target: http://127.0.0.1:26660/v2
Requests: 50
Workers: 5

Testing DevNet connectivity...
✓ DevNet is accessible

Starting load test...
Requests per worker: 10
...
Load Test Results
=================
Total requests: 50
Successful requests: 50
Failed requests: 0
Success rate: 100.00%
Total time: 5.23s
Requests per second: 9.56
✓ All requests successful!
```

## Troubleshooting

### DevNet not accessible
- Check if devnet is running: `ps aux | grep accumulated`
- Check correct port: `lsof -i:27004` (NOT 26660!)
- Check devnet logs: `tail -f test/load/devnet.log`
- Restart devnet: `./devnet_manager.sh`

### Common Issues
1. **Wrong port**: The devnet runs on 27004, not 26660
2. **Connection errors during startup**: Normal - the devnet takes 10-15 seconds to stabilize
3. **"stat main_branch.go: no such file"**: Harmless error from devnet_manager.sh, can be ignored

### Low success rate
- Check devnet logs for errors
- Reduce concurrent workers
- Check system resources (CPU, memory)

### Performance issues
- Monitor with `./run_visual_monitor.sh`
- Check network latency
- Verify no other processes consuming resources

## Configuration Files

- `devnet_config.sh`: Contains devnet configuration parameters
- Port settings, node counts, etc.

## Log Files

- `devnet_manager.log`: DevNet lifecycle management logs
- `/tmp/worker_*_results`: Temporary worker result files (cleaned up automatically)

## Integration with CrossChain Conductor

The load tests can be used to verify CrossChain Conductor performance:
1. Ensure CrossChain Conductor changes are compiled
2. Run devnet with new binary
3. Execute load tests to verify message throughput
4. Monitor for any degradation in TPS or success rates

## Notes

- **IMPORTANT**: DevNet API runs on port **27004** (not 26660 as some scripts may default to)
- The devnet is started with `--port 27000` but the API endpoint is on 27004
- Tests alternate between light (describe) and heavier (query) requests
- Workers have 0.1s delay between requests to prevent overwhelming
- Results are aggregated from all workers for final statistics
- The devnet_manager.sh script handles all the complexity automatically

## Verified Working Examples

These commands were tested and confirmed working:

```bash
# Start devnet and run tests automatically
cd test/load
./devnet_manager.sh

# Run basic load test (50 requests, 5 workers)
./devnet_load_test.sh http://127.0.0.1:27004/v2 50 5
# Result: 100% success, ~47 requests/second

# Run intensive load test (500 requests, 20 workers)
./devnet_load_test.sh http://127.0.0.1:27004/v2 500 20
# Result: 100% success, ~183 requests/second
```