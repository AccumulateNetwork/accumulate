# Delegation Patterns in Accumulate

## Metadata
- **Document Type**: Technical Architecture
- **Version**: 1.1
- **Last Updated**: 2025-05-17
- **Related Components**: Identity Management, Security, Key Management
- **Code References**: `protocol/keypage.go`, `internal/core/execute/v2/chain/update_key.go`
- **Tags**: delegation, key_management, identity, authorization

## 1. Introduction

Delegation is a powerful feature in Accumulate that allows one entity to authorize another to act on its behalf. This capability enables sophisticated access control patterns, operational flexibility, and separation of concerns. This document explores delegation patterns in Accumulate, their implementation, and common use cases.

## 2. Delegation Fundamentals

Delegation in Accumulate is built on the principle that authority can be explicitly transferred or shared between entities while maintaining security and accountability.

### 2.1 Delegation Mechanisms

Accumulate supports delegation through several mechanisms:

```go
// Key specification with delegation
type KeySpec struct {
    PublicKey []byte
    Delegate  *url.URL
}

// Delegated signature
type DelegatedSignature struct {
    Signature Signature
    Delegator *url.URL
}
```

Key delegation components:
- **Key Delegation**: A key can delegate to another entity (ADI or key page)
- **Signature Delegation**: A signature can indicate it's made on behalf of another entity
- **Authority Delegation**: An account can delegate authority to another account

### 2.2 Delegation vs. Direct Authority

Delegation differs from direct authority in several important ways:

| Aspect | Delegation | Direct Authority |
|--------|------------|------------------|
| Revocability | Can be revoked by the delegator | Permanent until key/authority change |
| Visibility | Explicitly shows delegation chain | No visible delegation |
| Granularity | Can be fine-grained | Usually broader authority |
| Accountability | Tracks both delegator and delegate | Only tracks direct signer |

## 3. Key-Level Delegation Patterns

### 3.1 Direct Key Delegation

The simplest form of delegation occurs at the key level:

```go
// Example: Key with delegation
keySpec := &protocol.KeySpec{
    PublicKey: publicKey,
    Delegate: delegateUrl,  // URL of the delegate entity
}
```

In this pattern:
1. A key in a key page specifies a delegate
2. The delegate can sign on behalf of this key
3. The signature is validated against the delegate's key

#### Implementation Considerations:
- The delegate must be an ADI or key page
- Delegation is explicit and visible in the key specification
- Revocation requires updating the key specification

The implementation in the codebase is found in `protocol/keypage.go`, where the `KeySpec` structure includes a delegate field:

```go
// From protocol/keypage.go
type KeySpec struct {
    fieldsSet     []bool
    PublicKeyHash []byte   `json:"publicKeyHash,omitempty" form:"publicKeyHash" query:"publicKeyHash" validate:"required"`
    Delegate      *url.URL `json:"delegate,omitempty" form:"delegate" query:"delegate"`
    LastUsedOn    uint64   `json:"lastUsedOn,omitempty" form:"lastUsedOn" query:"lastUsedOn" validate:"required"`
    extraData     []byte
}
```

When a key is used to sign a transaction, the system checks for delegation relationships and validates that the delegate has the authority to sign on behalf of the delegator.

### 3.2 Hierarchical Delegation

This pattern establishes a delegation hierarchy:

1. **Multi-Level Structure**: Delegations can form a chain (A delegates to B, B delegates to C)
2. **Authority Flow**: Authority flows down the chain with potential restrictions at each level
3. **Validation Path**: The system validates the entire delegation path

```go
// Example: Hierarchical delegation
keyPage1 := &protocol.KeyPage{
    Keys: []*protocol.KeySpec{
        {
            PublicKeyHash: keyHash1,
            Delegate: keyPage2Url,
        },
    },
}

keyPage2 := &protocol.KeyPage{
    Keys: []*protocol.KeySpec{
        {
            PublicKeyHash: keyHash2,
            Delegate: keyPage3Url,
        },
    },
}
```

The implementation in the codebase includes validation for delegation chains. In `internal/core/execute/v2/chain/update_key.go`, the system verifies delegation relationships when updating keys:

