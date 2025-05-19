# Keybooks and Signatures in Accumulate

## Metadata
- **Document Type**: Technical Architecture
- **Version**: 1.1
- **Last Updated**: 2025-05-17
- **Related Components**: Identity Management, Transaction Processing, Security
- **Code References**: `protocol/signature.go`, `internal/core/execute/v2/block/sig_user.go`
- **Tags**: keybooks, signatures, identity, security, validation

## 1. Introduction

Accumulate's security model is built around a sophisticated signature validation system that uses keybooks and key pages to manage identity and authorize transactions. This document details how signatures work in Accumulate, focusing on the structure and function of keybooks, key pages, and the signature validation process.

## 2. Signature Types

Accumulate supports multiple signature types to accommodate various use cases and security requirements.

### 2.1 Supported Signature Types

```go
// SignatureType represents the type of a signature.
type SignatureType uint64

const (
    SignatureTypeUnknown SignatureType = 0
    SignatureTypeLegacyED25519 SignatureType = 1
    SignatureTypeED25519 SignatureType = 2
    SignatureTypeRCD1 SignatureType = 3
    SignatureTypeReceipt SignatureType = 4
    SignatureTypePartition SignatureType = 5
    SignatureTypeSet SignatureType = 6
    SignatureTypeRemote SignatureType = 7
    SignatureTypeBTC SignatureType = 8
    SignatureTypeBTCLegacy SignatureType = 9
    SignatureTypeETH SignatureType = 10
    SignatureTypeDelegated SignatureType = 11
    SignatureTypeInternal SignatureType = 12
    SignatureTypeAuthority SignatureType = 13
    SignatureTypeRsaSha256 SignatureType = 14
    SignatureTypeEcdsaSha256 SignatureType = 15
    SignatureTypeTypedData SignatureType = 16
)
```

The most commonly used signature types include:

- **ED25519**: The primary signature type used in Accumulate, based on the Edwards-curve Digital Signature Algorithm
- **Authority**: Used for authority signatures that authorize operations on behalf of an ADI
- **Remote**: Used when a signature is provided by a remote entity
- **Delegated**: Used when signature authority is delegated to another entity
- **Set**: Used for multi-signature scenarios where multiple signatures are required

### 2.2 Signature Structure

All signatures in Accumulate implement the `Signature` interface:

```go
// Signature is an interface implemented by all signature types
type Signature interface {
    Type() SignatureType
    GetSignerVersion() uint64
    GetTimestamp() uint64
    GetPublicKey() []byte
    GetSignature() []byte
    GetSigner() *url.URL
    GetTransactionHash() [32]byte
    Hash() [32]byte
}
```

Each signature contains:
- A type identifier
- The signer's version
- A timestamp
- The public key used for verification
- The signature data itself
- The signer's URL
- The transaction hash being signed

## 3. Keybooks and Key Pages

Accumulate uses a hierarchical key management system consisting of keybooks and key pages. This structure provides flexibility, security, and the ability to update keys without disrupting operations.

### 3.1 Keybook Structure

A keybook is a container for key pages and serves as the root of identity management for an ADI.

```go
type KeyBook struct {
    Url       *url.URL
    BookType  BookType
    AccountAuth
    PageCount uint64
}
```

Key characteristics of a keybook:
- Each keybook has a unique URL within the Accumulate network
- Keybooks can have different types (e.g., normal, lite)
- Keybooks contain multiple key pages, each with its own set of keys
- The `PageCount` tracks the number of key pages in the keybook

### 3.2 Key Page Structure

A key page contains a set of keys and rules for how those keys can be used to authorize transactions.

```go
type KeyPage struct {
    Url              *url.URL
    CreditBalance    uint64
    AcceptThreshold  uint64
    RejectThreshold  uint64
    ResponseThreshold uint64
    BlockThreshold   uint64
    Version          uint64
    Keys             []*KeySpec
    TransactionBlacklist *AllowedTransactions
}
```

Key characteristics of a key page:
- Each key page has a unique URL derived from its parent keybook
- Key pages contain multiple keys, each with its own specification
- Thresholds define how many signatures are required for different operations:
  - `AcceptThreshold`: Signatures needed to accept a transaction
  - `RejectThreshold`: Signatures needed to reject a transaction
  - `ResponseThreshold`: Signatures needed for a response
  - `BlockThreshold`: Signatures needed to block a transaction
- Key pages have a version number that increments with updates
- A credit balance for paying transaction fees
- Optional transaction blacklist to restrict certain transaction types

### 3.3 Key Specification

Each key in a key page is defined by a `KeySpec`:

```go
type KeySpec struct {
    PublicKey []byte
    Delegate  *url.URL
}
```

A key specification includes:
- The public key used for signature verification
- An optional delegate URL, which allows another entity to sign on behalf of this key

