# DevNet Automation and Load Testing

This directory contains automated tools for managing Accumulate DevNet and running load tests.

## Quick Start

```bash
# Full restart (kill + clean + compile + start + test)
./devnet_manager.sh

# Or explicitly:
./devnet_manager.sh restart
```

## DevNet Manager Commands

| Command | Description |
|---------|-------------|
| `./devnet_manager.sh restart` | Full cycle: kill → clean → compile → start → test |
| `./devnet_manager.sh kill` | Kill existing devnet processes |
| `./devnet_manager.sh clean` | Kill processes + clean data directory |
| `./devnet_manager.sh compile` | Compile new accumulate version |
| `./devnet_manager.sh start` | Start fresh devnet |
| `./devnet_manager.sh test` | Run load tests against running devnet |
| `./devnet_manager.sh status` | Check devnet status and show logs |

## Files

- **`devnet_manager.sh`** - Main automation script
- **`main_branch.go`** - Load test with 100% success rate
- **`devnet.log`** - DevNet runtime logs
- **`devnet.pid`** - Process ID of running devnet
- **`devnet_manager.log`** - Script execution logs

## Load Test Results

The current load test achieves:
- **100% Success Rate** (20/20 transactions)
- **~5 TPS** throughput
- **Full lite account lifecycle**: creation → funding → credits → transfers

## Troubleshooting

1. **Check Status**: `./devnet_manager.sh status`
2. **View Logs**: `tail -f devnet.log` or `tail -f devnet_manager.log`
3. **Hard Reset**: `./devnet_manager.sh clean` then `./devnet_manager.sh restart`

## Technical Details

### DevNet Configuration
- **API Port**: 27004 (http://127.0.0.1:27004)
- **Data Directory**: `../../.devnet-test`
- **Network**: 3 BVNs + Directory Network
- **Startup Time**: ~15-20 seconds

### Load Test Process
1. **Account Creation**: Generate 5 lite accounts cryptographically
2. **Funding**: 3x faucet calls per account (30 ACME total)
3. **Credits**: Add credits via AddCredits transaction (1 ACME → credits)
4. **Transfers**: 20 concurrent token transfers between accounts
5. **Results**: Success rate, TPS, error analysis

### Key Fixes Applied
- ✅ **Signing Authority**: Use lite identity (not token URL) for signing
- ✅ **Credits**: Automated AddCredits after funding
- ✅ **Port Detection**: Automatic devnet port detection
- ✅ **Health Checks**: Wait for API availability before testing