```go
// From internal/core/execute/v2/chain/update_key.go
func verifyIsNotPage(auth *protocol.AccountAuth, delegate *url.URL) error {
    // Get the account
    account, err := auth.GetAccount(delegate)
    if err != nil {
        return errors.UnknownError.Wrap(err)
    }

    // Verify it's not a key page
    if _, ok := account.(*protocol.KeyPage); ok {
        return errors.BadRequest.WithFormat("cannot use a key page as a delegate")
    }
    return nil
}
```

This function ensures that delegation relationships are valid and prevents circular references in delegation chains.

## 4. Account-Level Delegation Patterns

### 4.1 Authority Delegation

Delegation at the account authority level:

```go
// Example: Account with delegated authority
account := &protocol.Account{
    Url: accountUrl,
    AccountAuth: protocol.AccountAuth{
        Authorities: []protocol.AuthorityEntry{
            {Url: primaryAuthority},
            {Url: delegatedAuthority},
        },
    },
}
```

In this pattern:
1. An account specifies multiple authorities
2. Secondary authorities act as delegates
3. Any authority can authorize actions on the account
4. The authority hierarchy determines priority

#### Implementation Considerations:
- Provides account-level delegation rather than key-level
- Enables organizational hierarchy representation
- Supports separation between operational and administrative control

## 5. Temporal Delegation Patterns

### 5.1 Time-Limited Delegation

Implementing delegation with time constraints:

```go
// Example: Time-limited delegation through a custom key page
timeLimitedKeyPage := &protocol.KeyPage{
    Keys: []*protocol.KeySpec{
        {PublicKey: delegateKey},
    },
    // Custom metadata or transaction blacklist to enforce time limits
}

// Validate time constraints during signature validation
func validateTimeLimitedDelegation(signature Signature, context ValidationContext) error {
    // Check if current time is within delegation period
    if currentTime < delegationStart || currentTime > delegationEnd {
        return errors.New("delegation period expired")
    }
    return nil
}
```

In this pattern:
1. Delegation is granted for a specific time period
2. Validation checks the current time against the delegation period
3. Delegation automatically expires after the end time

#### Implementation Considerations:
- May require custom validation logic
- Enables temporary access grants
- Reduces need for explicit revocation

## 6. Functional Delegation Patterns

### 6.1 Transaction-Type Delegation

Delegation limited to specific transaction types:

```go
// Example: Transaction-type limited delegation through blacklist
limitedKeyPage := &protocol.KeyPage{
    Keys: []*protocol.KeySpec{
        {PublicKey: delegateKey, Delegate: delegateUrl},
    },
    TransactionBlacklist: &protocol.AllowedTransactions{
        // Only allow specific transaction types
        Allowed: []protocol.TransactionType{
            protocol.TransactionTypeTransferTokens,
            protocol.TransactionTypeWriteData,
        },
    },
}
```

In this pattern:
1. Delegation is granted only for specific transaction types
2. The key page specifies allowed or disallowed transaction types
3. Validation checks the transaction type against the restrictions

#### Implementation Considerations:
- Uses built-in transaction filtering
- Enables precise functional delegation
- Provides clear boundaries for delegate authority

#### Implementation Considerations:
- Requires custom validation logic
- Enables financial control limits
- Suitable for treasury and financial operations

### 6.3 Target-Limited Delegation

Delegation limited to specific target accounts:

```go
// Example: Target-limited delegation through custom validation
// Implement custom validation logic
func validateTargetLimitedDelegation(tx *protocol.Transaction, signature Signature) error {
    // Extract the target account from the transaction
    target := extractTargetAccount(tx)
    
    // Check if target is in the allowed list
    if !isAllowedTarget(target, allowedTargets) {
        return errors.New("target account not allowed for delegation")
    }
    return nil
}
```

In this pattern:
1. Delegation is granted only for specific target accounts
2. Custom validation logic checks transaction targets
3. Transactions to unauthorized targets are rejected

#### Implementation Considerations:
- Requires custom validation logic
- Enables precise control over delegation scope
- Suitable for controlled interaction with specific partners

## 7. Use Cases for Delegation

### 7.1 Enterprise Key Management

**Scenario**: Large enterprise with complex organizational structure.

**Delegation Pattern**:
- C-level executives hold manager keybook authority
- Department heads have delegated authority for their domains
- Operational staff have limited delegated authority for daily tasks

