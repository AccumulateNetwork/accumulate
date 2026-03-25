# Automated Key Rotation for Accumulate Validators

## Overview

This document describes the automated key rotation mechanism for Accumulate validator nodes. Key rotation is a critical security practice for production deployments with 20+ validators, ensuring that compromised keys have a limited window of usefulness and reducing the impact of potential security breaches.

## Security Model

### Threat Model

The key rotation system addresses the following security concerns:

1. **Long-lived Key Compromise**: Keys that never rotate provide attackers with unlimited time to extract or compromise them
2. **Insider Threats**: Automated rotation limits the damage from insider access to signing keys
3. **Key Material Aging**: Cryptographic best practices recommend periodic key rotation (90-180 days)
4. **Emergency Revocation**: Rapid response capability when key compromise is detected

### Security Properties

- **Forward Secrecy**: Compromise of current key does not reveal past signatures
- **Grace Period**: 7-day overlap ensures no consensus disruption during rotation
- **Audit Trail**: Complete audit logging of all key operations with tamper-evident logs
- **HSM/KMS Integration**: Support for hardware security modules and key management services

## Architecture

### Components

```
┌─────────────────────────────────────────────────────────────┐
│                    Validator Node                            │
│                                                               │
│  ┌──────────────────┐         ┌──────────────────┐          │
│  │  Key Rotation    │────────▶│  Signer          │          │
│  │  Manager         │         │  (ADISigner/     │          │
│  │                  │         │   RawKeySigner)  │          │
│  └──────────────────┘         └──────────────────┘          │
│          │                             │                     │
│          │                             │                     │
│          ▼                             ▼                     │
│  ┌──────────────────┐         ┌──────────────────┐          │
│  │  Key Metadata    │         │  Consensus       │          │
│  │  Store           │         │  Engine          │          │
│  └──────────────────┘         └──────────────────┘          │
│          │                                                   │
│          ▼                                                   │
│  ┌──────────────────┐                                       │
│  │  Audit Logger    │                                       │
│  └──────────────────┘                                       │
└───────────────────────────────────────────────────────────┬─┘
                                                             │
                     ┌───────────────────────────────────────┘
                     │
                     ▼
         ┌────────────────────────┐
         │  HSM / KMS Provider    │
         │  - AWS CloudHSM        │
         │  - Google Cloud KMS    │
         │  - HashiCorp Vault     │
         └────────────────────────┘
```

### Key Metadata Store

Each key maintains the following metadata:

```go
type KeyMetadata struct {
    KeyID          string    // Unique identifier for the key
    PublicKey      []byte    // Ed25519 public key
    CreatedAt      time.Time // When the key was generated
    ActivatedAt    time.Time // When the key became active for signing
    ExpiresAt      time.Time // When the key should be rotated
    RevokedAt      *time.Time // Set if key is emergency-revoked
    Status         KeyStatus  // Active, Grace, Expired, Revoked
    PreviousKeyID  string    // Link to previous key (for audit trail)
    NextKeyID      string    // Link to next key (during grace period)
}

type KeyStatus int
const (
    KeyStatusPending  KeyStatus = iota  // Generated but not yet active
    KeyStatusActive                     // Current signing key
    KeyStatusGrace                      // Old key in grace period
    KeyStatusExpired                    // Past grace period, no longer valid
    KeyStatusRevoked                    // Emergency revocation
)
```

### Rotation Workflow

#### Normal Rotation (Automated)

```
Day 0                  Day 83                 Day 90                  Day 97
  │                      │                      │                       │
  │  Key A Active       │  Generate Key B      │  Key B Active         │  Key A Expired
  │                     │                      │  Key A Grace Period   │
  │◄─────────────────────►│◄──────────────────►│◄────────────────────►│
  │  90 days            │  7 days warning      │  7 days grace         │
  │                     │                      │                       │
  └─────────────────────┴──────────────────────┴───────────────────────┴───────▶
```

1. **Day 0**: Key A is generated and activated
2. **Day 83**: Warning logged, Key B generation scheduled
3. **Day 90**: Key B activated, Key A enters grace period
4. **Day 97**: Key A expired and archived, only Key B valid

#### Grace Period Behavior

During the 7-day grace period:
- **New signatures**: Always use new key (Key B)
- **Verification**: Accept signatures from both old (Key A) and new (Key B) keys
- **Consensus messages**: Use new key for headers, votes, certificates
- **Historical verification**: Old signatures remain valid indefinitely

