# Load Generator Reliability Fix Plan

## Problem Summary
The load generator is unreliable because it:
1. Doesn't wait for transactions to complete (wallet uses `waitForTxnUsingHash`)  
2. Uses fixed sleep times instead of polling for completion
3. Has no retry mechanism for failed transactions
4. Doesn't verify transaction success before proceeding

## Minimal Fix Implementation

### Step 1: Add Transaction Waiting Function (Priority: HIGH)
Create a function similar to wallet's `waitForTxnUsingHash`:

```go
// Add to sl_helpers.go
func (ctx *LoadTestContext) WaitForTransaction(txID *url.TxID, timeout time.Duration) error {
    deadline := time.Now().Add(timeout)
    
    for time.Now().Before(deadline) {
        status, err := ctx.Client.QueryTransaction(ctx.Context, txID, api.TransactionQueryOptions{
            Wait: 5 * time.Second,
        })
        
        if err == nil && status != nil {
            if status.Status.Code == errors.Delivered {
                return nil
            }
            if status.Status.Code != errors.Pending {
                return fmt.Errorf("transaction failed: %v", status.Status.Message)
            }
        }
        
        time.Sleep(1 * time.Second)
    }
    
    return fmt.Errorf("transaction timeout after %v", timeout)
}
```

### Step 2: Update Critical Functions (Priority: HIGH)

#### 2.1 Fix FundAccount (sl_accounts.go)
```go
func (ctx *LoadTestContext) FundAccount(account LiteAccount, amount int64) error {
    sub, err := ctx.Client.Faucet(ctx.Context, account.URL, api.FaucetOptions{})
    if err != nil {
        return fmt.Errorf("failed to faucet account: %w", err)
    }
    
    if sub == nil || sub.Status.TxID == nil {
        return fmt.Errorf("faucet transaction returned no ID")
    }
    
    // WAIT for transaction instead of fixed sleep
    return ctx.WaitForTransaction(sub.Status.TxID, 30*time.Second)
}
```

#### 2.2 Fix sendACME (sl_accounts.go)
```go
func (ctx *LoadTestContext) sendACME(from, to LiteAccount, amount int64) error {
    // ... existing transaction building code ...
    
    sub, err := ctx.Client.Submit(ctx.Context, env, api.SubmitOptions{})
    if err != nil {
        return fmt.Errorf("failed to send ACME: %w", err)
    }
    
    if len(sub) == 0 || sub[0].Status.TxID == nil {
        return fmt.Errorf("send transaction returned no ID")
    }
    
    // WAIT for transaction
    return ctx.WaitForTransaction(sub[0].Status.TxID, 30*time.Second)
}
```

#### 2.3 Fix AddCredits (sl_credits.go)
```go
func (ctx *LoadTestContext) AddCredits(from, to LiteAccount, amount int64) error {
    // ... existing transaction building code ...
    
    sub, err := ctx.Client.Submit(ctx.Context, env, api.SubmitOptions{})
    if err != nil {
        return fmt.Errorf("failed to add credits: %w", err)
    }
    
    if len(sub) == 0 || sub[0].Status.TxID == nil {
        return fmt.Errorf("add credits transaction returned no ID")
    }
    
    // WAIT for transaction
    err = ctx.WaitForTransaction(sub[0].Status.TxID, 30*time.Second)
    if err != nil {
        return err
    }
    
    to.Credits += credits
    return nil
}
```

### Step 3: Add Retry Logic (Priority: MEDIUM)
```go
// Add to sl_helpers.go
func (ctx *LoadTestContext) SubmitWithRetry(env *messaging.Envelope, maxRetries int) (*api.Submission, error) {
    var lastErr error
    
    for i := 0; i < maxRetries; i++ {
        sub, err := ctx.Client.Submit(ctx.Context, env, api.SubmitOptions{})
        if err == nil && len(sub) > 0 {
            return sub[0], nil
        }
        
        lastErr = err
        if i < maxRetries-1 {
            time.Sleep(time.Duration(i+1) * time.Second) // Exponential backoff
        }
    }
    
    return nil, fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
}
```

### Step 4: Remove Fixed Sleeps (Priority: LOW)
- Remove all `time.Sleep(GetSettlementWait())` calls
- Remove arbitrary 500ms delays between transactions
- Replace with actual transaction waiting

## Implementation Order
1. **Day 1**: Implement `WaitForTransaction` and update `FundAccount`
2. **Day 1**: Update `sendACME` and `AddCredits` to use transaction waiting
3. **Day 2**: Add retry logic for critical operations
4. **Day 2**: Test with small load (100 txs) to verify improvements
5. **Day 3**: Remove remaining fixed sleeps and test at scale

## Expected Results
- Transaction success rate should increase from ~0.6% to >95%
- Predictable timing based on actual completion, not fixed waits
- Proper error reporting when transactions fail
- Ability to handle network congestion gracefully

## Testing Plan
1. Start with 100 transactions at 10 TPS
2. If successful, increase to 1000 at 50 TPS  
3. Finally test 10,000 at 100 TPS
4. Monitor success rates and adjust timeouts as needed

## Total Effort
- ~200 lines of code changes
- 2-3 days including testing
- Minimal risk - adding waiting/retry, not changing core logic