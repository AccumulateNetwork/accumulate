# Simulator Tool

## Overview

The `simulator` tool provides a standalone network simulator for the Accumulate Network. It creates a complete blockchain network environment for testing, development, and debugging without requiring a full network deployment.

## Installation

```bash
# Build the simulator tool
go build -o bin/simulator ./tools/cmd/simulator

# Or build all tools
make tools
```

## Usage

```bash
./bin/simulator [flags]
```

## Basic Usage

### Start Simple Simulator

```bash
# Start with default settings (3 BVNs, 1 DN)
./bin/simulator

# Start with custom configuration
./bin/simulator --bvn-count 5 --port 8080
```

### Background Mode

```bash
# Start in background
./bin/simulator --background --port 8080

# Check if running
curl http://localhost:8080/v2/status
```

## Configuration Options

### Network Configuration

```bash
# Set number of BVNs (Block Validator Networks)
./bin/simulator --bvn-count 3

# Set number of Directory Networks
./bin/simulator --dn-count 1

# Custom network topology
./bin/simulator --bvn-count 5 --dn-count 2
```

### Server Configuration

```bash
# Set HTTP port
./bin/simulator --port 8080

# Set listen address
./bin/simulator --listen 127.0.0.1:8080

# Enable CORS
./bin/simulator --cors
```

### Database Configuration

```bash
# Use persistent database
./bin/simulator --database ./sim-data

# Use in-memory database (default)
./bin/simulator --memory

# Custom database path
./bin/simulator --database /tmp/accumulate-sim
```

## Advanced Configuration

### Genesis Configuration

```bash
# Custom genesis file
./bin/simulator --genesis ./custom-genesis.json

# Include Factom addresses
./bin/simulator --factom-addresses ./factom-addresses.csv

# Custom initial state
./bin/simulator --initial-state ./initial-state.json
```

### Logging Configuration

```bash
# Set log level
./bin/simulator --log-level debug

# JSON logging
./bin/simulator --log-format json

# Log to file
./bin/simulator --log-file ./simulator.log
```

### Performance Tuning

```bash
# Adjust block time
./bin/simulator --block-time 1s

# Set transaction batch size
./bin/simulator --batch-size 100

# Memory limits
./bin/simulator --memory-limit 1GB
```

## API Endpoints

The simulator exposes the same API as the full Accumulate Network:

### Status Endpoints

```bash
# Network status
curl http://localhost:8080/v2/status

# Metrics
curl http://localhost:8080/v2/metrics

# Health check
curl http://localhost:8080/v2/health
```

### Transaction Endpoints

```bash
# Submit transaction
curl -X POST http://localhost:8080/v2/submit \
  -H "Content-Type: application/json" \
  -d @transaction.json

# Query transaction
curl http://localhost:8080/v2/transaction/abc123
```

### Account Endpoints

```bash
# Query account
curl http://localhost:8080/v2/account/acc://alice

# Query account history
curl http://localhost:8080/v2/account/acc://alice/history
```

## Integration with Testing

### E2E Test Integration

```go
// In your test file
func TestWithSimulator(t *testing.T) {
    // Start simulator
    sim := simulator.New(t, 3)
    sim.InitFromGenesis()
    
    // Your test code here
    // ...
    
    // Simulator automatically cleaned up
}
```

### Manual Testing

```bash
# 1. Start simulator
./bin/simulator --port 8080 --background

# 2. Run tests
go test ./test/e2e/... -simulator-url http://localhost:8080

# 3. Stop simulator
pkill simulator
```

### CI/CD Integration

```yaml
test_with_simulator:
  stage: test
  before_script:
    - ./bin/simulator --port 8080 --background --log-file simulator.log
    - sleep 5  # Wait for startup
  script:
    - go test ./test/e2e/... -simulator-url http://localhost:8080
  after_script:
    - pkill simulator || true
  artifacts:
    when: on_failure
    paths:
      - simulator.log
```

