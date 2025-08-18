# Claude Code Memory - Accumulate Project

## Load Testing

### Available Load Tests with Flags

The project has a comprehensive load testing system with command-line flags for configuration. DO NOT create new test versions when parameters can be changed via flags.

#### Main Test: TestStreamlinedLoad
```bash
# Configure via flags:
go test -v -run TestStreamlinedLoad -args \
  -txs 100000    # Number of transactions
  -tps 200       # Target TPS (0 = unlimited)
  -k 40          # Number of sender accounts
  -a 40          # Number of receiver accounts
  -timeout 15m   # Timeout duration
  -verbose       # Enable verbose output
```

#### Example Commands:
```bash
# 50k at 100 TPS
go test -v -run TestStreamlinedLoad -args -txs 50000 -tps 100 -k 20 -a 20

# 100k at 200 TPS
go test -v -run TestStreamlinedLoad -args -txs 100000 -tps 200 -k 40 -a 40 -timeout 15m

# 100k at 50 TPS (for stability)
go test -v -run TestStreamlinedLoad -args -txs 100000 -tps 50 -k 40 -a 40 -timeout 40m
```

### Performance Results
- 50 TPS: 100% success rate, very stable
- 100 TPS: 99%+ success rate, stable
- 200 TPS: May have lower success rate depending on system

### Devnet Configuration

**IMPORTANT: Always use the devnet_config.sh script to start devnet properly:**

```bash
# From project root:
./test/load/devnet_config.sh standard   # Standard setup (2 BVNs, 3 validators, 1 follower)
./test/load/devnet_config.sh quick      # Minimal setup (2 BVNs, 1 validator)
./test/load/devnet_config.sh large      # Large setup (3 BVNs, 3 validators, 2 followers)

# Other commands:
./test/load/devnet_config.sh stop       # Stop running devnet
./test/load/devnet_config.sh status     # Check devnet status
./test/load/devnet_config.sh clean      # Stop and clean devnet data
```

The script ensures:
- Proper cleanup of any existing devnet processes
- Correct port allocation and configuration
- Uses current local codebase (via go run)
- Waits for all partitions to be ready before returning

The devnet runs locally with:
- Base IP: 127.0.0.1 (fixed from original 127.0.1.1)
- API endpoint: http://127.0.0.1:26660/v3
- Multiple nodes on ports 26656-26700
- Smart discovery system automatically finds endpoints

### Important Commands

Check devnet status:
```bash
./test/load/devnet_config.sh status     # Best way to check status
ps aux | grep accumulated
ss -tlnp | grep 266
curl -s http://127.0.0.1:26660/metrics
```

### Key Files
- `/test/load/sl_test.go` - Main streamlined load test with flag support
- `/test/load/simple_50k_test.go` - Simple 50k test (has DEFAULTS documented)
- `/test/load/simple_100k_test.go` - Simple 100k test (has DEFAULTS documented)
- `/test/load/devnet_endpoint.go` - Smart endpoint discovery
- `/test/load/LOAD_TEST_GUIDE.md` - Full documentation of load testing

### Testing Notes
- Always use existing tests with flags instead of creating new versions
- Devnet must be running before tests
- Lower TPS provides better stability and success rates
- Tests automatically handle funding via faucet
- Tests verify real transactions on actual devnet (not mocked)