# Key Rotation Strategies in Accumulate

## Metadata
- **Document Type**: Technical Architecture
- **Version**: 1.1
- **Last Updated**: 2025-05-17
- **Related Components**: Identity Management, Security, Key Management
- **Code References**: `internal/core/execute/v2/chain/update_key.go`, `internal/core/execute/v2/chain/update_key_page.go`
- **Tags**: key_rotation, security, key_management, identity

## 1. Introduction

Key rotation is a critical security practice that involves periodically changing cryptographic keys to limit the impact of key compromise. Accumulate's keybook and key page architecture provides a flexible framework for implementing various key rotation strategies. This document outlines key rotation approaches in Accumulate, best practices, and implementation considerations.

## 2. Importance of Key Rotation

Regular key rotation provides several security benefits:

1. **Limiting Exposure**: Reduces the window of opportunity for attackers if a key is compromised
2. **Compartmentalizing Risk**: Limits the scope of potential compromise
3. **Compliance**: Meets regulatory requirements for key management
4. **Forward Secrecy**: Ensures that compromise of current keys doesn't compromise past transactions
5. **Operational Hygiene**: Forces regular review of access controls and security practices

## 3. Accumulate's Key Rotation Architecture

Accumulate's design facilitates key rotation through its hierarchical keybook and key page structure.

### 3.1 Key Rotation Fundamentals

The key components that enable rotation include:

```go
type KeyBook struct {
    Url       *url.URL
    BookType  BookType
    AccountAuth
    PageCount uint64
}

type KeyPage struct {
    Url              *url.URL
    Version          uint64
    Keys             []*KeySpec
    AcceptThreshold  uint64
    // Other fields...
}
```

Key rotation leverages:
- The ability to add new key pages to a keybook
- Version tracking for key pages
- Threshold signatures to enable gradual transition
- Authority hierarchies to manage the rotation process

### 3.2 Key Rotation Mechanisms

Accumulate supports several key rotation mechanisms:

1. **Page Addition**: Adding new key pages with new keys
2. **Key Update**: Updating keys within an existing key page
3. **Threshold Adjustment**: Modifying thresholds to change signature requirements
4. **Authority Transition**: Changing the authority structure

## 4. Key Rotation Strategies

### 4.1 Progressive Page Rotation

This strategy involves adding new key pages and gradually transitioning authority:

1. **Create New Page**: Add a new key page with new keys to the keybook
2. **Dual Authority Period**: Maintain both old and new pages as valid signers
3. **Gradual Transition**: Gradually shift operations to use the new page
4. **Deprecate Old Page**: Eventually remove or disable the old page

```go
// Example: Creating a new key page for rotation
createKeyPage := &protocol.CreateKeyPage{
    Keys: []*protocol.KeySpecParams{
        {KeyType: protocol.SignatureTypeED25519, PublicKeyHash: newKey1Hash},
        {KeyType: protocol.SignatureTypeED25519, PublicKeyHash: newKey2Hash},
        {KeyType: protocol.SignatureTypeED25519, PublicKeyHash: newKey3Hash},
    },
}
```

#### Advantages:
- Smooth transition without service disruption
- Ability to revert if issues arise with new keys
- Clear separation between old and new keys

#### Disadvantages:
- Longer exposure period during transition
- More complex management during transition
- Requires careful tracking of which page to use

### 4.2 In-Place Key Rotation

This strategy involves updating keys within an existing key page:

1. **Update Keys**: Replace existing keys with new ones
2. **Version Increment**: The key page version is incremented
3. **Immediate Transition**: All operations immediately use the new keys

```go
// Example: Updating keys in an existing page
updateKeyPage := &protocol.UpdateKeyPage{
    Operation: []protocol.KeyPageOperation{
        &protocol.UpdateKeyOperation{
            OldKeyIndex: 0,
            NewKeySpec: protocol.KeySpecParams{
                KeyType: protocol.SignatureTypeED25519,
                PublicKeyHash: newKeyHash,
            },
        },
    },
}
```

The actual implementation in the codebase is found in `internal/core/execute/v2/chain/update_key.go`:

```go
// From internal/core/execute/v2/chain/update_key.go
func updateKey(page *protocol.KeyPage, book *protocol.KeyBook, old, new *protocol.KeySpecParams, preserveDelegate bool) error {
    if new.Delegate != nil {
        if err := verifyIsNotPage(&book.AccountAuth, new.Delegate); err != nil {
            return errors.UnknownError.WithFormat("invalid delegate %v: %w", new.Delegate, err)
        }
    }

    // Find the old entry
    oldPos, entry, found := findKeyPageEntry(page, old)
    if !found {
        return fmt.Errorf("entry to be updated not found on the key page")
    }

    // Check for an existing key with same delegate
    newPos, _, found := findKeyPageEntry(page, new)
    if found && oldPos != newPos {
        return fmt.Errorf("cannot have duplicate entries on key page")
    }

    // Update the entry
    entry.PublicKeyHash = new.KeyHash

    if new.Delegate != nil || !preserveDelegate {
        entry.Delegate = new.Delegate
    }

    // Relocate the entry
    page.RemoveKeySpecAt(oldPos)
    page.AddKeySpec(entry)
    return nil
}
```

