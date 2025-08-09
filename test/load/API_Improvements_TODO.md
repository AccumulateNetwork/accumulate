# API Error Messages and Documentation Improvements

## Overview
This document tracks areas where API error messages and documentation could be improved based on issues encountered during development. These improvements would help developers avoid common mistakes and understand requirements more quickly.

## Error Message Improvements

### 1. Missing Version in Signature Building
**Current Error:**
```
"missing version"
```

**Improved Error:**
```
"missing version: SignWith() requires Version() to be called. For lite accounts use Version(1), for ADI accounts check the current key page version"
```

**Even Better (with example):**
```
"missing version in signature: must call Version() after SignWith(). 
Example: SignWith(url).Version(1).Timestamp(&ts).PrivateKey(key)"
```

**Location:** `pkg/build/signature.go` (or wherever signature building is validated)

### 2. Context-Aware Error Messages for AddCredits
**Current Behavior:** Generic signing errors don't indicate the specific requirements for the transaction type

**Improved Behavior:** When building an AddCredits transaction from a lite account:
```
"AddCredits from lite account requires: SignWith(tokenURL).Version(1) - not SignWith(identityURL)"
```

**Location:** Transaction validation in build package

### 3. Account Not Found Errors
**Current Error:**
```
"Account acc://[address].Main not found"
```

**Improved Error:**
```
"Account acc://[address].Main not found. For lite accounts, ensure the account is funded first via the faucet, or note that AddCredits operations will create the account if it doesn't exist."
```

## Documentation Improvements

### 1. SignWith() Method Documentation
**Location:** `pkg/build/builder.go` or similar

**Current:** Minimal or no inline documentation

**Proposed:**
```go
// SignWith sets the signer for the transaction.
// 
// For lite accounts: 
//   - Use SignWith(liteTokenURL).Version(1)
//   - The lite token URL itself is the signer
//
// For ADI accounts: 
//   - Use SignWith(keyPageURL).Version(currentVersion)
//   - Must specify the key page that will sign
//
// Version() must be called after SignWith() or the transaction will fail
// with "missing version" error.
//
// Example (lite account):
//   SignWith(liteTokenURL).Version(1).Timestamp(&ts).PrivateKey(key)
//
// Example (ADI account):
//   SignWith(adi.JoinPath("book", "1")).Version(keyPageVersion).Timestamp(&ts).PrivateKey(key)
func (b *TxBuilder) SignWith(signerUrl *url.URL) *SignatureBuilder {
```

### 2. AddCredits Transaction Documentation
**Location:** `protocol/operations.go` or transaction type definitions

**Add Examples Section:**
```go
// AddCredits converts ACME tokens to credits for transaction fees.
//
// For lite accounts:
//   - Principal: the lite token account URL  
//   - Signer: the same lite token account URL
//   - Recipient: the lite identity URL (derived from token URL)
//
// Example (lite account adding credits to itself):
//   build.Transaction().
//     For(liteTokenURL).
//     Body(&protocol.AddCredits{
//       Recipient: liteTokenURL.Identity(),
//       Amount:    *big.NewInt(100000),
//       Oracle:    oraclePrice,
//     }).
//     SignWith(liteTokenURL).Version(1).Timestamp(&ts).PrivateKey(key)
//
// For ADI accounts:
//   - Principal: the ADI token account URL
//   - Signer: a key page from the ADI
//   - Recipient: the target identity URL for credits
type AddCredits struct {
```

### 3. Lite Account vs ADI Account Signing Guide
**Location:** New file `docs/signing-guide.md` or in main documentation

**Content:**
```markdown
# Transaction Signing Guide

## Lite Accounts
- Lite accounts are self-signing
- Always use the token URL as the signer
- Always use Version(1)
- Identity URL is derived from token URL

### Common Patterns:
| Operation | Principal | Signer | Version |
|-----------|-----------|---------|---------|
| Send Tokens | liteTokenURL | liteTokenURL | 1 |
| Add Credits | liteTokenURL | liteTokenURL | 1 |
| Create Token Account | liteTokenURL | liteTokenURL | 1 |

## ADI Accounts  
- ADI accounts use key pages for signing
- Must specify the key page URL and current version
- Version number increments with key page updates

### Common Patterns:
| Operation | Principal | Signer | Version |
|-----------|-----------|---------|---------|
| Send Tokens | adiTokenAccount | keyPageURL | current |
| Add Credits | adiTokenAccount | keyPageURL | current |
| Update Key Page | keyPageURL | keyPageURL | current |
```

## Testing Improvements

### 1. Add Signature Building Tests with Clear Examples
**Location:** `pkg/build/signature_test.go`

**Add test cases that demonstrate:**
- Correct lite account signing
- Correct ADI account signing  
- Clear error messages when Version() is missing
- Clear error messages when wrong signer URL is used

### 2. Example Test Utilities
**Location:** `test/testing/helpers.go`

**Add helper functions with clear documentation:**
```go
// SignLiteTransaction creates a properly signed transaction for a lite account.
// This helper ensures Version(1) is always used for lite accounts.
func SignLiteTransaction(txBuilder *TxBuilder, liteTokenURL *url.URL, privateKey ed25519.PrivateKey) (*messaging.Envelope, error) {
    timestamp := uint64(time.Now().UnixMilli())
    return txBuilder.
        SignWith(liteTokenURL).
        Version(1).
        Timestamp(&timestamp).
        PrivateKey(privateKey).
        Done()
}
```

## Implementation Priority

1. **High Priority** (Developer Experience):
   - Improve "missing version" error message
   - Add inline documentation to SignWith() method
   - Add context-aware errors for AddCredits operations

2. **Medium Priority** (Prevent Future Issues):
   - Create comprehensive signing guide documentation
   - Add example test utilities
   - Improve Account not found error messages

3. **Low Priority** (Nice to Have):
   - Add more test cases demonstrating proper usage
   - Create interactive examples or tutorials

## Notes

- These improvements would significantly reduce developer confusion around lite account vs ADI account signing patterns
- The distinction between signing with token URL (lite) vs key page URL (ADI) is a common source of errors
- Version requirements should be more explicitly documented and enforced with helpful error messages
- Consider adding builder pattern validations that catch these issues at compile time rather than runtime where possible

## Related Issues

- Credit purchase failures due to incorrect SignWith URL usage
- Missing version errors that don't explain the requirement
- Confusion between identity URL and token URL for lite accounts
- No clear examples of proper signing patterns in documentation

---
*Last Updated: 2025-01-09*
*Created during debugging of CrossChainConductor per-destination blocking tests*