## 4. Keybook Hierarchy and Authority

Keybooks form a hierarchy of authority within an ADI, allowing for sophisticated access control and delegation.

### 4.1 Page Hierarchy

Key pages within a keybook are ordered, with page 0 typically having the highest authority. This hierarchy enables:

1. **Key Rotation**: New keys can be added to a new page while maintaining the old ones
2. **Privilege Separation**: Different pages can have different authorities
3. **Recovery Mechanisms**: Higher-authority pages can recover or modify lower-authority pages

### 4.2 Authority Delegation

Keybooks can delegate authority to other keybooks or specific key pages:

```go
type AccountAuth struct {
    Authorities []AuthorityEntry
}

type AuthorityEntry struct {
    Url *url.URL
}
```

This delegation system allows:
- Multiple levels of authority within an organization
- Separation of operational and administrative keys
- Temporary delegation of signing authority

## 5. Signature Validation Process

When a transaction is submitted to the Accumulate network, its signatures are validated through a multi-step process. This process is implemented in the `internal/core/execute/v2/block` package, particularly in the signature validation code.

### 5.1 Basic Signature Validation

For each signature, the system:

1. Verifies the signature cryptographically using the provided public key
2. Checks that the signature timestamp is within acceptable bounds
3. Validates that the signature type is appropriate for the transaction
4. Ensures the signer has authority to sign for the principal account

```go
// From internal/core/execute/v1/block/signature.go
// validateKeySignature verifies that the signature matches the signer state.
func validateKeySignature(transaction *protocol.Transaction, signer protocol.Signer, signature protocol.KeySignature) (protocol.KeyEntry, error) {
    // Check the version
    if transaction.Body.Type().IsUser() && signature.GetSignerVersion() != signer.GetVersion() {
        return nil, errors.BadSignerVersion.WithFormat("invalid version: have %d, got %d", signer.GetVersion(), signature.GetSignerVersion())
    }

    // Find the key entry
    _, entry, ok := signer.EntryByKeyHash(signature.GetPublicKeyHash())
    if !ok {
        return nil, errors.Unauthorized.With("key does not belong to signer")
    }

    // Check the timestamp
    if transaction.Body.Type() != protocol.TransactionTypeAcmeFaucet &&
        signature.GetTimestamp() != 0 &&
        entry.GetLastUsedOn() >= signature.GetTimestamp() {
        return nil, errors.BadTimestamp.WithFormat("invalid timestamp: have %d, got %d", entry.GetLastUsedOn(), signature.GetTimestamp())
    }

    return entry, nil
}
```

### 5.2 Threshold Validation

For key page signatures, the system:

1. Identifies the key page referenced by the signature
2. Verifies the signature against the public keys in the key page
3. Counts valid signatures toward the appropriate threshold (accept, reject, etc.)
4. Determines if the threshold has been met

The validation process is implemented in the executor's validation pipeline, which collects signatures and evaluates them against the thresholds defined in the key page.

```go
// Simplified threshold validation logic
if validSignatureCount >= keyPage.AcceptThreshold {
    // Transaction is accepted
} else if rejectSignatureCount >= keyPage.RejectThreshold {
    // Transaction is rejected
}
```

The actual implementation in the codebase tracks signatures in the transaction status and evaluates thresholds during transaction processing. The `callSignatureValidator` function in `internal/core/execute/v2/block/exec_validate.go` is a key part of this process:

```go
// From internal/core/execute/v2/block/exec_validate.go
func (b *bundle) callSignatureValidator(batch *database.Batch, ctx *SignatureContext) (*protocol.TransactionStatus, error) {
    // Find the appropriate executor
    x, ok := getExecutor(b.Executor.signatureExecutors, ctx)
    if !ok {
        return protocol.NewErrorStatus(ctx.message.ID(), errors.BadRequest.WithFormat("unsupported signature type %v", ctx.Type())), nil
    }

    // Validate the message
    st, err := x.Validate(batch, ctx)
    err = errors.UnknownError.Wrap(err)
    return st, err
}
```

### 5.3 Multi-Signature Validation

For complex transactions requiring multiple signatures:

1. The system collects all signatures associated with the transaction
2. Groups signatures by their signer (key page or other authority)
3. Validates each group against its respective authority rules
4. Ensures all required authorities have provided sufficient signatures

## 6. Keybook and Key Page Lifecycle

Keybooks and key pages have a defined lifecycle that includes creation, updates, and potentially deprecation.

### 6.1 Keybook Creation

A keybook is created with an initial key page:

```go
// CreateKeyBook transaction body
type CreateKeyBook struct {
    Url           *url.URL
    PublicKeyHash []byte
    Authorities   []*url.URL
}
```

