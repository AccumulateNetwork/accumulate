# Accumulate Transaction Builder

The `build` package provides a fluent API for constructing and signing
Accumulate transactions. This is the recommended way to create transactions for
submission to the network.

## Quick Start

```go
import (
    "gitlab.com/accumulatenetwork/accumulate/pkg/build"
    "gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Create and sign a SendTokens transaction
env, err := build.Transaction().
    For("alice.acme", "tokens").           // Principal account
    SendTokens(100, 0).To("bob.acme", "tokens"). // Transaction body
    SignWith("alice.acme", "book", "1").   // Signer URL
    Version(1).                             // Signer version
    Timestamp(time.Now().UnixMilli()).      // Required for initiator
    PrivateKey(alicePrivateKey).            // ED25519 private key (64 bytes)
    Done()
if err != nil {
    log.Fatal(err)
}

// Submit to the network
client := jsonrpc.NewClient("https://testnet.accumulatenetwork.io/v3")
submissions, err := client.Submit(ctx, env, api.SubmitOptions{})
```

## Important: Signature Format

Accumulate signatures are **not** simply `ed25519.Sign(privateKey, txnHash)`.

The actual signed message is:

```
SHA256(signature_metadata_hash || transaction_hash)
```

Where:
- `signature_metadata_hash` = SHA256 of the signature struct with the signature
  and transaction hash fields cleared
- `transaction_hash` = SHA256(SHA256(header) || SHA256(body))

**Always use this package or `pkg/client/signing` to create signatures.** Do not
call `ed25519.Sign` directly on the transaction hash.

## Transaction Types

### Token Transactions

```go
// Send tokens
build.Transaction().For(sender).
    SendTokens(amount, precision).To(recipient).
    SignWith(signer)...

// Burn tokens
build.Transaction().For(tokenAccount).
    BurnTokens(amount, precision).
    SignWith(signer)...

// Issue tokens (token issuer only)
build.Transaction().For(tokenUrl).
    IssueTokens(amount, precision).To(recipient).
    SignWith(signer)...
```

### Credit Transactions

```go
// Add credits (convert ACME to credits)
build.Transaction().For(acmeTokenAccount).
    AddCredits().To(creditRecipient).WithOracle(oracle).Purchase(creditAmount).
    SignWith(signer)...

// Transfer credits
build.Transaction().For(creditAccount).
    TransferCredits(amount).To(recipient).
    SignWith(signer)...

// Burn credits
build.Transaction().For(creditAccount).
    BurnCredits(amount).
    SignWith(signer)...
```

### Identity and Account Creation

```go
// Create identity (ADI)
build.Transaction().For(sponsor).
    CreateIdentity(identityUrl).WithKey(publicKey).WithKeyBook(bookUrl).
    SignWith(signer)...

// Create token account
build.Transaction().For(identity).
    CreateTokenAccount(accountUrl).ForToken(tokenUrl).
    SignWith(signer)...

// Create data account
build.Transaction().For(identity).
    CreateDataAccount(accountUrl).
    SignWith(signer)...
```

### Key Management

```go
// Create key book
build.Transaction().For(identity).
    CreateKeyBook(bookUrl).WithKeyHash(keyHash).
    SignWith(signer)...

// Create key page
build.Transaction().For(keyBook).
    CreateKeyPage().WithEntry().Hash(keyHash).FinishEntry().FinishPage().
    SignWith(signer)...

// Update key page
build.Transaction().For(keyPage).
    UpdateKeyPage().Add().Entry().Hash(newKeyHash).FinishEntry().FinishOperation().
    SignWith(signer)...
```

### Data Transactions

```go
// Write data
build.Transaction().For(dataAccount).
    WriteData(data...).
    SignWith(signer)...
```

## Signature Options

### Basic Signing

```go
.SignWith("account/book/1").  // Signer URL
    Version(1).                // Signer version (required)
    Timestamp(ts).             // Timestamp in milliseconds (required for initiator)
    PrivateKey(key)            // 64-byte ED25519 private key
```

### Signature Types

```go
.SignWith(signer).
    Type(protocol.SignatureTypeED25519).  // Default
    // or
    Type(protocol.SignatureTypeRCD1).     // Factom RCD1
    // or
    Type(protocol.SignatureTypeBTC).      // Bitcoin (compressed)
    // or
    Type(protocol.SignatureTypeETH).      // Ethereum
```

### Delegated Signatures

```go
.SignWith(actualSigner).
    Delegator(delegatingAuthority).  // Add delegation
    ...
```

### Multiple Signatures

```go
build.Transaction().For(principal).
    Body(txnBody).
    SignWith(signer1).Version(1).Timestamp(&ts).PrivateKey(key1).
    SignWith(signer2).Version(1).Timestamp(&ts).PrivateKey(key2).
    Done()
```

### Using Timestamp Variables

For multiple signatures that need unique timestamps:

```go
var ts uint64 = uint64(time.Now().UnixMilli())

build.Transaction().For(principal).
    Body(txnBody).
    SignWith(signer1).Version(1).Timestamp(&ts).PrivateKey(key1).
    SignWith(signer2).Version(1).Timestamp(&ts).PrivateKey(key2).
    Done()
```

When you pass `&ts`, the builder automatically increments it for each signature.

## Submitting Transactions

```go
import (
    "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
    "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
)

client := jsonrpc.NewClient("https://testnet.accumulatenetwork.io/v3")

submissions, err := client.Submit(ctx, env, api.SubmitOptions{})
if err != nil {
    return err
}

for _, sub := range submissions {
    if sub.Status.Failed() {
        return fmt.Errorf("submission failed: %v", sub.Status.AsError())
    }
    fmt.Printf("Submitted: %v\n", sub.Status.TxID)
}
```

## Using the Faucet

For testnet, you can use the faucet to get ACME tokens:

```go
env, err := build.Faucet(liteTokenUrl)
if err != nil {
    return err
}

_, err = client.Submit(ctx, env, api.SubmitOptions{})
```

## Complete Example

See `example_test.go` in this package for complete, runnable examples.

## Alternative: Using signing.Builder

For more control over signature construction, use `pkg/client/signing`:

```go
import "gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"

builder := &signing.Builder{
    Url:       signerUrl,
    Version:   1,
    Timestamp: signing.TimestampFromValue(uint64(time.Now().UnixMilli())),
    Signer:    signing.PrivateKey(privateKey),
}

// For initiating a new transaction
sig, err := builder.Initiate(txn)

// For signing an existing transaction (non-initiator)
sig, err := builder.Sign(txn.GetHash())
```

## Common Mistakes

### Wrong: Signing transaction hash directly

```go
// DO NOT DO THIS - signatures will be rejected
sig.Signature = ed25519.Sign(privateKey, txn.GetHash())
```

### Right: Using the SDK signing functions

```go
// Use build package
env, err := build.Transaction().For(principal).Body(body).
    SignWith(signer).Version(1).Timestamp(ts).PrivateKey(key).
    Done()

// Or use protocol.SignED25519
protocol.SignED25519(sig, privateKey, nil, txn.GetHash())

// Or use signing.Builder
builder := &signing.Builder{...}
sig, err := builder.Initiate(txn)
```

## See Also

- `pkg/client/signing` - Lower-level signature construction
- `pkg/api/v3` - API client for submitting transactions
- `protocol` - Transaction body types and signature types