This ensures:
- No consensus disruption if some validators are delayed in rotation
- Gradual transition across the validator set
- Tolerance for clock skew and network partitions

### Emergency Revocation

When a key compromise is detected:

1. **Immediate Actions** (automated or operator-triggered):
   ```
   rotationManager.RevokeKey(keyID, reason)
   ```
   - Key marked as REVOKED immediately
   - All pending operations using this key rejected
   - New key generated and activated

2. **Notification**:
   - Audit log entry created
   - Alert sent to monitoring system
   - Operator notification (email, PagerDuty, etc.)

3. **Grace Period Bypassed**:
   - Normal 7-day grace period skipped
   - Immediate transition to new key
   - Risk: Potential temporary consensus disruption
   - Mitigation: Coordinate with other validators

## HSM/KMS Integration

### Supported Providers

#### AWS CloudHSM

```toml
[accumulate.key-rotation.hsm]
provider = "aws-cloudhsm"
cluster-id = "cluster-abc123xyz"
region = "us-east-1"
key-label = "accumulate-validator-key"
```

Features:
- FIPS 140-2 Level 3 certified
- Key material never leaves HSM
- Automatic backup and recovery
- High availability with cluster

#### Google Cloud KMS

```toml
[accumulate.key-rotation.hsm]
provider = "google-cloud-kms"
project-id = "my-project"
location = "us-east1"
key-ring = "accumulate-validators"
key-name = "validator-signing-key"
```

Features:
- FIPS 140-2 Level 3 certified
- Automatic key rotation scheduling
- IAM-based access control
- Audit logging integrated with Cloud Logging

#### HashiCorp Vault

```toml
[accumulate.key-rotation.hsm]
provider = "vault"
address = "https://vault.example.com:8200"
token-file = "/etc/accumulate/vault-token"
mount-path = "transit"
key-name = "validator-signing-key"
```

Features:
- Software-based (not hardware HSM)
- Flexible deployment options
- Good for development/testing
- Transit secrets engine for signing

### Provider Interface

```go
type HSMProvider interface {
    // GenerateKey creates a new signing key in the HSM
    GenerateKey(ctx context.Context, keyID string) (publicKey []byte, error)

    // Sign signs data using the specified key
    Sign(ctx context.Context, keyID string, data []byte) (signature []byte, error)

    // GetPublicKey retrieves the public key for verification
    GetPublicKey(ctx context.Context, keyID string) ([]byte, error)

    // RevokeKey marks a key as revoked in the HSM
    RevokeKey(ctx context.Context, keyID string) error

    // ListKeys returns all keys managed by this HSM
    ListKeys(ctx context.Context) ([]string, error)
}
```

## Configuration

### Configuration File

Add to `config/accumulate.toml`:

```toml
[accumulate.key-rotation]
# Enable automatic key rotation
enabled = true

# Rotation interval in days (90-180 recommended)
rotation-interval-days = 90

# Grace period in days (7 recommended)
grace-period-days = 7

# Warning period before rotation (days before rotation to generate new key)
warning-period-days = 7

# Audit log configuration
[accumulate.key-rotation.audit]
enabled = true
directory = "/var/log/accumulate/key-rotation"
retention-days = 730  # 2 years

# HSM/KMS provider configuration (optional)
[accumulate.key-rotation.hsm]
# Provider: "aws-cloudhsm", "google-cloud-kms", "vault", or "none" (raw keys)
provider = "none"

# AWS CloudHSM specific
# cluster-id = "cluster-abc123xyz"
# region = "us-east-1"
# key-label = "accumulate-validator-key"

# Google Cloud KMS specific
# project-id = "my-project"
# location = "us-east1"
# key-ring = "accumulate-validators"
# key-name = "validator-signing-key"

# HashiCorp Vault specific
# address = "https://vault.example.com:8200"
# token-file = "/etc/accumulate/vault-token"
# mount-path = "transit"
# key-name = "validator-signing-key"

# Emergency contacts for revocation alerts
[accumulate.key-rotation.alerts]
email = ["ops@example.com", "security@example.com"]
pagerduty-integration-key = "your-key-here"
slack-webhook = "https://hooks.slack.com/services/..."
```

### Environment Variables

For sensitive configuration:

```bash
# AWS credentials for CloudHSM
export AWS_ACCESS_KEY_ID="..."
export AWS_SECRET_ACCESS_KEY="..."

# Google Cloud credentials
export GOOGLE_APPLICATION_CREDENTIALS="/path/to/credentials.json"

# Vault token (alternative to token-file)
export VAULT_TOKEN="..."
```

