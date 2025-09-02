# Recovery Testing Feature - Summary

## Quick Start

```bash
# Enable recovery testing with 3 drops per minute
./accumulated --dpm 3

# Monitor logs for recovery activity
tail -f /var/log/accumulated.log | grep "RECOVERY TEST\|Gap detected"
```

## What It Does

**Tests the complete crosschain healing mechanism by:**

1. **Randomly drops** anchor and synthetic messages (at specified rate)
2. **Detects gaps** in sequence numbers on receiving side
3. **Sends recovery requests** to source partitions
4. **Verifies healing** when source partitions resend missing messages

## Security (Production Safe)

**Dual-gate security ensures NEVER activates in production:**

✅ **Gate 1**: Requires `--dpm > 0` command line flag  
✅ **Gate 2**: Requires active faucet (only exists in test networks)

**Both must be present** - if either missing, testing disabled.

## Usage Examples

```bash
# Light testing - 1 drop per minute
./accumulated --dpm 1

# Normal testing - 3 drops per minute  
./accumulated --dpm 3

# Stress testing - 10 drops per minute
./accumulated --dpm 10

# Disabled (production mode)
./accumulated
```

## Key Architecture

### Why Anchors and Synthetics Are Separate

**Anchor Transactions:**
- 🔒 **Permanent data** - required for cryptographic proofs
- 🔒 **Cannot be pruned** - must be kept indefinitely  
- 📊 **Tracked separately** for long-term retention

**Synthetic Transactions:**
- ⚡ **Temporary data** - can be pruned after processing
- ⚡ **Different lifecycle** - optimized for quick processing
- 📊 **Tracked separately** for efficient cleanup

This separation is **essential for proper data lifecycle management**.

## Monitoring

**Key Log Messages:**
```
INFO Recovery testing ENABLED - faucet + drops per minute detected
INFO RECOVERY TEST: Dropping message to test recovery mechanism  
INFO Gap detected in synthetic/anchor messages
INFO RECOVERY TEST: Recovery mechanism triggered
INFO Adjusted send position for gap recovery
```

**Metrics:**
```json
{
  "enabled": true,
  "drops_per_minute": 3,
  "total_dropped": 45,
  "anchors_dropped": 18,
  "synthetics_dropped": 27,
  "recovery_triggered": 42,
  "drops_this_minute": 2
}
```

## Files Modified

- `recovery_testing.go` - Core testing framework (NEW)
- `conductor.go` - Integrated message dropping and gap detection  
- `sequence_tracker_simple.go` - Gap detection logic (EXISTING)
- `destination_state.go` - Send index tracking (EXISTING)
- `docs/testing/RECOVERY_TESTING.md` - Complete documentation (NEW)

## Complete Implementation ✅

The feature provides **safe, comprehensive testing** of the crosschain healing mechanism:

1. **✅ Message dropping** - Simulates network failures
2. **✅ Gap detection** - ProcessInbound detects missing sequences  
3. **✅ Recovery requests** - Destinations alert sources
4. **✅ Healing verification** - Sources resend from gap point
5. **✅ Production safety** - Dual-gate prevents accidental activation

Perfect for testing the robustness of crosschain communication!