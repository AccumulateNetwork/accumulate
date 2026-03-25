# Timestamp Replay Protection Requirements

## Overview

Accumulate uses timestamp-based replay protection to ensure that signatures cannot be replayed maliciously. This document provides deployment requirements and operational guidance for validator operators to ensure proper timestamp-based security.

## Replay Protection Mechanism

### How It Works

Every transaction signature includes a timestamp field that must:
1. Be present on the initiating signature (required)
2. Be strictly greater than the last timestamp used by that key
3. Fall within acceptable bounds relative to the block time

When a signature is processed, the timestamp is stored in the key entry's `LastUsedOn` field. Subsequent signatures from the same key must have timestamps greater than this value, preventing replay attacks.

**Code Reference:** `/internal/core/execute/v2/block/sig_user.go:262-264`

```go
// Check the timestamp
if ctx.keySig.GetTimestamp() != 0 && ctx.keyEntry.GetLastUsedOn() >= ctx.keySig.GetTimestamp() {
    return errors.BadTimestamp.WithFormat("invalid timestamp: have %d, got %d",
        ctx.keyEntry.GetLastUsedOn(), ctx.keySig.GetTimestamp())
}
```

### Timestamp Requirements by Signature Type

| Signature Type | Timestamp Required | Notes |
|----------------|-------------------|-------|
| Initiating signature | **Required** | Must have non-zero timestamp |
| Secondary signature | Optional | Can be zero |
| Delegated signature | **Required** | Inner key signature must have timestamp |
| Remote signature | **Required** | Must initiate with timestamp |

## NTP Requirements for Validators

### Critical Requirement

**All validators MUST run an NTP daemon** to ensure accurate time synchronization. Clock drift directly impacts transaction processing and can cause:
- Transaction rejection due to out-of-bounds timestamps
- Replay protection failures
- Consensus timing issues

### Maximum Acceptable Clock Drift

- **Target:** ±50ms from UTC
- **Maximum tolerated:** ±100ms from UTC
- **Critical threshold:** ±5 minutes (transaction rejection boundary)

Validators with clock drift exceeding 100ms should investigate NTP configuration immediately. Drift exceeding 5 minutes will cause transaction failures.

### Recommended NTP Configuration

#### Primary NTP Servers

Use multiple reliable time sources for redundancy:

```bash
# /etc/chrony/chrony.conf (recommended for modern Linux)
server time.google.com iburst
server time1.google.com iburst
server time2.google.com iburst
server time3.google.com iburst
server 0.pool.ntp.org iburst
server 1.pool.ntp.org iburst
```

Or for `ntpd`:

```bash
# /etc/ntp.conf
server time.google.com iburst
server time1.google.com iburst
server time2.google.com iburst
server time3.google.com iburst
server 0.pool.ntp.org iburst
server 1.pool.ntp.org iburst

# Allow system clock to drift slightly rather than stepping
tinker panic 0
```

#### Recommended Time Sources

1. **Google Public NTP** (recommended primary)
   - `time.google.com`
   - `time1.google.com` through `time4.google.com`
   - Benefits: Highly accurate, globally distributed, smeared leap seconds

2. **NTP Pool Project** (recommended secondary)
   - `0.pool.ntp.org` through `3.pool.ntp.org`
   - Regional pools: `0.north-america.pool.ntp.org`, etc.
   - Benefits: Large distributed network, geographic proximity

3. **Cloud Provider NTP** (if applicable)
   - AWS: `169.254.169.123` (link-local NTP)
   - Azure: `time.windows.com`
   - GCP: `metadata.google.internal` (169.254.169.254)

#### Installation and Setup

**Ubuntu/Debian (using chrony - recommended):**
```bash
# Install chrony
sudo apt-get update
sudo apt-get install -y chrony

# Edit configuration
sudo nano /etc/chrony/chrony.conf
# Add NTP servers as shown above

# Restart service
sudo systemctl restart chrony
sudo systemctl enable chrony

# Verify synchronization
chronyc tracking
chronyc sources -v
```

**RHEL/CentOS:**
```bash
# Install chrony
sudo yum install -y chrony

# Configure and enable
sudo systemctl start chronyd
sudo systemctl enable chronyd

# Verify
chronyc tracking
```

**Traditional ntpd (legacy):**
```bash
sudo apt-get install ntp
sudo systemctl restart ntp
sudo systemctl enable ntp

# Verify
ntpq -p
```

## Timestamp Bounds Validation

### Current Implementation

As of the current implementation, timestamp bounds checking is **not yet enforced** at the protocol level. Timestamps are validated for monotonic increase per key, but not checked against block time bounds.