**Benefits**:
- Clear authority hierarchy matching organizational structure
- Separation between strategic and operational control
- Simplified key management for large organizations

### 7.2 Temporary Access Grants

**Scenario**: Temporary contractor needs limited system access.

**Delegation Pattern**:
- Time-limited delegation to contractor's ADI
- Transaction-type limited to specific functions
- Usage tracking for audit purposes

**Benefits**:
- Automatic expiration without manual revocation
- Precise control over contractor capabilities
- Clear audit trail of all contractor actions

### 7.3 Multi-Signature Treasury Management

**Scenario**: Corporate treasury requiring multiple approvals.

**Delegation Pattern**:
- Threshold delegation requiring multiple signatures
- Amount-limited delegation based on transaction value
- Hierarchical delegation for different approval levels

**Benefits**:
- Enforces financial controls and separation of duties
- Enables flexible approval workflows
- Provides strong security for high-value transactions

### 7.4 Regulatory Compliance

**Scenario**: Regulated entity with compliance requirements.

**Delegation Pattern**:
- Compliance officer has oversight delegation
- Transaction-type delegation for different regulatory domains
- Multi-tier delegation matching regulatory hierarchy

**Benefits**:
- Enforces regulatory oversight
- Maintains operational efficiency
- Provides clear evidence of compliance controls

### 7.5 Distributed Applications (dApps)

**Scenario**: dApp interacting with user accounts.

**Delegation Pattern**:
- Users delegate limited authority to dApp
- Function-specific delegation for app features
- Usage-limited delegation for trial or freemium models

**Benefits**:
- Users maintain control over their accounts
- dApps can operate without holding user keys
- Clear boundaries for application authority

## 8. Implementation Best Practices

### 8.1 Delegation Design Principles

1. **Least Privilege**: Delegate the minimum authority necessary
2. **Explicit Intent**: Make delegation relationships clear and explicit
3. **Auditability**: Ensure delegation creates a clear audit trail
4. **Revocability**: Design delegation to be easily revocable
5. **Time Bounding**: Consider adding time constraints to delegation

### 8.2 Security Considerations

1. **Delegation Chains**: Be cautious with long delegation chains
2. **Validation Depth**: Ensure validation checks the entire delegation chain
3. **Revocation Mechanisms**: Implement clear, reliable revocation
4. **Monitoring**: Monitor delegation usage for unusual patterns
5. **Transparency**: Make delegation visible to all relevant stakeholders

### 8.3 Performance Considerations

1. **Validation Complexity**: Complex delegation can increase validation time
2. **Caching**: Consider caching delegation relationships
3. **Chain Length**: Limit delegation chain length for performance
4. **Batch Validation**: Optimize validation for multiple delegated signatures

## 9. Future Delegation Enhancements

Potential future enhancements to Accumulate's delegation system:

1. **Conditional Delegation**: Delegation based on complex conditions
2. **Delegation Templates**: Standardized delegation patterns for common use cases
3. **Delegation Marketplace**: Secure exchange of delegation relationships
4. **Cross-Chain Delegation**: Delegation across different blockchain networks
5. **AI-Assisted Delegation**: Intelligent suggestion and monitoring of delegation

## 10. Conclusion

Delegation in Accumulate provides a powerful mechanism for implementing sophisticated access control, operational workflows, and organizational structures. By understanding the available delegation patterns and their implementation considerations, developers can create secure, flexible systems that accurately reflect real-world authority relationships.

The combination of key-level, account-level, temporal, and functional delegation patterns enables a wide range of use cases, from enterprise key management to dApp integration. By following best practices and considering security implications, organizations can leverage delegation to enhance both security and operational efficiency.

## Related Documents

- [Keybooks and Signatures](./01_keybooks_and_signatures.md)
- [Advanced Signature Validation](./02_advanced_signature_validation.md)
- [Key Rotation Strategies](./03_key_rotation_strategies.md)
- [Identity Management](../04_identity_management.md)
- [Transaction Validation Process](../../04_network/05_consensus/05_02_validation_process.md)
- [Security Best Practices](../../07_operations/03_security_best_practices.md)
- [Signature System Comparison](./05_signature_system_comparison.md)
- [Transaction Processing](../../06_implementation/03_transaction_processing.md)
