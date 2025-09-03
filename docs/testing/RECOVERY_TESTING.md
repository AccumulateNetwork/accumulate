# Recovery Testing Framework

## Overview

The Recovery Testing Framework provides a safe mechanism to test the crosschain healing system by randomly dropping anchor and synthetic transactions. This simulates network failures and verifies that the gap detection and recovery mechanisms work correctly.

## Security Design

**CRITICAL**: This feature can **NEVER activate in production** due to dual security requirements:

### Security Requirement 1: DPM Flag
```bash
./accumulated run devnet --dpm 3  # 3 drops per minute
```
Must be explicitly set as command line argument with value > 0.

### Security Requirement 2: Active Faucet
The system automatically detects if a faucet is active in the network configuration. Faucets only exist in test environments, never in production.

**Both conditions must be met** - if either is missing, recovery testing remains disabled.

## How It Works

### 1. Initialization
The devnet command automatically configures recovery testing:
```bash
# DevNet sets environment variable based on --dpm flag
./accumulated run devnet --dpm 5  # Sets ACCUMULATE_DPM=5

# CrossChain Conductor reads environment variable directly 
dropsPerMinute := os.Getenv("ACCUMULATE_DPM")  // Gets "5"
cc.recoveryTestConfig = NewRecoveryTestConfig(logger, describe, dropsPerMinute)
```

**Architecture:**
- **Runtime-only**: DPM never persisted to configuration files
- **Environment-based**: Direct propagation via ACCUMULATE_DPM environment variable  
- **Process-wide**: All partitions inherit the same DPM setting
- **No reboot survival**: DPM flag must be specified on every start (by design)

### 2. Message Dropping
During normal crosschain message transmission:
- **Time-based dropping** at specified rate (e.g., --dpm 5 = 5 drops per minute)
- **Only crosschain messages** (anchors/synthetics) are eligible
- **Rate limited** to exact drops per minute target
- **Minimum interval** between drops to achieve target rate
- **Clear logging** marks all drops as "RECOVERY TEST"

#### Drop Rate Calculation
```go
// For --dpm 5 (5 drops per minute):
minDropInterval := time.Minute / 5  // = 12 seconds between drops
if time.Since(lastDrop) >= minDropInterval {
    // Drop this message
}
```

### 3. Gap Detection
When dropped messages create gaps:
- `ProcessInbound` detects missing sequence numbers
- Recovery requests are sent to source partitions  
- Metrics track recovery trigger events

### 4. Recovery Verification
Source partitions respond to recovery requests:
- Reset send index to gap point
- Resend all missing messages
- Complete healing cycle verified

## Configuration Examples

### Basic Recovery Testing
```bash
# Start DevNet with 5 message drops per minute
./accumulated run devnet --dpm 5 --bvns 3 --validators 2

# Start DevNet with minimal setup and recovery testing  
./accumulated run devnet --dpm 3 --bvns 2 --validators 1

# Disable recovery testing (default)
./accumulated run devnet --dpm 0  # or omit --dpm entirely
```

### Advanced Usage
```bash
# High frequency testing (10 drops per minute = every 6 seconds)
./accumulated run devnet --dpm 10

# Low frequency testing (1 drop per minute = every 60 seconds)  
./accumulated run devnet --dpm 1

# Debug mode with recovery testing
./accumulated run devnet --dpm 5 --debug
```

## Architecture

### Simplified DPM Propagation
The recovery testing system uses a simplified architecture for flag propagation:

```
CLI Flag (--dpm 5) → Environment Variable (ACCUMULATE_DPM=5) → All Partitions
```

**Benefits:**
- **Runtime-only**: No configuration file persistence
- **Process-wide**: All partitions inherit the same DMP setting  
- **Simple**: Direct environment variable access
- **Safe**: Must be specified on every start (never survives reboot)

### Key Components
1. **DevNet Command**: Sets `ACCUMULATE_DPM` environment variable
2. **Consensus Module**: Reads environment variable directly
3. **CrossChain Conductor**: Always enabled, gets DPM value from execute options
4. **Recovery Testing**: Activates when DPM > 0 AND faucet detected

## Implementation Details

### Command Line Arguments

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--dpm` | Yes | `0` | Drops per minute (0 = disabled, >0 = enabled) |

### Example Configuration
```bash
# Enable recovery testing with 5 drops per minute
./accumulated --dpm 5

# Enable with higher drop rate for stress testing
./accumulated --dpm 10
```

## Usage

### 1. Enable Testing
```bash
./accumulated --dpm 3  # 3 drops per minute
```

### 2. Monitor Logs
Look for recovery testing activity:
```
INFO RECOVERY TEST: Dropping message to test recovery mechanism
INFO Gap detected in synthetic messages
INFO RECOVERY TEST: Recovery mechanism triggered
INFO Adjusted send position for gap recovery
```

### 3. Check Metrics
Query recovery test metrics via conductor:
```go
metrics := conductor.GetRecoveryTestMetrics()
fmt.Printf("Drops: %d, Recovery Events: %d\n", 
    metrics["total_dropped"], 
    metrics["recovery_triggered"])