### Planned Bounds (Post-Launch Enhancement)

Future versions will enforce timestamp bounds relative to block time:

- **Lower bound:** Block time - 5 minutes
- **Upper bound:** Block time + 5 minutes

This tolerance allows for:
- Normal clock drift (up to 100ms recommended)
- Network propagation delays
- Clock skew recovery without transaction loss

**Rationale for ±5 minutes:**
- Provides reasonable tolerance for operational issues
- Prevents egregiously backdated or future-dated signatures
- Aligns with transaction expiration policy
- Balances security with operational flexibility

### Transaction Expiration Policy

Transactions remain valid within the timestamp tolerance window:
- **Maximum age:** 5 minutes from creation
- **Future tolerance:** 5 minutes ahead of block time
- **Default expiry:** 5 minutes (configurable in client SDKs)

**Code Reference:** `/pkg/consensus/txtracker/tracker.go:78`
```go
DefaultExpiryDuration = 5 * time.Minute // Transaction tracking expiry
```

## Monitoring and Alerting

### Essential Metrics

Monitor these metrics on all validators:

#### 1. Clock Drift
```bash
# Check current drift with chrony
chronyc tracking | grep "System time"

# Example output:
# System time     : 0.000012345 seconds slow of NTP time
```

**Alert thresholds:**
- **Warning:** Drift > 50ms
- **Critical:** Drift > 100ms

#### 2. NTP Synchronization Status
```bash
# Verify NTP sync status
timedatectl status

# Check if systemd-timesyncd is active
systemctl status systemd-timesyncd

# Or for chrony
systemctl status chrony
```

**Alert if:** NTP service is not running or not synchronized

#### 3. Timestamp Rejection Rate
Monitor transaction rejection errors:
```bash
# Search logs for BadTimestamp errors
grep "BadTimestamp" /var/log/accumulate/accumulated.log

# Or via metrics endpoint (if available)
curl http://localhost:8080/metrics | grep timestamp_errors
```

**Alert if:** Timestamp error rate > 1% of transactions

### Monitoring Script Example

```bash
#!/bin/bash
# check-time-sync.sh - Monitor NTP synchronization

DRIFT_WARN_MS=50
DRIFT_CRIT_MS=100

# Get current drift in milliseconds
DRIFT=$(chronyc tracking | grep "System time" | awk '{print $4 * 1000}')
DRIFT_ABS=${DRIFT#-}  # Absolute value

if (( $(echo "$DRIFT_ABS > $DRIFT_CRIT_MS" | bc -l) )); then
    echo "CRITICAL: Clock drift is ${DRIFT}ms (exceeds ${DRIFT_CRIT_MS}ms)"
    exit 2
elif (( $(echo "$DRIFT_ABS > $DRIFT_WARN_MS" | bc -l) )); then
    echo "WARNING: Clock drift is ${DRIFT}ms (exceeds ${DRIFT_WARN_MS}ms)"
    exit 1
else
    echo "OK: Clock drift is ${DRIFT}ms"
    exit 0
fi
```

### Prometheus Metrics (Recommended)

Export NTP metrics for centralized monitoring:

```yaml
# node_exporter textfile collector
# /var/lib/node_exporter/textfile_collector/ntp.prom
ntp_drift_seconds{source="chrony"} 0.000012
ntp_synchronized{source="chrony"} 1
ntp_stratum{source="chrony"} 2
```

**Grafana Dashboard:**
- Graph: NTP drift over time
- Alert: Drift > 100ms for 5 minutes
- Alert: NTP not synchronized for 1 minute

## Troubleshooting

### Issue: High Timestamp Rejection Rate

**Symptoms:** Transactions failing with `BadTimestamp` errors

**Causes:**
1. Clock drift exceeds tolerance
2. NTP service not running
3. Client clocks out of sync
4. Attempted replay attack

**Resolution:**
```bash
# 1. Check validator clock drift
chronyc tracking

# 2. Force time synchronization
sudo chronyc -a makestep

# 3. Restart NTP service
sudo systemctl restart chrony

# 4. Verify synchronization
chronyc sources -v

# 5. Check transaction submission client clocks
# Ensure client systems also run NTP
```

### Issue: NTP Service Not Synchronizing

**Symptoms:** `chronyc tracking` shows "Not synchronized"

**Causes:**
1. Firewall blocking NTP (UDP port 123)
2. Unreachable NTP servers
3. Network connectivity issues
4. Large initial clock offset

