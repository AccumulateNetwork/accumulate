# Signature Processing Pipeline in Accumulate

## Metadata
- **Document Type**: Technical Implementation
- **Version**: 1.0
- **Last Updated**: 2025-05-17
- **Related Components**: Transaction Processing, Security, Identity Management
- **Code References**: `internal/core/execute/v2/block/sig_user.go`, `internal/core/execute/v2/block/sig_authority.go`
- **Tags**: signature_processing, implementation, authority_signatures, transaction_flow

## 1. Introduction

This document details the signature processing pipeline in Accumulate, focusing on how signatures are processed from submission to execution. Understanding this pipeline is essential for developers working on the protocol or building applications that interact with it.

## 2. Signature Processing Overview

The signature processing pipeline in Accumulate consists of several distinct phases:

1. **Signature Submission**: Signatures are submitted to the network
2. **Signature Validation**: Signatures are validated for cryptographic correctness
3. **Authority Verification**: The authority of the signer is verified
4. **Fee Processing**: Fees are calculated and charged
5. **Signature Recording**: Valid signatures are recorded in the state
6. **Threshold Evaluation**: Signatures are evaluated against thresholds
7. **Transaction Execution**: When thresholds are met, the transaction is executed

## 3. Signature Context

The signature processing pipeline operates within a `SignatureContext`, which encapsulates all the information needed to process a signature:

```go
// SignatureContext is the context in which a message is executed.
type SignatureContext struct {
    *TransactionContext
    signature protocol.Signature
}
```

This context provides methods for signature processing, authority verification, and transaction execution.

## 4. User Signature Processing

User signatures (ED25519, ECDSA, RSA, etc.) are processed by the `UserSignature` executor in `internal/core/execute/v2/block/sig_user.go`.

### 4.1 User Signature Validation

The validation process for user signatures includes:

```go
func (x UserSignature) Validate(batch *database.Batch, ctx *SignatureContext) (*protocol.TransactionStatus, error) {
    err := x.check(batch, &userSigContext{SignatureContext: ctx})
    return nil, errors.UnknownError.Wrap(err)
}
```

The `check` method performs several validation steps:
1. Verifies the signature is a valid user signature
2. Unwraps delegated signatures if necessary
3. Verifies the signature signs the transaction
4. Checks the chain ID for EIP-712 signatures
5. Determines if the signature is the transaction initiator
6. Verifies the signer and its authority
7. Checks if the signer can pay the required fees

### 4.2 User Signature Processing

Once validated, user signatures are processed:

```go
func (x UserSignature) Process(batch *database.Batch, ctx *SignatureContext) (_ *protocol.TransactionStatus, err error) {
    batch = batch.Begin(true)
    defer func() { commitOrDiscard(batch, &err) }()

    // Create a user signature context
    uCtx := &userSigContext{SignatureContext: ctx}

    // Check the signature
    err = x.check(batch, uCtx)
    if err != nil {
        return nil, errors.UnknownError.Wrap(err)
    }

    // Process the signature
    err = x.process(batch, uCtx)
    if err != nil {
        return nil, errors.UnknownError.Wrap(err)
    }

    return nil, nil
}
```

The processing includes:
1. Charging fees to the signer
2. Recording the signature in the state
3. Sending signature requests to additional authorities
4. Sending credit payment notifications

## 5. Authority Signature Processing

Authority signatures are a special type of signature used for cross-chain authorization and are processed by the `AuthoritySignature` executor in `internal/core/execute/v2/block/sig_authority.go`.

### 5.1 Authority Signature Validation

Authority signatures have a simpler validation process:

```go
func (x AuthoritySignature) Validate(batch *database.Batch, ctx *SignatureContext) (*protocol.TransactionStatus, error) {
    // An authority signature must be synthetic, so only do enough validation to
    // make sure its valid. Properly produced synthetic messages should _always_
    // be recorded, even if the accounts involved don't exist or are invalid.
    _, err := x.check(batch, ctx)
    return nil, errors.UnknownError.Wrap(err)
}
```

The `check` method verifies:
1. The signature is a valid authority signature
2. The signature has the required fields (origin, transaction ID)
3. The transaction is a user transaction
4. The signature is part of a synthetic message

### 5.2 Authority Signature Processing

Authority signatures are processed differently depending on whether they are direct or delegated:

