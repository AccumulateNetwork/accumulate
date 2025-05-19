# Signature Verification Process in Accumulate

## Metadata
- **Document Type**: Technical Implementation
- **Version**: 1.0
- **Last Updated**: 2025-05-17
- **Related Components**: Transaction Processing, Security, Identity Management
- **Code References**: `protocol/signature.go`, `internal/core/execute/v2/block/sig_validator.go`
- **Tags**: signature_verification, implementation, security, validation_process

## 1. Introduction

This document provides a detailed explanation of the signature verification process in Accumulate. Understanding this process is critical for developers implementing clients that interact with the Accumulate protocol, as well as for those working on the protocol itself.

## 2. Signature Verification Flow

The signature verification process in Accumulate follows a multi-step flow that ensures the cryptographic validity of signatures and their authority to perform the requested operations.

### 2.1 High-Level Verification Flow

The signature verification process consists of these major steps:

1. **Signature Type Identification**: Determine the signature type and route to the appropriate validator
2. **Cryptographic Verification**: Verify the cryptographic validity of the signature
3. **Authority Verification**: Verify that the signer has authority to sign for the principal
4. **Threshold Evaluation**: For key pages, evaluate signatures against thresholds
5. **Transaction Execution**: If verification succeeds, execute the transaction

### 2.2 Verification Entry Points

The main entry point for signature verification is in the `Executor.Validate` method in `internal/core/execute/v2/block/exec_validate.go`:

```go
func (x *Executor) Validate(envelope *messaging.Envelope, _ bool) ([]*protocol.TransactionStatus, error) {
    batch := x.db.Begin(false)
    defer batch.Discard()

    messages, err := envelope.Normalize()
    if err != nil {
        return nil, errors.UnknownError.Wrap(err)
    }

    // Make sure every transaction is signed
    err = x.checkForUnsignedTransactions(messages)
    if err != nil {
        return nil, errors.UnknownError.Wrap(err)
    }

    // Set up the bundle
    d := new(bundle)
    d.Block = new(Block)
    d.Executor = x
    d.logger = x.logger

    // Validate each message
    statuses := make([]*protocol.TransactionStatus, len(messages))
    for i, msg := range messages {
        // Validate the message
        ctx := &MessageContext{bundle: d, message: msg}
        s, err := d.callMessageValidator(batch, ctx)
        // ... error handling and status processing
    }

    return statuses, nil
}
```

## 3. Cryptographic Signature Verification

Each signature type implements its own cryptographic verification logic through the `Verify` method.

### 3.1 ED25519 Signature Verification

ED25519 signatures are verified using the standard Go crypto library:

```go
func (e *ED25519Signature) Verify(sig Signature, msg Signable) bool {
    return verifySig(e, sig, false, msg, func(msg []byte) bool {
        return ed25519.Verify(e.PublicKey, msg, e.Signature)
    })
}
```

### 3.2 Common Verification Helper

The `verifySig` helper function standardizes signature verification across different signature types:

```go
func verifySig(sig KeySignature, outer Signature, legacy bool, msg Signable, verify func([]byte) bool) bool {
    if outer == nil {
        outer = sig
    }

    var hash [32]byte
    if legacy {
        // Legacy verification
        hash = msg.Hash()
    } else {
        // Standard verification
        if !bytes.Equal(outer.GetTransactionHash().Bytes(), msg.Hash().Bytes()) {
            return false
        }
        hash = outer.GetTransactionHash()
    }

    // Call the type-specific verification function
    return verify(hash[:])
}
```

## 4. Signer Authority Verification

After cryptographic verification, the system verifies that the signer has authority to sign for the principal account.

### 4.1 Key Signature Validation

The core validation for key signatures is implemented in `internal/core/execute/v1/block/signature.go`:

```go
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

### 4.2 Signer Authority Check

The system checks if a signer has authority to sign for a transaction:

```go
func (x *SignatureValidator) SignerCanSign(batch *database.Batch, ctx *block.SignatureContext, signer protocol.Signer) (bool, error) {
    // Get the transaction type
    txType := ctx.Transaction().Body.Type()
    
    // Find the appropriate executor
    exec, ok := getExecutor(x.Executors, txType)
    if !ok {
        return false, errors.BadRequest.WithFormat("unsupported transaction type %v", txType)
    }
    
    // Check if the signer can sign
    fallback, err := exec.SignerCanSign(x, batch, ctx.Transaction(), signer)
    if err != nil {
        return false, errors.UnknownError.Wrap(err)
    }
    
    return fallback, nil
}
```

## 5. Threshold Signature Validation

Key pages in Accumulate support threshold signatures, where multiple signatures may be required to authorize a transaction.

### 5.1 Signature Collection

Signatures are collected and tracked in the transaction status:

```go
func (x *SignatureValidator) CollectSignature(batch *database.Batch, ctx *block.SignatureContext, signature protocol.Signature) (*protocol.TransactionStatus, error) {
    // Get the transaction status
    status, err := x.GetStatus(batch, ctx.Transaction())
    if err != nil {
        return nil, errors.UnknownError.Wrap(err)
    }

    // Add the signature to the status
    status.AddSignature(signature)
    
    // Update the status
    err = batch.Put(status)
    if err != nil {
        return nil, errors.UnknownError.Wrap(err)
    }
    
    return status, nil
}
```

### 5.2 Threshold Evaluation

The system evaluates collected signatures against the thresholds defined in the key page:

```go
func (s *TransactionStatus) AddSignature(sig protocol.Signature) {
    // Add the signature to the appropriate collection
    switch sig.Type() {
    case protocol.SignatureTypeED25519, protocol.SignatureTypeLegacyED25519, protocol.SignatureTypeRCD1:
        s.addKeySignature(sig.(protocol.KeySignature))
    case protocol.SignatureTypeDelegate:
        s.addDelegateSignature(sig.(*protocol.DelegatedSignature))
    // ... other signature types
    }
    
    // Update signature counts
    s.SignatureCount++
}