This function handles the core logic of updating a key within a key page, including validation and preserving delegation relationships when needed.

#### Advantages:
- Simpler implementation
- Immediate security improvement
- No need to track multiple active pages

#### Disadvantages:
- No fallback if new keys have issues
- Potential for service disruption
- All-or-nothing approach

### 4.3 Threshold-Based Rotation

This strategy uses threshold adjustments to facilitate rotation:

1. **Add New Keys**: Add new keys to the key page
2. **Adjust Thresholds**: Modify thresholds to require signatures from both old and new keys
3. **Transition Period**: Require signatures from both key sets
4. **Complete Rotation**: Remove old keys and adjust thresholds

```go
// Example: Adjusting thresholds during rotation
updateKeyPage := &protocol.UpdateKeyPage{
    Operation: []protocol.KeyPageOperation{
        &protocol.SetThresholdKeyPageOperation{
            AcceptThreshold: 2,  // Require 2 signatures during transition
        },
    },
}
```

The implementation in the codebase includes operations for adjusting thresholds as part of key page updates. In `internal/core/execute/v2/chain/update_key_page.go`, the `executeOperation` method handles different types of key page operations, including threshold adjustments:

```go
// From internal/core/execute/v2/chain/update_key_page.go (simplified)
func (x UpdateKeyPage) executeOperation(page *protocol.KeyPage, book *protocol.KeyBook, op protocol.KeyPageOperation) error {
    switch op := op.(type) {
    case *protocol.SetThresholdKeyPageOperation:
        // Update thresholds
        if op.AcceptThreshold > 0 {
            page.AcceptThreshold = op.AcceptThreshold
        }
        if op.RejectThreshold > 0 {
            page.RejectThreshold = op.RejectThreshold
        }
        // Additional threshold settings...
        return nil

    case *protocol.AddKeyOperation:
        // Add a new key
        // ...

    case *protocol.RemoveKeyOperation:
        // Remove a key
        // ...

    // Other operation types...
    }
    return nil
}
```

This approach allows for sophisticated key rotation strategies that leverage threshold signatures for enhanced security during transition periods.

#### Advantages:
- Enhanced security during transition
- Enforced dual control
- Gradual verification of new keys

#### Disadvantages:
- More complex signature requirements
- Potential for confusion about required signatures
- Higher operational overhead

### 4.4 Hierarchical Rotation

This strategy leverages Accumulate's authority hierarchy:

1. **Higher Authority Rotation**: Rotate keys at higher authority levels first
2. **Delegated Rotation**: Use higher authority to rotate lower-level keys
3. **Cascading Updates**: Update authority references throughout the hierarchy

```go
// Example: Using higher authority to rotate lower keys
// First, ensure the higher authority keybook is updated
// Then use it to authorize updates to lower keybooks
updateLowerKeyBook := &protocol.UpdateKeyPage{
    // Operations to update lower key page
}
// Sign with the higher authority keybook
```

#### Advantages:
- Clear authority chain
- Ability to recover from compromises at lower levels
- Structured approach to organization-wide rotation

#### Disadvantages:
- Complex coordination required
- Potential for authority conflicts
- Longer overall rotation process

## 5. Scheduled vs. Emergency Rotation

Key rotation can be either scheduled or emergency-driven:

### 5.1 Scheduled Rotation

Planned, regular key rotation:

1. **Defined Schedule**: Establish a regular rotation interval (e.g., quarterly, annually)
2. **Preparation Phase**: Generate and secure new keys before rotation
3. **Execution Phase**: Implement the rotation according to the chosen strategy
4. **Verification Phase**: Verify all systems work with the new keys

### 5.2 Emergency Rotation

Rotation in response to a security incident:

1. **Immediate Response**: Quickly generate new keys
2. **Containment**: Limit the use of potentially compromised keys
3. **Accelerated Rotation**: Implement rotation with abbreviated transition
4. **Forensic Analysis**: Investigate the extent of compromise

## 6. Implementation Considerations

### 6.1 Key Generation

Secure key generation is critical for effective rotation:

1. **Entropy Source**: Use high-quality entropy sources
2. **Isolation**: Generate keys in isolated, secure environments
3. **Hardware Security**: Consider hardware security modules (HSMs) for key generation
4. **Key Ceremony**: Implement formal key ceremonies for high-value keys