```go
func (x AuthoritySignature) Process(batch *database.Batch, ctx *SignatureContext) (_ *protocol.TransactionStatus, err error) {
    batch = batch.Begin(true)
    defer func() { commitOrDiscard(batch, &err) }()

    // Check the message for basic validity
    sig, err := x.check(batch, ctx)
    if err != nil {
        return nil, errors.UnknownError.Wrap(err)
    }

    // Process the signature (update the transaction status)
    if len(sig.Delegator) > 0 {
        err = x.processDelegated(batch, ctx, sig)
    } else {
        err = x.processDirect(batch, ctx, sig)
    }
    if err != nil {
        return nil, errors.UnknownError.Wrap(err)
    }

    // Record the cause
    err = batch.Message(ctx.message.Hash()).Cause().Add(sig.Cause)
    if err != nil {
        return nil, errors.UnknownError.WithFormat("store message cause: %w", err)
    }

    // Once a signature has been included in the block, record the signature and
    // its status not matter what, unless there is a system error
    if len(sig.Delegator) > 0 {
        return nil, nil
    }

    // Process the transaction
    _, err = ctx.callMessageExecutor(batch, &messaging.TransactionMessage{Transaction: ctx.transaction})
    if err != nil {
        return nil, errors.UnknownError.Wrap(err)
    }

    return nil, nil
}
```

#### Direct Authority Signatures

Direct authority signatures are processed by:
1. Verifying the transaction has been initiated
2. Verifying the signer is authorized to sign for the principal
3. Adding the signature to the principal's chain
4. Recording the vote

#### Delegated Authority Signatures

Delegated authority signatures are processed by:
1. Loading the delegator
2. Verifying the authority is a delegate of the delegator
3. Verifying the delegator is allowed to sign this type of transaction
4. Adding the signature to the transaction's signature set
5. Sending the next authority signature if needed

## 6. Signature Collection and Threshold Evaluation

Signatures are collected and evaluated against thresholds defined in key pages:

```go
func addSignature(batch *database.Batch, ctx *SignatureContext, signer protocol.Signer, entry *database.SignatureSetEntry) error {
    // Get the authority
    authority := signer.GetAuthority()

    // Get the transaction status
    status, err := batch.Account(ctx.transaction.Header.Principal).
        Transaction(ctx.transaction.ID().Hash()).
        Status().Get()
    if err != nil {
        return errors.UnknownError.WithFormat("load transaction status: %w", err)
    }

    // Get the active signature set
    sigSet, err := batch.Account(authority).
        Transaction(ctx.transaction.ID().Hash()).
        Signatures().Get()
    if err != nil {
        return errors.UnknownError.WithFormat("load signature set: %w", err)
    }

    // Add the signature to the set
    sigSet = append(sigSet, entry)
    err = batch.Account(authority).
        Transaction(ctx.transaction.ID().Hash()).
        Signatures().Put(sigSet)
    if err != nil {
        return errors.UnknownError.WithFormat("store signature set: %w", err)
    }

    // Record the signature in the transaction's history
    err = batch.Account(ctx.transaction.Header.Principal).
        Transaction(ctx.transaction.ID().Hash()).
        RecordHistory(ctx.message)
    if err != nil {
        return errors.UnknownError.WithFormat("record history: %w", err)
    }

    return nil
}
```

## 7. Held Signatures and Block Thresholds

Accumulate supports holding signatures until certain block thresholds are met:

### 7.1 Recording Held Authority Signatures

```go
func (s *SignatureContext) recordOnHoldAuthSig(batch *database.Batch, authSig *protocol.AuthoritySignature) error {
    record := batch.Account(s.Executor.Describe.Ledger()).
        Events().Minor().
        Votes(s.transaction.Header.HoldUntil.MinorBlock)

    other, err := record.Find(authSig)
    switch {
    case err == nil:
        // Found an existing signature. If the origin of the new signature is
        // lower priority (higher page number) than the previous signature,
        // discard it.
        if comparePriority(authSig, other) > 0 {
            return nil
        }

    case errors.Is(err, errors.NotFound):
        // No existing signature so write this one

    default:
        return errors.UnknownError.WithFormat("load previous on-hold auth sig: %w", err)
    }

    err = record.Add(authSig)
    if err != nil {
        return errors.UnknownError.WithFormat("store on-hold auth sig: %w", err)
    }
    return nil
}
```

### 7.2 Releasing Held Signatures

```go
func (m *MessageContext) releaseHeldAuthSigs(batch *database.Batch, blocks []uint64) error {
    record := batch.Account(m.Executor.Describe.Ledger()).
        Events().Minor()

    for _, block := range blocks {
        votes, err := record.Votes(block).Get()
        if err != nil {
            return errors.UnknownError.WithFormat("load held authority signatures: %w", err)
        }

        for _, vote := range votes {
            // Skip votes that don't match our partition
            partition, err := m.Executor.Router.RouteAccount(vote.Authority)
            if err != nil {
                m.logger.Error("Failed to route authority", "error", err, "authority", vote.Authority)
                continue
            }
            if !strings.EqualFold(partition, m.Executor.Describe.PartitionId) {
                continue
            }

            // Produce the authority signature
            err = m.didProduce(
                batch,
                vote.RoutingLocation(),
                &messaging.SignatureMessage{
                    Signature: vote,
                    TxID:      vote.TxID,
                },
            )
            if err != nil {
                return errors.UnknownError.Wrap(err)
            }
        }

        // Clear the votes
        err = record.Votes(block).Put(nil)
        if err != nil {
            return errors.UnknownError.WithFormat("clear held authority signatures: %w", err)
        }
    }

    return nil
}
```

