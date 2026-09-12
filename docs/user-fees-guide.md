# User-Specified Transaction Fees (AIP-50)

## Overview

AIP-50 enables users to attach fees to transactions that are paid to designated recipients upon successful execution. Fees are escrowed at transaction initiation and automatically released or refunded based on outcome.

## Quick Start

### Basic Usage

```go
import (
    "math/big"
    "gitlab.com/accumulatenetwork/accumulate/pkg/build"
    "gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Send 10 ACME to recipient, paying 1 ACME fee to service
env, _ := build.Transaction().For(sender, "tokens").
    UserFee(serviceAccount, big.NewInt(1*protocol.AcmePrecision), protocol.AcmeUrl(), sender.JoinPath("tokens")).
    SendTokens(10, protocol.AcmePrecisionPower).To(recipient, "tokens").
    SignWith(sender, "book", "1").Version(1).Timestamp(ts).PrivateKey(key).
    Done()
```

### UserFee Parameters

| Parameter | Description |
|-----------|-------------|
| `recipient` | Account receiving the fee (e.g., `service.JoinPath("fees")`) |
| `amount` | Fee amount as `*big.Int` |
| `token` | Token URL (currently only `protocol.AcmeUrl()`) |
| `payer` | Account paying the fee (defaults to transaction principal) |

## Fee Lifecycle

```
1. SUBMIT     User submits transaction with UserFee in header
              ↓
2. ESCROW     Initiating signature processed → tokens locked in escrow
              ↓
3. PENDING    Transaction awaits additional signatures (if multi-auth)
              ↓
4. OUTCOME    ┬─ SUCCESS → Fee sent to recipient
              ├─ FAILURE → Fee refunded to payer
              └─ EXPIRE  → Fee refunded to payer
```

## Use Cases

### Service Provider Fees

A wallet service charges 0.5 ACME per transaction:

```go
build.Transaction().For(user, "tokens").
    UserFee(walletService.JoinPath("fees"), big.NewInt(protocol.AcmePrecision/2), protocol.AcmeUrl(), user.JoinPath("tokens")).
    SendTokens(100, protocol.AcmePrecisionPower).To(merchant, "tokens").
    SignWith(user, "book", "1").Version(1).Timestamp(ts).PrivateKey(userKey)
```

### Multiple Fees

Pay fees to multiple recipients:

```go
build.Transaction().For(user, "tokens").
    UserFee(service1.JoinPath("fees"), fee1, protocol.AcmeUrl(), user.JoinPath("tokens")).
    UserFee(service2.JoinPath("fees"), fee2, protocol.AcmeUrl(), user.JoinPath("tokens")).
    SendTokens(50, protocol.AcmePrecisionPower).To(recipient, "tokens").
    SignWith(user, "book", "1").Version(1).Timestamp(ts).PrivateKey(userKey)
```

### Custom Fee Payer

Specify a different account to pay the fee:

```go
build.Transaction().For(user, "tokens").
    UserFee(service.JoinPath("fees"), fee, protocol.AcmeUrl(), sponsor.JoinPath("tokens")). // sponsor pays
    SendTokens(10, protocol.AcmePrecisionPower).To(recipient, "tokens").
    SignWith(user, "book", "1").Version(1).Timestamp(ts).PrivateKey(userKey)
```

## Enforcement Models

User fees are voluntary unless combined with authorization controls. Choose a model based on your requirements:

### Model 1: Multi-Signature Authority

Service added as authority on user's account. Both signatures required.

**Setup:**
```go
// Add service as authority on user's token account
build.Transaction().For(user, "tokens").
    UpdateAccountAuth().
    Add(service.JoinPath("book")).
    SignWith(user, "book", "1").Version(1).Timestamp(ts).PrivateKey(userKey)

// Service must co-sign to approve
build.SignatureForTransaction(txn).
    Url(service, "book", "1").Version(1).Timestamp(ts).PrivateKey(serviceKey)
```

**Transaction Flow:**
1. User submits transaction with UserFee
2. Service validates fee is present and sufficient
3. Service co-signs → transaction executes
4. Without fee → service refuses → transaction stuck

### Model 2: Dual-Authority Account

Create account with both authorities from the start:

```go
build.Transaction().For(user).
    CreateTokenAccount(user.JoinPath("tokens")).
    ForToken(protocol.AcmeUrl()).
    WithAuthority(user.JoinPath("book")).
    WithAuthority(service.JoinPath("book")).
    SignWith(user, "book", "1").Version(1).Timestamp(ts).PrivateKey(userKey)
```

### Model 3: Delegation (No Enforcement)

Grant service delegation to sign on user's behalf. User retains direct signing ability.

```go
// Add delegation (via direct database update or UpdateKeyPage)
keyPage.AddKeySpec(&protocol.KeySpec{Delegate: service.JoinPath("book")})

// Service signs as delegate
build.Transaction().For(user, "tokens").
    UserFee(service.JoinPath("fees"), fee, protocol.AcmeUrl(), user.JoinPath("tokens")).
    SendTokens(10, protocol.AcmePrecisionPower).To(recipient, "tokens").
    SignWith(service, "book", "1").Version(1).Timestamp(ts).
    Delegator(user.JoinPath("book", "1")).
    PrivateKey(serviceKey)
```

### Model 4: Combined (Authority + Delegation)

Maximum control: service required to sign AND can act on user's behalf.

```go
// 1. Add service as authority (enforcement)
// 2. Add service as delegate (convenience)

// Service can now:
// - Sign as delegate for user transactions
// - Co-sign as authority (always includes fee)
// User cannot bypass service
```

## Validation Rules

| Condition | Result |
|-----------|--------|
| `Amount <= 0` | Transaction rejected |
| Token not ACME | Transaction rejected |
| Payer insufficient balance | Signature rejected |
| Payer wrong token type | Signature rejected |

## Querying Fees

Check if a transaction has user fees:

```go
txn := queryResult.Message.Transaction
if len(txn.Header.UserFees) > 0 {
    for _, fee := range txn.Header.UserFees {
        fmt.Printf("Fee: %s to %s\n", fee.Amount, fee.Recipient)
    }
}
```

## Error Handling

### Insufficient Balance

```go
// Error during signature processing
// "insufficient balance for user fee"
```

### Invalid Fee Amount

```go
// Error during transaction validation
// "user fee amount must be positive"
```

## Best Practices

1. **Validate before signing**: Services should verify fee presence and amount before co-signing
2. **Use multi-authority for enforcement**: Delegation alone does not prevent bypass
3. **Monitor escrow refunds**: Failed transactions trigger automatic refunds
4. **Set appropriate fee amounts**: Balance between service sustainability and user adoption

## Network Requirements

- Executor version: `V2UserFeesEnabled`
- Token support: ACME only (extensible via governance)