The creation process:
1. Specifies the keybook URL
2. Provides the initial public key hash
3. Optionally defines additional authorities
4. Creates the keybook with an initial key page (page 0)

### 6.2 Adding Key Pages

New key pages can be added to an existing keybook:

```go
// CreateKeyPage transaction body
type CreateKeyPage struct {
    Keys []*KeySpecParams
}
```

The key page creation process:
1. Specifies the keys to include in the new page
2. Sets appropriate thresholds for the page
3. Adds the page to the keybook with an incremented page number

### 6.3 Updating Key Pages

Existing key pages can be updated to add, remove, or modify keys:

```go
// UpdateKeyPage transaction body
type UpdateKeyPage struct {
    Operation []KeyPageOperation
}

// KeyPageOperation types
const (
    KeyPageOperationTypeUpdateKeyPage KeyPageOperationType = 1
    KeyPageOperationTypeAddKey KeyPageOperationType = 2
    KeyPageOperationTypeRemoveKey KeyPageOperationType = 3
    KeyPageOperationTypeUpdateKey KeyPageOperationType = 4
    KeyPageOperationTypeSetThreshold KeyPageOperationType = 5
)
```

Key page updates:
1. Specify the operation type (add, remove, update)
2. Provide the necessary parameters for the operation
3. Require authorization from a key page with sufficient authority

## 7. Signature Contribution from Keybooks

Keybooks contribute signatures to transactions through their key pages. This process involves several steps:

### 7.1 Signature Generation

When a transaction needs to be signed:

1. The client identifies the appropriate keybook and key page
2. Retrieves the private keys corresponding to the public keys in the key page
3. Creates signatures using those private keys
4. Attaches the signatures to the transaction

### 7.2 Signature Sets

For transactions requiring multiple signatures, a signature set is used:

```go
type SignatureSet struct {
    Signatures []Signature
}
```

Signature sets allow:
- Grouping related signatures together
- Tracking which signatures contribute to which thresholds
- Managing complex multi-signature scenarios

### 7.3 Remote and Delegated Signatures

Keybooks support advanced signature patterns:

1. **Remote Signatures**: Allow signatures to be provided by entities not directly associated with the transaction
2. **Delegated Signatures**: Enable one entity to sign on behalf of another through explicit delegation

```go
type RemoteSignature struct {
    Signature Signature
    Signer    *url.URL
}

type DelegatedSignature struct {
    Signature Signature
    Delegator *url.URL
}
```

## 8. Security Considerations

The keybook and signature system incorporates several security features:

### 8.1 Key Rotation

Keys can be rotated without disrupting operations:
1. Add a new key page with new keys
2. Gradually transition operations to the new keys
3. Eventually remove or disable the old key page

### 8.2 Threshold Security

Multi-signature thresholds provide defense in depth:
1. Require multiple signatures for critical operations
2. Set different thresholds for different types of operations
3. Implement m-of-n signature schemes for distributed trust

### 8.3 Delegation Controls

Delegation is carefully controlled:
1. Explicit delegation must be configured in key specifications
2. Delegated signatures are validated against both the delegator and delegate
3. Delegation can be revoked by updating key pages

## 9. Best Practices

When working with keybooks and signatures in Accumulate:

### 9.1 Key Management

1. **Hierarchical Structure**: Design a hierarchical keybook structure with clear separation of concerns
2. **Key Backup**: Ensure secure backup of private keys, especially for high-authority key pages
3. **Threshold Selection**: Choose appropriate thresholds based on security requirements and operational needs

### 9.2 Signature Usage

1. **Minimize Privilege**: Use the lowest-authority key page sufficient for each operation
2. **Signature Freshness**: Generate new signatures for each transaction rather than reusing them
3. **Validation**: Always validate signatures client-side before submitting to the network

## 10. Conclusion

Accumulate's keybook and signature system provides a flexible, secure foundation for identity management and transaction authorization. By understanding the structure and function of keybooks, key pages, and signatures, developers can leverage the full power of Accumulate's security model.

The hierarchical nature of keybooks and key pages, combined with the variety of supported signature types, enables sophisticated security patterns while maintaining usability and flexibility.

## Related Documents

- [ADI and Special Accounts](../03_adi_and_special_accounts.md)
- [Transaction Processing](../../06_implementation/03_transaction_processing.md)
- [Transaction Validation Process](../../04_network/05_consensus/05_02_validation_process.md)
- [Transaction Processing Deep Dive](../../04_network/05_consensus/05_01_transaction_processing_deep_dive.md)
- [Consensus Integration](../../04_network/05_consensus/01_overview.md)
- [Identity Management](../04_identity_management.md)
- [Advanced Signature Validation](./02_advanced_signature_validation.md)
- [Key Rotation Strategies](./03_key_rotation_strategies.md)
- [Delegation Patterns](./04_delegation_patterns.md)
