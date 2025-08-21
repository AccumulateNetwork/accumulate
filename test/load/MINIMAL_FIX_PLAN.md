# Minimal Load Generator Fix

## Single Root Cause
The wallet uses v2 API and works. The load generator uses v3 API and fails.

## Minimal Fix: Add Retry Logic ONLY
Since switching APIs requires significant changes, just add retry logic to the existing v3 calls.

### Step 1: Add ONE Retry Function (10 lines)
```go
// Add to sl_helpers.go
func retryOperation(op func() error) error {
    err := op()
    if err == nil {
        return nil
    }
    // Single retry after 2 seconds
    time.Sleep(2 * time.Second)
    return op()
}
```

### Step 2: Wrap Existing Calls (3 changes total)

#### Change 1: Fix Faucet (sl_accounts.go line ~48)
```go
// OLD:
sub, err := ctx.Client.Faucet(ctx.Context, account.URL, api.FaucetOptions{})

// NEW:
var sub *api.Submission
err := retryOperation(func() error {
    var e error
    sub, e = ctx.Client.Faucet(ctx.Context, account.URL, api.FaucetOptions{})
    return e
})
```

#### Change 2: Fix AddCredits (sl_credits.go line ~34)
```go
// OLD:
sub, err := ctx.Client.Submit(ctx.Context, env, api.SubmitOptions{})

// NEW:
var sub []*api.Submission
err = retryOperation(func() error {
    var e error
    sub, e = ctx.Client.Submit(ctx.Context, env, api.SubmitOptions{})
    return e
})
```

#### Change 3: Fix sendACME (sl_accounts.go line ~143)
```go
// OLD:
sub, err := ctx.Client.Submit(ctx.Context, env, api.SubmitOptions{})

// NEW:
var sub []*api.Submission
err = retryOperation(func() error {
    var e error
    sub, e = ctx.Client.Submit(ctx.Context, env, api.SubmitOptions{})
    return e
})
```

## That's It!
- Total changes: 1 new function + 3 wrapped calls
- Lines changed: ~30 lines total
- Risk: Near zero - just adding retry
- Time to implement: 30 minutes

## Why This Works
Transient network/API failures are likely causing the issues. A single retry with delay handles most transient failures without any architectural changes.

## Test It
```bash
# Test with small load first
go test -v -run TestStreamlinedLoad -args -txs 100 -k 5 -a 5 -tps 10

# If successful, increase
go test -v -run TestStreamlinedLoad -args -txs 1000 -k 10 -a 10 -tps 50
```