```

## Expected Behavior

### Normal Operation (No Gaps)
```
2025/09/02 12:00:01 INFO Processing inbound cross-partition messages count=10
2025/09/02 12:00:01 DEBUG All messages valid, no gaps detected
```

### With Recovery Testing Active
```
2025/09/02 12:00:01 INFO Recovery testing ENABLED - faucet + test flag detected
2025/09/02 12:00:05 INFO RECOVERY TEST: Dropping message to test recovery mechanism
2025/09/02 12:00:10 INFO Gap detected in synthetic messages source=acc://bvn1.acme sequence=42
2025/09/02 12:00:10 INFO RECOVERY TEST: Recovery mechanism triggered
2025/09/02 12:00:11 INFO Gap recovery request requester=acc://bvn0.acme fromNumber=42
2025/09/02 12:00:11 INFO Adjusted send position for gap recovery willResendFrom=42
```

## Metrics

### Recovery Test Metrics
```json
{
  "enabled": true,
  "drop_rate": 0.05,
  "max_drops_per_hour": 100,
  "total_dropped": 23,
  "anchors_dropped": 8,
  "synthetics_dropped": 15,
  "recovery_triggered": 19,
  "drops_this_hour": 12,
  "WARNING": "THIS SHOULD NEVER BE ENABLED IN PRODUCTION"
}
```

### Partition Health Metrics
```json
{
  "total_queued": 0,
  "total_pending": 0,
  "blocked_queues": 0,
  "destinations_with_backlog": {}
}
```

## Testing Scenarios

### 1. Basic Recovery Test
```bash
# Enable with low drop rate - 2 drops per minute
./accumulated --dpm 2
```

### 2. Stress Recovery Test
```bash
# Higher drop rate for intensive testing - 10 drops per minute
./accumulated --dpm 10
```

### 3. Anchor vs Synthetic Recovery
Monitor separate tracking:
- Anchor drops test permanent data recovery
- Synthetic drops test temporary data recovery
- Verify both types heal correctly

## Architecture Notes

### Why Anchors and Synthetics Are Separate

**Anchor Transactions:**
- ✅ **Permanent data** - required for cryptographic proofs
- ✅ **Cannot be pruned** - must be kept indefinitely
- ✅ **Tracked separately** for long-term retention

**Synthetic Transactions:**
- ✅ **Temporary data** - can be pruned after processing
- ✅ **Different lifecycle** - optimized for quick processing
- ✅ **Tracked separately** for efficient cleanup

This separation is **essential for proper data lifecycle management**.

## Production Safety Verification

### How to Verify It's Disabled in Production

1. **Check Command Line Arguments**
   ```bash
   ps aux | grep accumulated  # Should NOT show --dpm flag
   ```

2. **Check Faucet Status**
   - Production networks have no faucet
   - System automatically detects this

3. **Check Logs**
   ```bash
   grep "Recovery testing" /var/log/accumulated.log
   # Should show: "Recovery testing disabled - no active faucet detected"
   ```

4. **Query Metrics**
   ```bash
   curl /metrics | grep recovery_test
   # Should show: {"enabled": false, "reason": "faucet + --dpm flag required"}
   ```

## Troubleshooting

### Recovery Testing Not Working

**Problem**: Recovery testing doesn't activate
**Solutions**:
1. Verify `--dpm > 0` flag is set
2. Verify you're running on a test network with faucet
3. Check logs for "Recovery testing disabled" messages

### Too Many Drops

**Problem**: Excessive message dropping
**Solutions**:
1. Lower drop rate: `--dpm 1` (1 drop per minute)
2. Check per-minute rate limiting is working
3. Verify metrics show reasonable drop counts

### Recovery Not Triggering

**Problem**: Messages dropped but no recovery detected
**Solutions**:
1. Check sequence tracker is initialized
2. Verify gap detection logic in `ProcessInbound`
3. Check source partition `HandleRecoveryRequest` is working

## Implementation Files

- `recovery_testing.go` - Core testing framework
- `conductor.go:ProcessInbound()` - Gap detection and recovery triggering
- `conductor.go:HandleRecoveryRequest()` - Recovery response handling  
- `sequence_tracker_simple.go` - Gap detection logic
- `destination_state.go` - Send index tracking and reset

## Future Enhancements

### Potential Additions
1. **Message type specific drop rates** - Different rates for anchors vs synthetics
2. **Burst dropping** - Drop multiple consecutive messages to test larger gaps
3. **Recovery latency testing** - Measure time from drop to recovery completion
4. **Partition-specific testing** - Target specific source/destination pairs

### Testing Integration
1. **Automated test suites** - Scripts that enable recovery testing and verify healing
2. **Load testing integration** - Combine with existing load tests
3. **CI/CD integration** - Automated recovery testing in test pipelines
4. **Metrics dashboards** - Real-time recovery testing visualization

## Warning

⚠️ **NEVER enable this in production environments**

The recovery testing framework is designed to be impossible to activate in production, but always verify:
- Environment variables are not set in production
- Production networks have no faucet
- Logs confirm "Recovery testing disabled"

This feature is **only for testing the healing mechanism** in development and test environments.