func (s *TransactionStatus) EvaluateThresholds(keyPage *protocol.KeyPage) {
    // Count valid signatures
    validSignatureCount := s.countValidSignatures()
    rejectSignatureCount := s.countRejectSignatures()
    
    // Evaluate against thresholds
    if validSignatureCount >= keyPage.AcceptThreshold {
        s.Accepted = true
    } else if rejectSignatureCount >= keyPage.RejectThreshold {
        s.Rejected = true
    }
}
```

## 6. Delegation Verification

Delegation verification is a critical part of Accumulate's signature system, allowing one entity to sign on behalf of another.

### 6.1 Delegated Signature Verification

When a delegated signature is used, the system verifies both the signature itself and the delegation relationship:

```go
func (d *DelegatedSignature) Verify(sig protocol.Signature, msg protocol.Signable) bool {
    // Verify the signature itself
    if !d.Signature.(protocol.UserSignature).Verify(sig, msg) {
        return false
    }
    
    // Verify the delegation relationship
    // This is handled at a higher level in the validation pipeline
    return true
}
```

### 6.2 Delegation Chain Verification

The system verifies the entire delegation chain to ensure proper authorization:

```go
func verifyDelegationChain(batch *database.Batch, delegator *url.URL, delegate protocol.Signer) (bool, error) {
    // Get the delegator account
    delegatorAccount, err := batch.Account(delegator).Get()
    if err != nil {
        return false, errors.UnknownError.Wrap(err)
    }
    
    // Check if the delegate is directly authorized by the delegator
    if isDirectDelegate(delegatorAccount, delegate) {
        return true, nil
    }
    
    // Check for indirect delegation through a chain
    return checkIndirectDelegation(batch, delegatorAccount, delegate)
}
```

## 7. Special Signature Types Verification

Accumulate supports several special signature types with custom verification logic.

### 7.1 Remote Signature Verification

Remote signatures allow for cross-partition authorization:

```go
func (r *RemoteSignature) Verify(sig protocol.Signature, msg protocol.Signable) bool {
    // Remote signatures are verified by the system during cross-partition processing
    // This is a placeholder for the actual verification logic
    return true
}
```

### 7.2 Synthetic Transaction Signature Verification

Synthetic transactions (cross-chain transactions) require special verification:

```go
func validateSyntheticTransactionSignatures(transaction *protocol.Transaction, signatures []protocol.Signature) error {
    var gotSynthSig, gotReceiptSig, gotED25519Sig bool
    for _, sig := range signatures {
        switch sig.(type) {
        case *protocol.PartitionSignature:
            gotSynthSig = true

        case *protocol.ReceiptSignature:
            gotReceiptSig = true

        case *protocol.ED25519Signature, *protocol.LegacyED25519Signature:
            gotED25519Sig = true

        default:
            return errors.BadRequest.WithFormat("synthetic transaction do not support %T signatures", sig)
        }
    }

    if !gotSynthSig {
        return errors.Unauthenticated.WithFormat("missing synthetic transaction origin")
    }
    if !gotED25519Sig {
        return errors.Unauthenticated.WithFormat("missing ED25519 signature")
    }
    if transaction.Body.Type() == protocol.TransactionTypeDirectoryAnchor || transaction.Body.Type() == protocol.TransactionTypeBlockValidatorAnchor {
        return nil
    }

    if !gotReceiptSig {
        return errors.Unauthenticated.WithFormat("missing synthetic transaction receipt")
    }
    return nil
}
```

## 8. Timestamp Validation

Signature timestamps are validated to prevent replay attacks and ensure proper ordering:

```go
func validateTimestamp(signature protocol.KeySignature, entry protocol.KeyEntry) error {
    // Skip timestamp validation for certain transaction types
    if signature.GetTimestamp() == 0 {
        return nil
    }
    
    // Check that the timestamp is newer than the last used timestamp
    if entry.GetLastUsedOn() >= signature.GetTimestamp() {
        return errors.BadTimestamp.WithFormat(
            "invalid timestamp: have %d, got %d",
            entry.GetLastUsedOn(),
            signature.GetTimestamp(),
        )
    }
    
    // Check that the timestamp is not too far in the future
    currentTime := uint64(time.Now().Unix())
    maxFutureTime := currentTime + MaxFutureTimestampSeconds
    if signature.GetTimestamp() > maxFutureTime {
        return errors.BadTimestamp.WithFormat(
            "timestamp too far in the future: now %d, got %d, max %d",
            currentTime,
            signature.GetTimestamp(),
            maxFutureTime,
        )
    }
    
    return nil
}
```

## 9. Version Validation

Signature versions are validated to ensure they match the current state of the signer:

```go
func validateVersion(transaction *protocol.Transaction, signer protocol.Signer, signature protocol.KeySignature) error {
    // Skip version validation for system transactions
    if !transaction.Body.Type().IsUser() {
        return nil
    }
    
    // Check that the signature version matches the signer version
    if signature.GetSignerVersion() != signer.GetVersion() {
        return errors.BadSignerVersion.WithFormat(
            "invalid version: have %d, got %d",
            signer.GetVersion(),
            signature.GetSignerVersion(),
        )
    }
    
    return nil
}
```

## 10. Error Handling and Security Considerations

### 10.1 Error Types

The system defines specific error types for signature validation:

```go
// From errors/errors.go
var (
    // Unauthenticated indicates the request lacks valid authentication.
    Unauthenticated = Code("unauthenticated")

    // Unauthorized indicates the authenticated principal lacks permission.
    Unauthorized = Code("unauthorized")

    // BadSignature indicates the signature is invalid.
    BadSignature = Code("bad-signature")

    // BadTimestamp indicates the timestamp is invalid.
    BadTimestamp = Code("bad-timestamp")

    // BadSignerVersion indicates the signer version is invalid.
    BadSignerVersion = Code("bad-signer-version")
)
```

### 10.2 Security Considerations

When implementing signature verification, consider these security aspects:

1. **Replay Protection**: Timestamps and nonces are used to prevent signature replay attacks.

2. **Version Control**: Version checking ensures signatures are valid for the current state of the signer.

3. **Threshold Security**: Threshold signatures provide defense in depth by requiring multiple signers.

4. **Key Compromise Mitigation**: Key rotation strategies help mitigate the impact of key compromise.

5. **Delegation Security**: Delegation chains are fully validated to prevent unauthorized delegation.

## Related Documents

- [Keybooks and Signatures](../../02_architecture/02_signatures/01_keybooks_and_signatures.md)
- [Advanced Signature Validation](../../02_architecture/02_signatures/02_advanced_signature_validation.md)
- [Signature Validation Implementation](./01_signature_validation_implementation.md)
- [Advanced Signature Types](./02_advanced_signature_types.md)
- [Signature Interoperability](./03_signature_interoperability.md)
- [Transaction Processing](../03_transaction_processing.md)
