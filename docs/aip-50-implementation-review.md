# AIP-50 Implementation Review Guide

## Summary

This document details the implementation of AIP-50 (User-Specified Transaction Fees) for merge request review.

## Architecture

### Escrow-Based Design

```
          FEE ESCROW LIFECYCLE

LOCK     sig_user.go:escrowUserFees()
         - Debit payer account
         - Credit partition escrow account
         - Emit FeeEscrowPayment synthetic

RECORD   msg_fee_escrow_payment.go:process()
         - Store FeeEscrowEntry in transaction metadata

RELEASE  transaction.go:releaseFeeEscrows()
         - SyntheticDepositTokens to each recipient

REFUND   transaction.go:refundFeeEscrows()
         - SyntheticDepositTokens back to payer
```

### Atomicity Guarantee

Escrow lock occurs within a single database batch on the same partition:
- Payer's token account: same partition as escrow account
- Debit + credit in single `batch.Begin(true)` → `batch.Commit()`

## File Changes

### Protocol Layer

#### `protocol/general.yml`

New types:

```yaml
UserFee:
  fields:
    - name: Recipient
      type: url
      pointer: true
    - name: Amount
      type: bigint
    - name: Token
      type: url
      pointer: true
    - name: Payer
      type: url
      pointer: true

FeeEscrowEntry:
  fields:
    - name: Index
      type: uint
    - name: Amount
      type: bigint
    - name: Token
      type: url
      pointer: true
    - name: Payer
      type: url
      pointer: true
    - name: Recipient
      type: url
      pointer: true
    - name: EscrowAccount
      type: url
      pointer: true
    - name: MessageHash
      type: hash
```

Extended `TransactionHeader`:

```yaml
TransactionHeader:
  fields:
    # ... existing fields ...
    - name: UserFees
      type: UserFee
      pointer: true
      repeatable: true
```

#### `protocol/system_accounts.go`

```go
func EscrowUrl(partitionId string) *url.URL {
    return protocol.PartitionUrl(partitionId).JoinPath("escrow")
}
```

### Messaging Layer

#### `pkg/types/messaging/enums.yml`

```yaml
MessageType:
  # ... existing types ...
  - name: FeeEscrowPayment
    value: 20  # or next available
```

#### `pkg/types/messaging/messages.yml`

```yaml
FeeEscrowPayment:
  union: { type: message }
  fields:
    - name: Amount
      type: bigint
    - name: Token
      type: url
      pointer: true
    - name: Payer
      type: url
      pointer: true
    - name: EscrowAccount
      type: url
      pointer: true
    - name: TxID
      type: txid
      pointer: true
    - name: Cause
      type: txid
      pointer: true
    - name: FeeIndex
      type: uint
```

### Database Layer

#### `internal/database/model.yml`

```yaml
AccountTransaction:
  # ... existing fields ...
  - name: FeeEscrows
    type: FeeEscrowEntry
    collection: set
```

### Executor Layer

#### `internal/core/execute/v2/block/sig_user.go`

New function `escrowUserFees()`:

```go
func (x UserSignature) escrowUserFees(batch *database.Batch, ctx *SignatureContext) error {
    for i, fee := range ctx.transaction.Header.UserFees {
        // 1. Validate fee amount > 0
        if fee.Amount.Sign() <= 0 {
            return errors.BadRequest.WithFormat("user fee %d: amount must be positive", i)
        }

        // 2. Determine payer (default to principal)
        payer := fee.Payer
        if payer == nil {
            payer = ctx.transaction.Header.Principal
        }

        // 3. Load and validate payer account
        var payerAcct *protocol.TokenAccount
        err := batch.Account(payer).Main().GetAs(&payerAcct)
        // ... validate token type, balance ...

        // 4. Debit payer, credit escrow (atomic)
        payerAcct.DebitTokens(&fee.Amount)
        escrowAcct.CreditTokens(&fee.Amount)

        // 5. Emit FeeEscrowPayment synthetic
        ctx.didProduce(batch, ctx.transaction.Header.Principal, &messaging.FeeEscrowPayment{...})
    }
    return nil
}
```

Called from `process()` when signature is initiator:

```go
if ctx.isInitiator && len(ctx.transaction.Header.UserFees) > 0 {
    err := x.escrowUserFees(batch, ctx)
    if err != nil {
        return nil, errors.UnknownError.Wrap(err)
    }
}
```

#### `internal/core/execute/v2/block/msg_fee_escrow_payment.go`

New file implementing `FeeEscrowPayment` message executor:

```go
func init() {
    registerSimpleExec[FeeEscrowPayment](&messageExecutors, messaging.MessageTypeFeeEscrowPayment)
}

type FeeEscrowPayment struct{}

func (FeeEscrowPayment) process(batch *database.Batch, ctx *MessageContext, pay *messaging.FeeEscrowPayment, txn *protocol.Transaction) error {
    // 1. Validate synthetic context
    // 2. Load transaction, validate fee index
    // 3. Store FeeEscrowEntry in transaction metadata
    entry := &protocol.FeeEscrowEntry{
        Index:         pay.FeeIndex,
        Amount:        pay.Amount,
        Token:         pay.Token,
        Payer:         pay.Payer,
        Recipient:     txn.Header.UserFees[pay.FeeIndex].Recipient,
        EscrowAccount: pay.EscrowAccount,
        MessageHash:   *(*[32]byte)(pay.Hash()),
    }
    return batch.Account(pay.TxID.Account()).Transaction(pay.TxID.Hash()).FeeEscrows().Add(entry)
}
```

