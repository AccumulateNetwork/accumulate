# Critical Review: What We Got Wrong

## Major Issues

### 1. **Not Using the Actual SDK - We're Bypassing It Entirely**

**What we did:**
- Built custom JSON-RPC clients using raw HTTP requests
- Manually constructed JSON payloads with guessed formats
- No use of actual Accumulate SDK types/interfaces

**What we should have done:**
- Import and use `gitlab.com/accumulatenetwork/accumulate/pkg/api/v3`
- Use the actual `api.Client` type via `api.Dial()`
- Use typed Query, Record, and Transaction structs

**Example of correct approach:**
```go
import "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"

client, _ := api.Dial(ctx, url.Parse("https://mainnet..."))
query := &api.DefaultQuery{Url: accountUrl}
record, _ := client.Query(ctx, query, nil)
```

**Impact:** 🔴 CRITICAL
- Our JSON formats are GUESSES
- Field names might be wrong ("queryType" vs actual enum)
- We don't know the actual wire format
- No type safety
- Will likely fail on first real API call

### 2. **Transaction Signing is Probably Wrong**

**What we did (client/client.go:147-238):**
```go
// Create transaction
tx := map[string]interface{}{
    "header": map[string]interface{}{
        "principal": from,
    },
    "body": map[string]interface{}{
        "type": "sendTokens",
        "to": []map[string]interface{}{...},
    },
}

// Sign it
txData, _ := json.Marshal(tx)
hash := sha256.Sum256(txData)
signature := ed25519.Sign(privateKey, hash[:])
```

**Problems:**
1. We're signing the JSON representation - probably wrong
2. Accumulate likely uses binary encoding (MarshalBinary)
3. Transaction envelope structure is guessed
4. Signature format/wrapping unknown
5. No nonce/sequence number handling
6. No timestamp
7. Missing required fields we don't know about

**Correct approach would use:**
```go
import "gitlab.com/accumulatenetwork/accumulate/protocol"

body := &protocol.SendTokens{...}
txn := &protocol.Transaction{
    Header: protocol.TransactionHeader{
        Principal: fromUrl,
    },
    Body: body,
}
// Use protocol's signing mechanisms
```

**Impact:** 🔴 CRITICAL
- SendTokens will almost certainly fail
- Transactions won't be accepted by network
- Could lose funds if somehow accepted incorrectly

### 3. **Query Parameter Names Are Guessed**

**Our guesses in queries.go:**
```go
"queryType": "chain"           // Is it "type"? "queryType"? An enum?
"chain_name": "main"           // Is it "name"? "chainName"? "chain"?
"minorBlockIndex": index       // Is it "index"? "blockIndex"? "minorIndex"?
"publicKeySearch"              // Exact string? CamelCase? snake_case?
```

**We don't know:**
- Actual field names in JSON
- Whether values are strings, numbers, or enums
- Required vs optional fields
- Default behaviors

**Impact:** 🔴 HIGH
- Queries will return errors
- Even if accepted, results might be wrong
- Pagination might not work

### 4. **Endpoint URLs Might Be Wrong**

**What we hardcoded:**
```go
MainnetEndpoint = "https://mainnet.accumulatenetwork.io/v3"
TestnetEndpoint = "https://testnet.accumulatenetwork.io/v3"
```

**Questions:**
- Are these the actual public endpoints?
- Do they accept JSON-RPC over POST?
- Is it `/v3`? `/v2`? Just `/`?
- Do we need authentication?
- Rate limiting?

**Impact:** 🟡 MEDIUM
- Might work if endpoints are correct
- Easy to fix once we test

### 5. **No Error Handling for Accumulate-Specific Errors**

**What we do:**
```go
if errObj, ok := result["error"].(map[string]interface{}); ok {
    return nil, fmt.Errorf("RPC error: %v", errObj["message"])
}
```

**Missing:**
- Accumulate status codes (Pending, Remote, WrongPartition, etc.)
- Partial success handling
- Retry logic for certain errors
- Transaction status tracking

**Impact:** 🟡 MEDIUM
- Users won't get proper error messages
- Can't distinguish temporary vs permanent failures

### 6. **Lite Account Generation is Simplified**

**What we do (client/client.go:127-144):**
```go
hash := sha256.Sum256(keyBytes)
address := hex.EncodeToString(hash[:20])
return fmt.Sprintf("acc://%s/ACME", address)
```

**Concerns:**
- Is this the correct derivation?
- Should use `protocol.LiteAuthorityForKey()` instead
- Format might be wrong

**Impact:** 🟡 MEDIUM
- Lite accounts might not work
- Users can't send to generated addresses

### 7. **Network Calls Have No Timeouts or Context Handling**

**What we do:**
```go
resp, err := http.Post(c.endpoint, "application/json", bytes.NewReader(data))
```

**Missing:**
- Timeout configuration
- Context cancellation
- Retry logic
- Connection pooling

**Impact:** 🟢 LOW
- Can hang forever
- No graceful cancellation
- But functionally might work

### 8. **No Validation of MCP Tool Arguments**

**What we do:**
```go
url, ok := args["url"].(string)
if !ok {
    return nil, fmt.Errorf("missing required parameter: url")
}
```

**Missing:**
- URL format validation
- Public key format validation
- Amount range checking
- Network name validation

**Impact:** 🟢 LOW
- Will fail later in API call
- Poor error messages

## What We Got Right

✅ **MCP Protocol Implementation**
- Server/client architecture is sound
- Tool registration works correctly
- JSON-RPC 2.0 format for MCP is correct

✅ **Code Structure**
- Clean separation of concerns
- Extensible design
- Good file organization

✅ **Documentation**
- Comprehensive exploration of SDK
- Good understanding of what needs to be built
- Clear roadmap

## Critical Path to Fix

### Priority 1: Use the Actual SDK
1. Replace custom HTTP clients with `api.Client`
2. Use typed Query structs (`api.DefaultQuery`, `api.ChainQuery`, etc.)
3. Use typed Record responses
4. Test against actual network

### Priority 2: Fix Transaction Signing
1. Import `protocol` package
2. Use proper Transaction/Envelope types
3. Use SDK's signing mechanisms
4. Test send tokens on testnet with faucet funds

### Priority 3: Validate Everything
1. Write integration tests against testnet
2. Check each query type returns valid data
3. Document actual vs expected behaviors
4. Fix discovered issues

## Estimated Rework Needed

**To make it actually work:**
- Rewrite client package: 4-6 hours
- Fix transaction signing: 2-3 hours
- Test and fix queries: 3-4 hours
- **Total: 9-13 hours**

**Current state:** Likely 0% functional against real network

**After rework:** Should be 80-90% functional

## Bottom Line

We built a sophisticated MCP server with comprehensive API coverage, but it's essentially a well-documented TODO list. The actual API integration is entirely guessed and will almost certainly not work without significant rework to use the actual Accumulate SDK types and methods.

The good news: The structure is solid and we understand what needs to be done. The fix is straightforward - replace our custom JSON-RPC with proper SDK usage.