## 8. Signature Verification Flow

The signature verification flow involves several components working together:

### 8.1 Signer Authority Verification

```go
func (s *SignatureContext) signerCanSignTransaction(batch *database.Batch, txn *protocol.Transaction, signer protocol.Signer) error {
    if val, ok := getValidator[chain.SignerCanSignValidator](s.Executor, txn.Body.Type()); ok {
        fallback, err := val.SignerCanSign(s, batch, txn, signer)
        if !fallback || err != nil {
            return errors.UnknownError.Wrap(err)
        }
    }

    return s.TransactionContext.signerCanSignTransaction(txn, signer)
}
```

### 8.2 Transaction-Specific Authority Verification

```go
func (x *TransactionContext) signerCanSignTransaction(txn *protocol.Transaction, signer protocol.Signer) error {
    switch signer := signer.(type) {
    case *protocol.LiteIdentity:
        // A lite token account is only allowed to sign for itself
        if !signer.Url.Equal(txn.Header.Principal.RootIdentity()) {
            return errors.Unauthorized.WithFormat("%v is not authorized to sign transactions for %v", signer.Url, txn.Header.Principal)
        }
        return nil

    case *protocol.KeyPage:
        // Verify that the key page is allowed to sign the transaction
        bit, ok := txn.Body.Type().AllowedTransactionBit()
        if ok && signer.TransactionBlacklist.IsSet(bit) {
            return errors.Unauthorized.WithFormat("%s is not authorized to sign %v", signer.Url, txn.Body.Type())
        }
        return nil

    default:
        // This should never happen
        return errors.InternalError.WithFormat("unknown signer type %v", signer.Type())
    }
}
```

### 8.3 Authority Acceptance Verification

```go
func (m *TransactionContext) authorityIsAccepted(batch *database.Batch, txn *protocol.Transaction, sig *protocol.AuthoritySignature) error {
    // Load the principal
    principal, err := batch.Account(txn.Header.Principal).Main().Get()
    if err != nil {
        return errors.UnknownError.WithFormat("load principal: %w", err)
    }

    // Get the principal's account auth
    auth, err := m.getAccountAuthoritySet(batch, principal)
    if err != nil {
        return errors.UnknownError.Wrap(err)
    }

    // Page belongs to book => authorized
    _, foundAuthority := auth.GetAuthority(sig.Authority)
    if foundAuthority {
        return nil
    }

    // Authorization is disabled and the transaction type does not force authorization => authorized
    if auth.AllAuthoritiesAreDisabled() && !txn.Body.Type().RequireAuthorization() {
        return nil
    }

    // Authorization is enabled => unauthorized
    // Transaction type forces authorization => unauthorized
    return errors.Unauthorized.WithFormat("%v is not authorized to sign transactions for %v", sig.Origin, principal.GetUrl())
}
```

## 9. Fee Calculation and Processing

Fees are calculated based on the signature type and whether it's the initiator:

```go
func (UserSignature) computeSignerFee(ctx *userSigContext) (protocol.Fee, error) {
    // Don't charge fees for internal administrative functions
    signer := ctx.signature.GetSigner()
    _, isBvn := protocol.ParsePartitionUrl(signer)
    if isBvn || protocol.IsDnUrl(signer) {
        return 0, nil
    }

    // Compute the signature fee
    fee, err := ctx.GetActiveGlobals().Globals.FeeSchedule.ComputeSignatureFee(ctx.signature)
    if err != nil {
        return 0, errors.UnknownError.Wrap(err)
    }

    // Only charge the transaction fee for the initial signature
    if !ctx.isInitiator {
        return fee, nil
    }

    // Add the transaction fee for the initial signature
    txnFee, err := ctx.GetActiveGlobals().Globals.FeeSchedule.ComputeTransactionFee(ctx.transaction)
    if err != nil {
        return 0, errors.UnknownError.Wrap(err)
    }

    // Subtract the base signature fee, but not the oversize surcharge if there is one
    fee += txnFee - protocol.FeeSignature
    return fee, nil
}
```

## 10. Integration with Transaction Processing

The signature processing pipeline integrates with the transaction processing pipeline:

1. **Signature Validation**: Signatures are validated before being processed
2. **Signature Recording**: Valid signatures are recorded in the state
3. **Threshold Evaluation**: Signatures are evaluated against thresholds
4. **Transaction Execution**: When thresholds are met, the transaction is executed

This integration ensures that transactions are only executed when properly authorized.

## Related Documents

- [Keybooks and Signatures](../../02_architecture/02_signatures/01_keybooks_and_signatures.md)
- [Advanced Signature Validation](../../02_architecture/02_signatures/02_advanced_signature_validation.md)
- [Signature Validation Implementation](./01_signature_validation_implementation.md)
- [Advanced Signature Types](./02_advanced_signature_types.md)
- [Signature Verification Process](./04_signature_verification_process.md)
- [Transaction Processing](../03_transaction_processing.md)