## Development Workflows

### Local Development

```bash
# Terminal 1: Start simulator with live reload
./bin/simulator --port 8080 --log-level debug

# Terminal 2: Run tests
go test ./test/e2e/TestFactomAddresses -v

# Terminal 3: Monitor logs
tail -f ~/.accumulate/simulator.log
```

### Debugging Workflow

```bash
# Start simulator with debug options
./bin/simulator \
  --port 8080 \
  --log-level debug \
  --log-format json \
  --database ./debug-sim \
  --cors

# Use debug tool to analyze
./bin/debug network --simulator http://localhost:8080
```

## Configuration File

Create `simulator.yaml`:

```yaml
network:
  bvn_count: 3
  dn_count: 1
  
server:
  port: 8080
  listen: "127.0.0.1:8080"
  cors: true
  
database:
  path: "./sim-data"
  memory: false
  
genesis:
  file: "./genesis.json"
  factom_addresses: "./factom-addresses.csv"
  
logging:
  level: "info"
  format: "text"
  file: "./simulator.log"
  
performance:
  block_time: "1s"
  batch_size: 100
  memory_limit: "1GB"
```

Use with:

```bash
./bin/simulator --config simulator.yaml
```

## Monitoring and Observability

### Metrics Collection

```bash
# Enable metrics endpoint
./bin/simulator --metrics --metrics-port 9090

# Scrape metrics
curl http://localhost:9090/metrics
```

### Health Checks

```bash
# Basic health check
curl http://localhost:8080/v2/health

# Detailed status
curl http://localhost:8080/v2/status | jq .
```

### Log Analysis

```bash
# Follow logs
tail -f simulator.log

# Filter errors
grep ERROR simulator.log

# JSON log analysis
cat simulator.log | jq 'select(.level == "error")'
```

## Troubleshooting

### Common Issues

| Issue | Solution |
|-------|----------|
| Port already in use | Use `--port` to specify different port |
| Database locked | Remove lock file or use `--memory` |
| Genesis failure | Check genesis file format and permissions |
| Network timeout | Increase startup wait time |
| Memory issues | Use `--memory-limit` or `--memory` mode |

### Debug Commands

```bash
# Check simulator status
curl -s http://localhost:8080/v2/status | jq .

# Verify network topology
curl -s http://localhost:8080/v2/network | jq .

# Check active connections
netstat -an | grep 8080
```

### Performance Issues

```bash
# Monitor resource usage
top -p $(pgrep simulator)

# Check database size
du -sh ./sim-data

# Analyze slow queries
./bin/debug database --path ./sim-data --slow-queries
```

## Examples

### Complete Development Setup

```bash
#!/bin/bash
# setup-dev-env.sh

# Build tools
make tools

# Start simulator
./bin/simulator \
  --port 8080 \
  --bvn-count 3 \
  --database ./dev-sim \
  --log-level debug \
  --background

# Wait for startup
sleep 5

# Verify simulator is running
curl -s http://localhost:8080/v2/status || {
  echo "Simulator failed to start"
  exit 1
}

echo "Development environment ready!"
echo "Simulator: http://localhost:8080"
echo "Logs: tail -f simulator.log"
```

### Test Automation Script

```bash
#!/bin/bash
# run-e2e-tests.sh

set -e

# Start simulator
./bin/simulator --port 8080 --memory --background
SIM_PID=$!

# Cleanup function
cleanup() {
  kill $SIM_PID 2>/dev/null || true
}
trap cleanup EXIT

# Wait for simulator
sleep 3

# Run tests
go test ./test/e2e/... -simulator-url http://localhost:8080 -v

echo "All tests passed!"
```

## See Also

- [E2E Testing Guide](../../test/docs/e2e-tests.md) - End-to-end testing strategies
- [Debug Tool](debug.md) - Debugging utilities
- [Network Tools](genesis.md) - Genesis block utilities