**Resolution:**
```bash
# 1. Check NTP server reachability
chronyc sources -v
# Look for "^*" (current best source) or "^+" (acceptable source)

# 2. Verify firewall allows NTP
sudo ufw status | grep 123
sudo iptables -L -n | grep 123

# 3. Allow NTP if blocked
sudo ufw allow 123/udp

# 4. Force step if large offset
sudo chronyc -a makestep

# 5. Check chrony logs
sudo journalctl -u chrony -n 100
```

### Issue: Sudden Clock Step

**Symptoms:** Batch of timestamp errors after time correction

**Explanation:** If the system clock steps forward or backward significantly, in-flight transactions may have timestamps that are now invalid.

**Resolution:**
- **Prevention:** Configure NTP to slew (gradual adjustment) rather than step
- **Mitigation:** Affected clients should retry transactions with new timestamps
- **Recovery:** Normal operations resume once new transactions are created

**Chrony configuration to minimize stepping:**
```bash
# /etc/chrony/chrony.conf
# Allow gradual slewing for small offsets
makestep 0.1 3  # Step only if offset > 0.1s in first 3 updates
```

## Pre-Launch Deployment Checklist

Use this checklist when deploying validator nodes:

### Time Synchronization
- [ ] NTP daemon (chrony or ntpd) installed
- [ ] NTP configuration includes multiple reliable sources
- [ ] NTP service enabled and running
- [ ] Initial time synchronization verified (`chronyc tracking`)
- [ ] Clock drift < 100ms confirmed
- [ ] Firewall allows UDP port 123 (NTP)

### Monitoring
- [ ] NTP monitoring script deployed
- [ ] Clock drift alerts configured (50ms warning, 100ms critical)
- [ ] NTP service health check in place
- [ ] Metrics exported to monitoring system (Prometheus/Grafana)
- [ ] Transaction timestamp error monitoring enabled

### Documentation
- [ ] Operator runbook includes NTP troubleshooting
- [ ] Escalation procedures documented
- [ ] Time synchronization requirements communicated to team

### Testing
- [ ] Submit test transactions and verify acceptance
- [ ] Simulate clock drift and verify recovery
- [ ] Test NTP failover (disable primary server)
- [ ] Verify monitoring alerts trigger correctly

## Security Considerations

### Replay Attack Prevention

The timestamp-based replay protection prevents:
1. **Signature replay:** Same signature cannot be reused across transactions
2. **Transaction replay:** Same transaction cannot be submitted twice
3. **Backdated transactions:** Transactions cannot be artificially backdated

### Clock Skew Attack Surface

**Potential attack:** Adversary manipulates validator clock to:
- Accept backdated transactions
- Reject legitimate current transactions
- Cause consensus timing issues

**Mitigations:**
1. **NTP authentication:** Use NTP authentication keys (NTS - NTP Security)
2. **Multiple time sources:** Rely on consensus among multiple NTP servers
3. **Monitoring:** Detect and alert on abnormal clock behavior
4. **Bounds enforcement:** Future timestamp bounds checking will limit impact

### NTP Security (Advanced)

For high-security deployments, consider:

**Network Time Security (NTS):**
```bash
# Chrony with NTS support
server time.cloudflare.com iburst nts
server ntppool1.time.nl iburst nts
server nts.ntp.se iburst nts
```

**Benefits:** Cryptographic authentication prevents NTP spoofing

## Future Enhancements

### Planned Improvements (Issue #3872 and Related)

1. **Timestamp Bounds Enforcement** (Month 3, Post-Launch)
   - Implement ±5 minute bounds checking in executor
   - Add configuration option for custom bounds
   - File: `/internal/core/execute/v2/block/sig_user.go`

2. **Enhanced Monitoring**
   - Built-in timestamp metrics in accumulated
   - Prometheus endpoint for clock drift
   - Grafana dashboard templates

3. **Client SDK Improvements**
   - Automatic timestamp generation
   - Retry logic for timestamp errors
   - Client-side clock validation

4. **Operator Tools**
   - CLI command to check validator clock health
   - Automated NTP configuration checker
   - Network-wide time sync status dashboard

## References

- **Issue #3872:** Document timestamp replay protection requirements
- **PRODUCTION-SECURITY-PLAN.md:** Production security roadmap
- **Code:** `/internal/core/execute/v2/block/sig_user.go` - User signature validation
- **Code:** `/test/e2e/replay_test.go` - Replay protection test cases
- **Specification:** Accumulate Protocol Specification (timestamp requirements)

## Contact and Support

For questions about timestamp requirements or NTP configuration:
- **Protocol Team:** Accumulate core developers
- **DevOps Team:** Infrastructure and monitoring support
- **Security Team:** Security implications and threat modeling

---

**Document Version:** 1.0
**Last Updated:** 2026-03-25
**Status:** Production deployment guidance