### 6.2 Backup and Recovery

Ensure robust backup procedures:

1. **Key Backup**: Securely back up new keys before rotation
2. **Recovery Testing**: Test recovery procedures before relying on new keys
3. **Secure Storage**: Store backups in secure, distributed locations
4. **Access Controls**: Implement strict access controls for key backups

### 6.3 Automation and Tooling

Consider automation to improve reliability:

1. **Rotation Scripts**: Develop scripts for common rotation tasks
2. **Verification Tools**: Create tools to verify rotation success
3. **Monitoring**: Implement monitoring for rotation-related issues
4. **Rollback Automation**: Prepare automated rollback procedures

## 7. Key Rotation Patterns for Different Use Cases

### 7.1 Individual ADI Rotation

For personal or small-entity ADIs:

1. **Simple Page Addition**: Add a new key page with new keys
2. **Self-Authorization**: Use existing keys to authorize the addition
3. **Manual Transition**: Manually transition to using the new page
4. **Optional Cleanup**: Optionally remove the old page when convenient

### 7.2 Organizational ADI Rotation

For large organizations with complex structures:

1. **Phased Approach**: Rotate keys in phases, starting with less critical systems
2. **Hierarchical Coordination**: Coordinate rotation across the authority hierarchy
3. **Role-Based Rotation**: Rotate keys based on roles and responsibilities
4. **Documentation**: Maintain detailed documentation of the rotation process

### 7.3 Token Issuer Rotation

For token issuers with additional security requirements:

1. **Heightened Security**: Implement more frequent rotation for issuer keys
2. **Dual Control**: Require multiple authorities to approve rotation
3. **Public Notification**: Notify token holders of rotation events
4. **Transparency**: Maintain transparency about rotation practices

### 7.4 Validator Key Rotation

For network validators:

1. **Coordinated Rotation**: Coordinate with other validators
2. **Announcement Period**: Announce rotation plans in advance
3. **Gradual Transition**: Implement rotation during low-activity periods
4. **Performance Monitoring**: Monitor network performance during rotation

## 8. Best Practices for Key Rotation

### 8.1 Planning and Preparation

1. **Rotation Policy**: Establish a formal key rotation policy
2. **Risk Assessment**: Conduct risk assessment before each rotation
3. **Testing**: Test rotation procedures in a staging environment
4. **Stakeholder Communication**: Inform all stakeholders about rotation plans

### 8.2 Execution

1. **Minimal Disruption**: Schedule rotation during low-activity periods
2. **Monitoring**: Actively monitor systems during rotation
3. **Incremental Approach**: Rotate one component at a time when possible
4. **Verification**: Verify each step before proceeding

### 8.3 Post-Rotation

1. **Validation**: Validate all systems work with new keys
2. **Documentation Update**: Update documentation to reflect new key state
3. **Secure Disposal**: Securely dispose of old key material when appropriate
4. **Lessons Learned**: Document lessons learned for future rotations

## 9. Common Challenges and Solutions

### 9.1 Coordination Challenges

**Challenge**: Coordinating rotation across multiple systems and stakeholders.

**Solutions**:
- Develop a detailed rotation playbook
- Assign a rotation coordinator
- Use collaborative tools to track progress
- Implement clear communication channels

### 9.2 Technical Challenges

**Challenge**: Ensuring all systems recognize and accept new keys.

**Solutions**:
- Comprehensive inventory of key dependencies
- Thorough testing before production rotation
- Monitoring systems during rotation
- Prepared rollback procedures

### 9.3 Security Challenges

**Challenge**: Maintaining security during the rotation process.

**Solutions**:
- Implement additional monitoring during rotation
- Use secure channels for key distribution
- Apply principle of least privilege to rotation activities
- Consider timing attacks and other side-channel risks

## 10. Conclusion

Effective key rotation is essential for maintaining the security of Accumulate-based systems over time. Accumulate's keybook and key page architecture provides flexible, powerful mechanisms for implementing various rotation strategies tailored to different security requirements and operational constraints.

By understanding the available rotation strategies and following best practices, organizations can enhance their security posture, meet compliance requirements, and minimize the risk of key compromise affecting their operations.

## Related Documents

- [Keybooks and Signatures](./01_keybooks_and_signatures.md)
- [Advanced Signature Validation](./02_advanced_signature_validation.md)
- [Delegation Patterns](./04_delegation_patterns.md)
- [Security Best Practices](../../07_operations/03_security_best_practices.md)
- [Transaction Validation Process](../../04_network/05_consensus/05_02_validation_process.md)
- [Identity Management](../04_identity_management.md)
- [Signature System Comparison](./05_signature_system_comparison.md)
- [Transaction Processing](../../06_implementation/03_transaction_processing.md)