#### `internal/core/execute/v2/block/transaction.go`

New functions in transaction executor:

```go
func (x *TransactionContext) releaseFeeEscrows(batch *database.Batch, txn *protocol.Transaction, state *chain.ProcessTransactionState) error {
    escrows, err := batch.Account(txn.Header.Principal).Transaction(txn.ID().Hash()).FeeEscrows().Get()
    if err != nil {
        return err
    }
    for _, escrow := range escrows {
        // Emit SyntheticDepositTokens to recipient
        deposit := &protocol.SyntheticDepositTokens{
            Token:  escrow.Token,
            Amount: escrow.Amount,
        }
        state.DidProduceTxn(escrow.Recipient, deposit)
    }
    return batch.Account(txn.Header.Principal).Transaction(txn.ID().Hash()).FeeEscrows().Put(nil)
}

func (x *TransactionContext) refundFeeEscrows(batch *database.Batch, txn *protocol.Transaction, state *chain.ProcessTransactionState) error {
    // Same as release, but sends to escrow.Payer instead of escrow.Recipient
}
```

Integration points:

```go
// In recordSuccessfulTransaction():
err = x.releaseFeeEscrows(batch, delivery.Transaction, state)

// In recordFailedTransaction():
err = x.refundFeeEscrows(batch, delivery.Transaction, state)
```

### Build Layer

#### `pkg/build/transaction.go`

```go
func (b *TransactionBuilder) UserFee(recipient *url.URL, amount *big.Int, token *url.URL, payer *url.URL) *TransactionBuilder {
    b.transaction.Header.UserFees = append(b.transaction.Header.UserFees, &protocol.UserFee{
        Recipient: recipient,
        Amount:    *amount,
        Token:     token,
        Payer:     payer,
    })
    return b
}
```

## Test Coverage

### Unit Tests

| Test | Location | Coverage |
|------|----------|----------|
| Fee validation | `sig_user_test.go` | Amount > 0, token type |
| Escrow lock | `sig_user_test.go` | Debit/credit atomicity |
| FeeEscrowPayment | `msg_fee_escrow_payment_test.go` | Storage, validation |
| Release on success | `transaction_test.go` | Recipient receives |
| Refund on failure | `transaction_test.go` | Payer receives refund |

### Integration Tests

| Test | Location | Scenario |
|------|----------|----------|
| `TestModel1_MultiSig_WithFee_Success` | `user_fees_enforcement_test.go` | Multi-auth + fee success |
| `TestModel1_MultiSig_WithoutFee_Blocked` | `user_fees_enforcement_test.go` | No fee → stuck |
| `TestModel2_DualAuthority_WithFee_Success` | `user_fees_enforcement_test.go` | Dual-auth + fee success |
| `TestModel2_DualAuthority_WithoutFee_Blocked` | `user_fees_enforcement_test.go` | Bypass blocked |
| `TestModel3_Delegation_ServiceSignsForUser` | `user_fees_enforcement_test.go` | Delegation mechanics |
| `TestModel3_Delegation_UserCanBypass` | `user_fees_enforcement_test.go` | Delegation no enforcement |
| `TestModel4_Combined_ServiceActsForUser` | `user_fees_enforcement_test.go` | Combined model |
| `TestModel4_Combined_UserCannotBypass` | `user_fees_enforcement_test.go` | Combined enforcement |

### Run Tests

```bash
go test -v -run "TestModel" ./test/e2e/
go test -v -run "TestUserFees" ./test/e2e/
```

## Security Review Checklist

### Atomicity

- [ ] Escrow lock: debit + credit in same batch
- [ ] Cross-partition: uses existing synthetic guarantees
- [ ] No partial escrow states possible

### Authorization

- [ ] Fee payer must be authorized for payer account
- [ ] Escrow account has no keys (protocol-controlled)
- [ ] No external access to escrow account

### Input Validation

- [ ] Amount > 0 enforced
- [ ] Token whitelist check (ACME only)
- [ ] Recipient validation
- [ ] FeeIndex bounds check

### Failure Modes

- [ ] Insufficient balance → signature rejected
- [ ] Transaction failure → automatic refund
- [ ] Transaction expiration → automatic refund
- [ ] SyntheticDepositTokens DidFail → retry mechanism

### Edge Cases

- [ ] Multiple fees on single transaction
- [ ] Payer different from signer
- [ ] Cross-partition payer
- [ ] Fee to non-existent recipient (handled at validation)

## Protocol Version

Feature gated by `V2UserFeesEnabled`:

```go
if !ctx.GetActiveGlobals().ExecutorVersion.V2UserFeesEnabled() {
    // Skip user fee processing
}
```

Activation requires:
1. All validators running compatible code
2. Governance transaction setting new executor version

## Breaking Changes

None. New optional field in TransactionHeader; existing transactions unaffected.

## Backwards Compatibility

- Old clients can still submit transactions (no UserFees field)
- New field ignored by old validators (pre-activation)
- No migration required

## Performance Impact

- Additional processing during initiator signature (escrow lock)
- Additional synthetic messages (FeeEscrowPayment, SyntheticDepositTokens)
- Negligible compared to existing transaction processing

## Review Recommendations

1. **Start with data structures**: Review `protocol/general.yml` changes
2. **Trace escrow flow**: `sig_user.go` → `msg_fee_escrow_payment.go` → `transaction.go`
3. **Verify atomicity**: Check batch handling in `escrowUserFees()`
4. **Run integration tests**: All 8 enforcement model tests should pass
5. **Check synthetic handling**: Ensure proper sequencing and delivery
