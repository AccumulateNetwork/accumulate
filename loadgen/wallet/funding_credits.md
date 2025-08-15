# Credit Management Design

## Overview
This module handles credit distribution from a funding account to target accounts (lite accounts and key pages). It ensures the funding account has sufficient ACME balance and manages credit levels for target accounts.

## Architecture

### Core Components

#### 1. CreditTarget Interface
Represents any account that can receive credits (lite accounts or key pages).

```go
type CreditTarget interface {
    GetURL() *url.URL
    GetCreditBalance() uint64
    SetCreditBalance(uint64)
    GetType() string // "lite" or "keypage"
}
```

#### 2. CreditManager
Main orchestrator for credit operations.

```go
type CreditManager struct {
    client        QueryClient
    submitter     SubmitClient
    signer        TransactionSigner
    fundingAccount *LiteIdentity
}
```

#### 3. Key Functions

##### CheckFundingBalance
- Queries funding account balance
- Returns error if balance < 100 ACME
- Returns current balance for logging

##### CheckTargetCredits
- Queries target account credit balance
- Returns current credits
- Used to determine if top-up needed

##### AddCredits
- Creates AddCredits transaction
- Signs with funding account
- Submits to network
- Adds exactly 1000 credits

## Flow

### Entry Point: TopUpLiteAccount / TopUpKeyPage
1. **Validate Funding Account**
   - Query ACME balance
   - Exit if < 100 ACME
   - Log funding account status

2. **Check Target Credits**
   - Query target account credits
   - Exit if credits > 500 (no top-up needed)
   - Log current credit level

3. **Add Credits**
   - Calculate ACME needed for 1000 credits
   - Create AddCredits transaction
   - Sign and submit
   - Verify success

## Implementation Details

### Credit Calculation
- 1 ACME = 100 credits (at oracle price 100)
- 1000 credits = 10 ACME
- Use big.Int for precision

### Error Handling
- Insufficient funding: Return clear error
- Network errors: Retry with backoff
- Transaction failures: Log and return error

### Transaction Building
Proper envelope creation with:
- Transaction body (AddCredits)
- Signature from funding account
- Proper routing information

## Usage Example

```go
// For lite account
manager := NewCreditManager(client, fundingAccount)
err := manager.TopUpLiteAccount(ctx, liteAccount)

// For key page
err := manager.TopUpKeyPage(ctx, keyPage)
```

## Benefits
1. **Unified Interface**: Single code path for both account types
2. **Safety Checks**: Prevents overdraft of funding account
3. **Efficiency**: Skips unnecessary top-ups (>500 credits)
4. **Clear Separation**: Credit logic isolated from other funding operations
5. **Testability**: Interface-based design enables easy mocking