## Audit Logging

### Log Format

All key operations are logged in structured JSON format:

```json
{
  "timestamp": "2026-03-25T10:30:00Z",
  "event": "key_rotated",
  "key_id": "key-2026-03-25-abc123",
  "previous_key_id": "key-2025-12-25-xyz789",
  "validator_id": "acc://validator-1.acme",
  "rotation_type": "automatic",
  "grace_period_end": "2026-04-01T10:30:00Z",
  "operator": "system",
  "checksum": "sha256:abcdef..."
}
```

### Events Logged

- `key_generated`: New key created
- `key_activated`: Key becomes active for signing
- `key_grace_started`: Old key enters grace period
- `key_expired`: Key past grace period
- `key_revoked`: Emergency revocation
- `sign_operation`: Each signing operation (optional, high volume)
- `hsm_error`: HSM/KMS operation failures

### Log Retention

- Default: 730 days (2 years)
- Configurable via `audit.retention-days`
- Archived logs compressed and stored securely
- Tamper-evident with checksum chain

## Operational Procedures

See [Operator Runbook](operator-runbook-key-rotation.md) for detailed procedures.

### Monitoring

Key metrics to monitor:

1. **Key Age**: Alert when approaching rotation interval
2. **Grace Period Status**: Track keys in grace period
3. **HSM Health**: Monitor HSM connectivity and errors
4. **Rotation Success Rate**: Track successful vs failed rotations
5. **Signature Failures**: Monitor signing operation failures

### Alerts

Configure alerts for:

- Key rotation failures
- HSM/KMS unavailability
- Emergency revocations
- Grace period expiration without rotation
- Audit log write failures

## Security Considerations

### Best Practices

1. **HSM Usage**: Always use HSM/KMS in production
2. **Backup Keys**: Maintain encrypted backups of key metadata (not private keys)
3. **Access Control**: Restrict access to key rotation configuration
4. **Monitoring**: 24/7 monitoring of key rotation events
5. **Testing**: Regular rotation drills in staging environment
6. **Documentation**: Keep operator runbook up to date

### Key Material Handling

- **Generation**: Keys generated within HSM (never exported)
- **Storage**: Private keys stored only in HSM
- **Transport**: Never transmit private keys over network
- **Backup**: Use HSM native backup mechanisms
- **Destruction**: Secure deletion after grace period + retention

### Disaster Recovery

In case of total HSM failure:

1. Failover to backup HSM (if clustered)
2. Use emergency backup key (stored in secure offline location)
3. Initiate emergency key rotation across all validators
4. Restore from HSM backup once system is recovered

## Implementation Notes

### Integration with ADISigner

The key rotation system integrates seamlessly with `ADISigner`:

```go
// During rotation, the ADISigner is updated with the new key
signer := types.NewADISigner(keyPageURL, newPrivateKey)

// The validator ID remains constant (ADI URL)
// Only the signing key changes on the key page
```

### Consensus Impact

- **No protocol changes required**: Rotation is transparent to consensus
- **Signature verification**: Updated to check both active and grace period keys
- **Certificate validation**: Enhanced to track key validity periods

### Performance

- **Rotation overhead**: Minimal, occurs once per 90-180 days
- **Grace period overhead**: Small increase in verification time (check 2 keys instead of 1)
- **HSM latency**: 1-5ms additional latency per signing operation
- **Audit logging**: Asynchronous, no impact on consensus performance

## Future Enhancements

1. **Multi-signature support**: Threshold signatures with key sharding
2. **Quantum-resistant algorithms**: Post-quantum cryptography integration
3. **Automated key health checks**: Periodic verification of key material
4. **Cross-validator coordination**: Coordinated rotation across validator set
5. **Key escrow**: Optional key escrow for regulatory compliance

## References

- [NIST SP 800-57: Recommendation for Key Management](https://csrc.nist.gov/publications/detail/sp/800-57-part-1/rev-5/final)
- [FIPS 140-2: Security Requirements for Cryptographic Modules](https://csrc.nist.gov/publications/detail/fips/140/2/final)
- [AWS CloudHSM Documentation](https://docs.aws.amazon.com/cloudhsm/)
- [Google Cloud KMS Documentation](https://cloud.google.com/kms/docs)
- [HashiCorp Vault Transit Secrets Engine](https://www.vaultproject.io/docs/secrets/transit)
