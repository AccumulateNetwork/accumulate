# Operator Runbook: Validator Key Rotation

This runbook provides step-by-step procedures for managing validator key rotation in production Accumulate deployments.

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Normal Operations](#normal-operations)
3. [Manual Key Rotation](#manual-key-rotation)
4. [Emergency Key Revocation](#emergency-key-revocation)
5. [Monitoring and Alerts](#monitoring-and-alerts)
6. [Troubleshooting](#troubleshooting)
7. [Recovery Procedures](#recovery-procedures)

## Prerequisites

### Required Access

- SSH access to validator node
- Read access to configuration files (`/etc/accumulate/config.toml`)
- Read access to audit logs (`/var/log/accumulate/key-rotation/`)
- Monitoring system access (Prometheus, Grafana)

### Required Tools

- `accumulated` CLI
- `jq` for parsing JSON audit logs
- `tail`, `grep` for log analysis

### Configuration Files

Key rotation configuration is in `config/accumulate.toml`:

```toml
[accumulate.key-rotation]
enabled = true
rotation-interval-days = 90
grace-period-days = 7
warning-period-days = 7

[accumulate.key-rotation.audit]
enabled = true
directory = "/var/log/accumulate/key-rotation"
retention-days = 730
```

## Normal Operations

### Checking Current Key Status

To view the current active key:

```bash
# View audit logs for recent key events
tail -n 20 /var/log/accumulate/key-rotation/audit-$(date +%Y-%m-%d).jsonl | jq .

# Check for active key
tail -n 100 /var/log/accumulate/key-rotation/audit-*.jsonl | \
  jq 'select(.event == "key_rotated" or .event == "key_generated")' | \
  tail -n 1
```

### Verifying Automatic Rotation

Automatic rotation should occur every 90 days (or as configured). Check the logs:

```bash
# Find all rotation events
grep -r '"event":"key_rotated"' /var/log/accumulate/key-rotation/ | \
  jq -r '[.timestamp, .key_id, .rotation_type] | @tsv' | \
  column -t
```

### Key Age Monitoring

Calculate how long the current key has been active:

```bash
# Get latest key activation timestamp
ACTIVATED=$(tail -n 100 /var/log/accumulate/key-rotation/audit-*.jsonl | \
  jq -r 'select(.event == "key_rotated" or .event == "key_generated") | .timestamp' | \
  tail -n 1)

echo "Key activated at: $ACTIVATED"

# Calculate age in days
python3 -c "
from datetime import datetime
activated = datetime.fromisoformat('$ACTIVATED'.replace('Z', '+00:00'))
now = datetime.now(activated.tzinfo)
age_days = (now - activated).days
print(f'Key age: {age_days} days')
print(f'Warning: Key will rotate in {90 - age_days} days')
"
```

## Manual Key Rotation

### When to Perform Manual Rotation

- Before scheduled automatic rotation (for planned maintenance)
- After personnel changes (departing team members with key access)
- As part of security compliance audit
- When upgrading validator infrastructure

### Pre-Rotation Checklist

- [ ] Verify current key is healthy
- [ ] Check validator is in sync with network
- [ ] Ensure backup systems are operational
- [ ] Notify team of planned rotation
- [ ] Review audit logs for any anomalies

### Rotation Procedure

1. **Trigger Manual Rotation**

   Manual rotation is currently triggered via the API. First, ensure the node is running:

   ```bash
   systemctl status accumulated
   ```

   Then trigger rotation (requires API endpoint):

   ```bash
   # Example API call (adjust endpoint as needed)
   curl -X POST http://localhost:8080/api/v1/key-rotation/rotate \
     -H "Content-Type: application/json" \
     -d '{
       "operator": "your-name",
       "reason": "planned rotation before maintenance"
     }'
   ```

2. **Verify Rotation Success**

   Check audit logs for rotation event:

   ```bash
   tail -n 10 /var/log/accumulate/key-rotation/audit-$(date +%Y-%m-%d).jsonl | \
     jq 'select(.event == "key_rotated")'
   ```

   Expected output:
   ```json
   {
     "timestamp": "2026-03-25T10:30:00Z",
     "event": "key_rotated",
     "key_id": "key-2026-03-25-abc123",
     "previous_key_id": "key-2025-12-25-xyz789",
     "validator_id": "acc://validator-1.acme",
     "rotation_type": "manual",
     "grace_period_end": "2026-04-01T10:30:00Z",
     "operator": "your-name",
     "reason": "planned rotation before maintenance"
   }
   ```

3. **Monitor Grace Period**

   During the 7-day grace period, both old and new keys are valid. Monitor for:

   - Consensus participation continues normally
   - No signature verification errors in logs
   - Other validators accept both keys

   ```bash
   # Monitor consensus logs
   journalctl -u accumulated -f | grep -i "signature\|consensus"
   ```

4. **Verify Old Key Expiration**

   After 7 days, verify the old key has been marked as expired:

   ```bash
   # Should see grace_ended event
   grep -r '"event":"key_grace_ended"' /var/log/accumulate/key-rotation/ | tail -n 1 | jq .
   ```

### Post-Rotation Checklist

- [ ] New key is active and signing blocks
- [ ] No consensus disruptions observed
- [ ] Grace period is active (both keys valid)
- [ ] Audit log entry created
- [ ] Monitoring alerts are clear
- [ ] Team notified of successful rotation

## Emergency Key Revocation

### When to Revoke

**Immediate revocation is required when:**

- Key compromise is confirmed or suspected
- Unauthorized access to validator system detected
- Private key leaked or exposed
- HSM/KMS security alert

### Revocation Procedure

**WARNING:** Emergency revocation bypasses the grace period. This may cause temporary consensus disruption. Coordinate with other validators when possible.

1. **Assess the Situation**

   ```bash
   # Check for suspicious activity in system logs
   journalctl -u accumulated --since "1 hour ago" | grep -i "error\|unauthorized\|failed"

   # Check for unusual network connections
   netstat -tupan | grep accumulated
   ```

2. **Trigger Emergency Revocation**

   ```bash
   # Emergency revocation API call
   curl -X POST http://localhost:8080/api/v1/key-rotation/revoke \
     -H "Content-Type: application/json" \
     -d '{
       "operator": "your-name",
       "reason": "Key compromise detected - unauthorized access"
     }'
   ```

3. **Verify Revocation**

   ```bash
   # Check for revocation event
   tail -n 20 /var/log/accumulate/key-rotation/audit-$(date +%Y-%m-%d).jsonl | \
     jq 'select(.event == "key_revoked")'
   ```

4. **Monitor New Key Activation**

   A new key should be generated and activated immediately:

   ```bash
   # Verify new active key
   tail -n 20 /var/log/accumulate/key-rotation/audit-$(date +%Y-%m-%d).jsonl | \
     jq 'select(.event == "key_rotated" and .rotation_type == "emergency")'
   ```

5. **Notify Security Team**

   - Send security incident notification
   - Document the compromise in incident tracker
   - Coordinate with other validator operators
   - Review security logs for root cause

### Post-Revocation Actions

- [ ] New key activated successfully
- [ ] Old key confirmed revoked (not valid)
- [ ] Security team notified
- [ ] Incident report filed
- [ ] Root cause analysis initiated
- [ ] Additional security measures implemented

## Monitoring and Alerts

### Key Metrics to Monitor

1. **Key Age**
   - Alert when key age > 83 days (7 days before rotation)
   - Critical when key age > 90 days (should have auto-rotated)

2. **Rotation Success Rate**
   - Track successful vs. failed rotations
   - Alert on rotation failure

3. **Grace Period Status**
   - Monitor keys in grace period
   - Alert if grace period expires without new key

4. **HSM Health** (if using HSM)
   - Monitor HSM connectivity
   - Alert on HSM errors or timeouts

5. **Audit Log Health**
   - Ensure audit logs are being written
   - Alert on write failures

### Prometheus Metrics

Key rotation exposes the following metrics:

```promql
# Key age in days
accumulate_key_rotation_age_days

# Keys in grace period
accumulate_key_rotation_grace_period_keys

# Rotation events (counter)
accumulate_key_rotation_events_total{type="automatic|manual|emergency"}

# HSM operation errors
accumulate_key_rotation_hsm_errors_total
```

### Alert Rules

Example Prometheus alert rules:

```yaml
groups:
  - name: key_rotation
    rules:
      - alert: KeyRotationDue
        expr: accumulate_key_rotation_age_days > 83
        for: 1h
        annotations:
          summary: "Key rotation due soon"
          description: "Validator key is {{ $value }} days old, rotation due in {{ sub 90 $value }} days"

      - alert: KeyRotationOverdue
        expr: accumulate_key_rotation_age_days > 90
        annotations:
          summary: "Key rotation overdue"
          description: "Validator key is {{ $value }} days old, rotation should have occurred"
          severity: critical

      - alert: HSMError
        expr: rate(accumulate_key_rotation_hsm_errors_total[5m]) > 0
        annotations:
          summary: "HSM errors detected"
          description: "HSM operations are failing"
          severity: critical
```

## Troubleshooting

### Rotation Failed

**Symptoms:** Rotation triggered but failed to complete

**Diagnosis:**

```bash
# Check logs for error messages
journalctl -u accumulated --since "10 minutes ago" | grep -i "rotation\|error"

# Check audit logs for failure events
tail -n 50 /var/log/accumulate/key-rotation/audit-*.jsonl | \
  jq 'select(.event == "rotation_failed")'
```

**Common Causes:**

1. **HSM Unavailable**
   - Check HSM connectivity
   - Verify HSM credentials
   - Review HSM logs

2. **Insufficient Permissions**
   - Check file permissions on audit directory
   - Verify process has write access

3. **Disk Full**
   - Check disk space: `df -h`
   - Clean up old logs if needed

**Resolution:**

```bash
# Retry rotation manually after fixing issue
curl -X POST http://localhost:8080/api/v1/key-rotation/rotate \
  -H "Content-Type: application/json" \
  -d '{"operator": "your-name", "reason": "retry after fixing HSM"}'
```

### Grace Period Not Working

**Symptoms:** Old key rejected immediately after rotation

**Diagnosis:**

```bash
# Check grace period configuration
grep -A 10 "\[accumulate.key-rotation\]" /etc/accumulate/config.toml

# Verify grace period end time
tail -n 20 /var/log/accumulate/key-rotation/audit-*.jsonl | \
  jq 'select(.event == "key_rotated") | .grace_period_end'
```

**Resolution:**

- Verify `grace-period-days` is set correctly (recommended: 7)
- Check system time/clock sync
- Review key metadata for correct grace end time

### Audit Logs Not Writing

**Symptoms:** No new entries in audit logs

**Diagnosis:**

```bash
# Check audit log permissions
ls -la /var/log/accumulate/key-rotation/

# Check disk space
df -h /var/log

# Verify audit is enabled
grep "audit" /etc/accumulate/config.toml
```

**Resolution:**

```bash
# Fix permissions if needed
sudo chown -R accumulated:accumulated /var/log/accumulate/key-rotation
sudo chmod 700 /var/log/accumulate/key-rotation

# Restart service
sudo systemctl restart accumulated
```

## Recovery Procedures

### Lost Key Material (HSM Failure)

**Scenario:** Total HSM failure, key material unrecoverable

**Prerequisites:**
- Emergency backup key stored in secure offline location
- Coordination with other validators

**Procedure:**

1. **Activate Emergency Backup Key**

   Temporarily configure node to use backup key:

   ```bash
   # Edit config to use backup key file
   sudo vim /etc/accumulate/config.toml

   # Restart with backup key
   sudo systemctl restart accumulated
   ```

2. **Trigger Emergency Rotation**

   Once running with backup key, immediately rotate:

   ```bash
   curl -X POST http://localhost:8080/api/v1/key-rotation/rotate \
     -H "Content-Type: application/json" \
     -d '{
       "operator": "your-name",
       "reason": "HSM failure recovery"
     }'
   ```

3. **Restore HSM Service**

   - Restore HSM from backup or provision new HSM
   - Configure new HSM connection
   - Verify HSM health

4. **Document Incident**

   - Create incident report
   - Update disaster recovery procedures
   - Review backup key security

### Corrupted Audit Logs

**Scenario:** Audit log checksum chain is broken

**Diagnosis:**

```bash
# Verify checksum chain integrity
python3 << 'EOF'
import json
import sys

previous_checksum = ""
for line in open("/var/log/accumulate/key-rotation/audit-2026-03-25.jsonl"):
    event = json.loads(line)
    if previous_checksum and event["previous_checksum"] != previous_checksum:
        print(f"CHAIN BROKEN at event {event['event']} (key {event['key_id']})")
        sys.exit(1)
    previous_checksum = event["checksum"]
print("Chain integrity verified")
EOF
```

**Recovery:**

1. Audit log corruption is a serious security event
2. Preserve corrupted logs for investigation
3. Start new audit log chain
4. Investigate root cause (disk corruption, unauthorized access, etc.)

### Clock Skew Issues

**Scenario:** Key rotation timing incorrect due to clock drift

**Diagnosis:**

```bash
# Check system time
timedatectl status

# Compare with NTP servers
ntpq -p
```

**Resolution:**

```bash
# Sync system time
sudo timedatectl set-ntp true
sudo systemctl restart systemd-timesyncd

# Verify sync
timedatectl status
```

After fixing clock:
- Review key expiration times
- Adjust rotation schedule if needed
- Consider triggering manual rotation

## Best Practices

1. **Regular Rotation Drills**
   - Practice manual rotation quarterly
   - Test emergency revocation procedures
   - Document lessons learned

2. **Monitoring**
   - Set up all recommended alerts
   - Review audit logs weekly
   - Monitor HSM health daily

3. **Security**
   - Limit access to validator systems
   - Use HSM in production
   - Keep backup keys in secure offline storage
   - Rotate backup keys annually

4. **Documentation**
   - Keep this runbook updated
   - Document all manual rotations
   - Maintain incident reports

5. **Coordination**
   - Notify other validators before manual rotation
   - Join validator operator chat channel
   - Share lessons learned

## Contact Information

- **Security Incidents:** security@accumulate.network
- **Validator Operations:** validators@accumulate.network
- **Emergency:** Use PagerDuty integration
- **Documentation:** https://docs.accumulate.network

## Revision History

| Date       | Version | Changes                  | Author |
|------------|---------|--------------------------|--------|
| 2026-03-25 | 1.0     | Initial version          | System |
