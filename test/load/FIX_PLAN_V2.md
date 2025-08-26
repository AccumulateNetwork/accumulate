# Load Generator Reliability Fix Plan - V2

## Problem Analysis
The load generator uses v3 JSONRPC API while the wallet (which works reliably) uses v2 API. The issue is NOT about waiting for transactions (they're sent in parallel then waited for), but about API reliability differences.

## Root Cause
1. **API Version Mismatch**: Load generator uses v3 API, wallet uses v2 API
2. **Different Faucet Implementation**: v2 and v3 may handle faucet requests differently
3. **Connection Handling**: v2 client may have better error handling/retry logic

## Minimal Fix Plan

### Option 1: Switch to V2 API (Recommended - Most Reliable)
Since wallet commands work reliably with v2 API, switch the load generator to use v2.

#### Step 1: Add V2 Client Support
```go
// sl_types.go - Update LoadTestContext
type LoadTestContext struct {
    ClientV3    *jsonrpc.Client  // Keep for some operations
    ClientV2    *client.Client   // Add v2 client for critical ops
    Context     context.Context
    // ... rest of fields
}
```

#### Step 2: Update Client Initialization
```go
// sl_helpers.go - Update NewLoadTestContext
func NewLoadTestContext(config LoadConfig) *LoadTestContext {
    endpoint, err := FindDevnetEndpoint()
    if err != nil {
        return nil
    }
    
    // Initialize v3 client
    clientV3 := jsonrpc.NewClient(endpoint)
    clientV3.Client.Timeout = DefaultTimeout
    
    // Initialize v2 client (wallet style)
    v2Endpoint := strings.Replace(endpoint, "/v3", "/v2", 1)
    clientV2, err := client.New(v2Endpoint)
    if err != nil {
        return nil
    }
    clientV2.Timeout = DefaultTimeout
    
    // ... rest of initialization
}
```

#### Step 3: Use V2 for Critical Operations
```go
// sl_accounts.go - Update FundAccount to use v2
func (ctx *LoadTestContext) FundAccount(account LiteAccount, amount int64) error {
    req := &protocol.AcmeFaucet{Url: account.URL}
    
    // Use v2 client like wallet does
    res, err := ctx.ClientV2.Faucet(context.Background(), req)
    if err != nil {
        return fmt.Errorf("failed to faucet account: %w", err)
    }
    
    // v2 response format may differ slightly
    time.Sleep(FaucetDelay)
    return nil
}
```

### Option 2: Fix V3 Connection Issues (Alternative)
If v2 switch is not desired, improve v3 reliability.

#### Step 1: Add Connection Pool
```go
// sl_helpers.go - Create multiple clients for load distribution
func NewLoadTestContext(config LoadConfig) *LoadTestContext {
    // Create a pool of clients to distribute load
    numClients := 5
    clients := make([]*jsonrpc.Client, numClients)
    
    for i := 0; i < numClients; i++ {
        client := jsonrpc.NewClient(endpoint)
        client.Client.Timeout = DefaultTimeout
        clients[i] = client
    }
    
    // Use round-robin or random selection for operations
}
```

#### Step 2: Add Retry with Backoff
```go
// sl_helpers.go - Add intelligent retry
func (ctx *LoadTestContext) SubmitWithRetry(env *messaging.Envelope) ([]*api.Submission, error) {
    maxRetries := 3
    baseDelay := 1 * time.Second
    
    for i := 0; i < maxRetries; i++ {
        sub, err := ctx.Client.Submit(ctx.Context, env, api.SubmitOptions{})
        
        // Check for specific error types
        if err == nil {
            return sub, nil
        }
        
        // Don't retry on certain errors
        if strings.Contains(err.Error(), "invalid") {
            return nil, err
        }
        
        // Exponential backoff
        if i < maxRetries-1 {
            delay := baseDelay * time.Duration(1<<i)
            time.Sleep(delay)
        }
    }
    
    return nil, fmt.Errorf("failed after %d retries", maxRetries)
}
```

### Option 3: Hybrid Approach (Best of Both)
Use v2 for faucet/credits, v3 for normal transactions.

```go
// Use v2 for setup phase (faucet, credits)
func (ctx *LoadTestContext) FundAccount(account LiteAccount, amount int64) error {
    // Use v2 client
    return ctx.ClientV2.Faucet(...)
}

// Keep v3 for load generation (better performance)
func (ctx *LoadTestContext) SendTransaction(from, to LiteAccount, amount int64) error {
    // Use v3 client with retry logic
    return ctx.submitWithRetry(...)
}
```

## Implementation Priority

1. **Day 1 Morning**: Add v2 client alongside v3
2. **Day 1 Afternoon**: Switch faucet and credits to v2
3. **Day 2**: Add retry logic for v3 transactions
4. **Day 2**: Test with increasing loads

## Testing Approach

1. **Phase 1**: Test faucet reliability
   - Run 10 faucet calls with v2
   - Compare to v3 success rate
   
2. **Phase 2**: Test credits reliability
   - Add credits to 10 accounts
   - Verify all succeed with v2
   
3. **Phase 3**: Load test
   - Start with 100 txs at 10 TPS
   - Increase to 1000 at 50 TPS
   - Monitor success rates

## Expected Improvements

- Faucet success: 0% → 100% (v2 is proven reliable)
- Credits success: Low → 100% 
- Transaction success: 0.6% → 95%+
- Predictable behavior matching wallet

## Minimal Code Changes

- ~100 lines to add v2 client support
- ~50 lines to update critical functions
- ~50 lines for retry logic
- Total: ~200 lines, low risk

## Why This Works

The wallet's reliability proves v2 API works correctly with devnet. By using the same API and patterns, we inherit that reliability without redesigning the load generator.