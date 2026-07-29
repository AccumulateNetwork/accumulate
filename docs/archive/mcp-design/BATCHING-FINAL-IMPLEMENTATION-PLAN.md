# Batching Implementation: Final Plan

**Status:** ✅ APPROVED - Proceed with Implementation
**Date:** 2025-10-20
**Target:** Phase 1 - Simplified Batch Tool

---

## Decision: Implement Batching for Convenience

**Approved Scope:**
- Phase 1: Simple `accumulate_submit_batch` tool
- Timeline: 1 week implementation
- Focus: Convenience (single API call for multiple transactions)

**Clear Limitations (Documented):**
- ❌ No fee savings
- ❌ No atomicity guarantees
- ⚠️ Partial execution possible

---

## Phase 1: Implementation Checklist

### Step 1: Create Batch Submission Tool (2-3 days)

**File:** `mcp-accumulate/server/tools_batch.go`

```go
package server

import (
    "context"
    "encoding/json"
    "fmt"

    "gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
    "gitlab.com/accumulatenetwork/accumulate/protocol"
    "gitlab.com/accumulatenetwork/mcp-go"
)

func (s *Server) handleSubmitBatch(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    var params struct {
        Transactions []struct {
            Type   string                 `json:"type"`
            Params map[string]interface{} `json:"params"`
        } `json:"transactions"`
        SigningMethod string `json:"signing_method"`
        KeyName       string `json:"key_name"`
        Password      string `json:"password"`
        Wait          bool   `json:"wait"`
    }

    if err := json.Unmarshal(request.Params.Arguments, &params); err != nil {
        return nil, fmt.Errorf("invalid parameters: %w", err)
    }

    // Validate we have transactions
    if len(params.Transactions) == 0 {
        return nil, fmt.Errorf("no transactions provided")
    }

    // Build all transactions
    var txns []*protocol.Transaction
    var sigs []protocol.Signature

    for i, txSpec := range params.Transactions {
        txn, sig, err := s.buildTransaction(ctx, txSpec.Type, txSpec.Params, params.KeyName, params.Password)
        if err != nil {
            return nil, fmt.Errorf("failed to build transaction %d (%s): %w", i, txSpec.Type, err)
        }
        txns = append(txns, txn)
        sigs = append(sigs, sig)
    }

    // Submit as batch using existing SubmitEnvelope
    hashes, err := s.client.SubmitEnvelope(ctx, txns, sigs)
    if err != nil {
        return nil, fmt.Errorf("failed to submit batch: %w", err)
    }

    // Wait for completion if requested
    statuses := make([]string, len(hashes))
    if params.Wait {
        for i, hash := range hashes {
            status, err := s.waitForTransaction(ctx, hash)
            if err != nil {
                statuses[i] = fmt.Sprintf("error: %v", err)
            } else {
                statuses[i] = status.Code.String()
            }
        }
    }

    // Return results
    result := map[string]interface{}{
        "transaction_count": len(txns),
        "transaction_hashes": hashesToStrings(hashes),
        "status": "submitted",
    }
    if params.Wait {
        result["transaction_statuses"] = statuses
    }

    resultJSON, _ := json.Marshal(result)
    return &mcp.CallToolResult{
        Content: []mcp.Content{{
            Type: "text",
            Text: string(resultJSON),
        }},
    }, nil
}

func (s *Server) buildTransaction(ctx context.Context, txType string, params map[string]interface{}, keyName, password string) (*protocol.Transaction, protocol.Signature, error) {
    // Map transaction type to builder function
    switch txType {
    case "sendTokens":
        return s.buildSendTokens(ctx, params, keyName, password)
    case "createIdentity":
        return s.buildCreateIdentity(ctx, params, keyName, password)
    case "createTokenAccount":
        return s.buildCreateTokenAccount(ctx, params, keyName, password)
    case "createDataAccount":
        return s.buildCreateDataAccount(ctx, params, keyName, password)
    case "writeData":
        return s.buildWriteData(ctx, params, keyName, password)
    case "updateKeyPage":
        return s.buildUpdateKeyPage(ctx, params, keyName, password)
    // Add other transaction types as needed
    default:
        return nil, nil, fmt.Errorf("unsupported transaction type: %s", txType)
    }
}

func hashesToStrings(hashes [][]byte) []string {
    result := make([]string, len(hashes))
    for i, hash := range hashes {
        result[i] = fmt.Sprintf("%x", hash)
    }
    return result
}
```

**Implementation Notes:**
- Reuses existing transaction builders
- Uses existing `SubmitEnvelope()` method
- Supports optional `wait` parameter
- Returns individual transaction hashes

### Step 2: Register Tool Definition (1 day)

**File:** `mcp-accumulate/server/tool_definitions.go`

Add to tool list:

```go
{
    Name: "accumulate_submit_batch",
    Description: "Submit multiple transactions in one API call for convenience. " +
        "WARNING: Each transaction is charged individually (no fee savings). " +
        "Transactions execute independently (no atomicity - partial failures possible).",
    InputSchema: map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "transactions": map[string]interface{}{
                "type": "array",
                "description": "Array of transactions to submit together",
                "items": map[string]interface{}{
                    "type": "object",
                    "properties": map[string]interface{}{
                        "type": map[string]interface{}{
                            "type": "string",
                            "description": "Transaction type (sendTokens, createIdentity, writeData, etc.)",
                            "enum": []string{
                                "sendTokens",
                                "createIdentity",
                                "createTokenAccount",
                                "createDataAccount",
                                "writeData",
                                "updateKeyPage",
                                // Add more as needed
                            },
                        },
                        "params": map[string]interface{}{
                            "type": "object",
                            "description": "Transaction-specific parameters",
                        },
                    },
                    "required": []string{"type", "params"},
                },
                "minItems": 1,
            },
            "signing_method": map[string]interface{}{
                "type": "string",
                "enum": []string{"wallet"},
                "default": "wallet",
                "description": "Signing method (currently only wallet supported)",
            },
            "key_name": map[string]interface{}{
                "type": "string",
                "description": "Wallet key name to sign with",
            },
            "password": map[string]interface{}{
                "type": "string",
                "description": "Vault password",
            },
            "wait": map[string]interface{}{
                "type": "boolean",
                "default": false,
                "description": "Wait for transaction confirmation",
            },
        },
        "required": []string{"transactions", "key_name", "password"},
    },
}
```

**Implementation Notes:**
- Clear warnings in description
- Enum of supported transaction types
- Required parameters marked
- Optional `wait` parameter

### Step 3: Wire Up Handler (1 day)

**File:** `mcp-accumulate/server/server.go`

Add to tool handler switch:

```go
func (s *Server) CallTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    switch request.Params.Name {
    // ... existing cases ...

    case "accumulate_submit_batch":
        return s.handleSubmitBatch(ctx, request)

    // ... rest of cases ...
    }
}
```

### Step 4: Add Tests (1-2 days)

**File:** `mcp-accumulate/server/tools_batch_test.go`

```go
package server

import (
    "context"
    "testing"

    "github.com/stretchr/testify/require"
)

func TestSubmitBatch_Success(t *testing.T) {
    srv := NewTestServer(t)

    params := map[string]interface{}{
        "transactions": []map[string]interface{}{
            {
                "type": "sendTokens",
                "params": map[string]interface{}{
                    "from": "acc://test.acme/tokens",
                    "to": "acc://alice.acme/tokens",
                    "amount": "1000000000",
                },
            },
            {
                "type": "sendTokens",
                "params": map[string]interface{}{
                    "from": "acc://test.acme/tokens",
                    "to": "acc://bob.acme/tokens",
                    "amount": "2000000000",
                },
            },
        },
        "signing_method": "wallet",
        "key_name": "test-key",
        "password": "test-password",
    }

    result, err := srv.handleSubmitBatch(context.Background(), wrapParams(params))
    require.NoError(t, err)
    require.NotNil(t, result)

    var data map[string]interface{}
    json.Unmarshal([]byte(result.Content[0].Text), &data)
    require.Equal(t, 2, int(data["transaction_count"].(float64)))
    require.Len(t, data["transaction_hashes"], 2)
}

func TestSubmitBatch_PartialFailure(t *testing.T) {
    srv := NewTestServer(t)

    params := map[string]interface{}{
        "transactions": []map[string]interface{}{
            {
                "type": "sendTokens",
                "params": map[string]interface{}{
                    "from": "acc://test.acme/tokens",
                    "to": "acc://alice.acme/tokens",
                    "amount": "100",
                },
            },
            {
                "type": "sendTokens",
                "params": map[string]interface{}{
                    "from": "acc://test.acme/tokens",
                    "to": "acc://bob.acme/tokens",
                    "amount": "999999999999999", // Will fail - insufficient balance
                },
            },
            {
                "type": "sendTokens",
                "params": map[string]interface{}{
                    "from": "acc://test.acme/tokens",
                    "to": "acc://charlie.acme/tokens",
                    "amount": "100",
                },
            },
        },
        "signing_method": "wallet",
        "key_name": "test-key",
        "password": "test-password",
        "wait": true,
    }

    result, err := srv.handleSubmitBatch(context.Background(), wrapParams(params))
    require.NoError(t, err)

    var data map[string]interface{}
    json.Unmarshal([]byte(result.Content[0].Text), &data)

    statuses := data["transaction_statuses"].([]interface{})
    require.Len(t, statuses, 3)

    // Verify partial execution
    // Transaction 1 and 3 should succeed, transaction 2 should fail
    t.Logf("Transaction statuses: %v", statuses)
}

func TestSubmitBatch_EmptyTransactions(t *testing.T) {
    srv := NewTestServer(t)

    params := map[string]interface{}{
        "transactions": []map[string]interface{}{},
        "signing_method": "wallet",
        "key_name": "test-key",
        "password": "test-password",
    }

    _, err := srv.handleSubmitBatch(context.Background(), wrapParams(params))
    require.Error(t, err)
    require.Contains(t, err.Error(), "no transactions")
}
```

**Test Coverage:**
- ✅ Successful batch submission
- ✅ Partial failure scenario (demonstrates independent execution)
- ✅ Empty batch error handling
- ✅ Invalid transaction type
- ✅ Integration with devnet

### Step 5: Documentation Updates (1 day)

**File:** `mcp-accumulate/README.md`

Add section:

```markdown
## Batching Transactions

Submit multiple transactions in one API call for convenience.

### Quick Example

```javascript
{
  "tool": "accumulate_submit_batch",
  "params": {
    "transactions": [
      {
        "type": "sendTokens",
        "params": {
          "from": "acc://alice.acme/tokens",
          "to": "acc://bob.acme/tokens",
          "amount": "1000000000"
        }
      },
      {
        "type": "sendTokens",
        "params": {
          "from": "acc://alice.acme/tokens",
          "to": "acc://charlie.acme/tokens",
          "amount": "2000000000"
        }
      }
    ],
    "signing_method": "wallet",
    "key_name": "my-key",
    "password": "my-password"
  }
}
```

### Important Limitations

⚠️ **No Fee Savings** - Each transaction is charged individually
⚠️ **No Atomicity** - Transactions execute independently (partial failures possible)
⚠️ **Error Handling Required** - Check individual transaction statuses

### When to Use

- ✅ Submitting multiple transactions (convenience)
- ✅ Simplifying client code (fewer API calls)
- ✅ Bulk operations (payroll, updates)

### When NOT to Use

- ❌ Need all-or-nothing execution
- ❌ Expect fee savings
- ❌ Critical operations where partial failure is unacceptable

For atomic operations, use transaction types that support multiple operations (e.g., SendTokens with multiple recipients).

See [BATCHING-USER-GUIDE.md](docs/BATCHING-USER-GUIDE.md) for detailed examples.
```

---

## Timeline

### Week 1: Implementation
- **Day 1-3:** Implement `handleSubmitBatch()` and `buildTransaction()`
- **Day 4:** Add tool definition and wire up handler
- **Day 5:** Write unit tests and integration tests

### Week 2: Testing & Documentation
- **Day 1-2:** Test against devnet, fix issues
- **Day 3:** Update README and documentation
- **Day 4:** Code review and polish
- **Day 5:** Merge and release

**Total:** ~2 weeks (1 week core implementation + 1 week testing/polish)

---

## Success Criteria

### Technical
- ✅ Tool accepts multiple transactions
- ✅ Returns individual transaction hashes
- ✅ Demonstrates partial execution in tests
- ✅ Works with existing wallet integration
- ✅ No memory leaks or performance issues

### Documentation
- ✅ Clear limitations documented
- ✅ Examples provided
- ✅ Warning about partial failures
- ✅ Error handling examples

### User Experience
- ✅ Reduces API calls from N to 1
- ✅ Simpler code for bulk operations
- ✅ Clear error messages
- ✅ Expectations correctly set

---

## Risk Mitigation

### Risk 1: User Expects Atomicity

**Mitigation:**
- Heavy warnings in tool description
- Examples showing partial failures
- Test demonstrating independent execution
- Documentation emphasizes limitations

### Risk 2: Complex Error Handling

**Mitigation:**
- Return individual transaction statuses
- Provide clear error messages
- Document error handling patterns
- Examples in documentation

### Risk 3: Performance Issues

**Mitigation:**
- Limit batch size (suggest < 20 transactions)
- Test with various batch sizes
- Monitor memory usage
- Implement timeout handling

---

## Post-Implementation

### Monitoring
- Track batch usage (how many users use it)
- Monitor average batch size
- Collect feedback on usefulness
- Track error rates

### Future Enhancements (If Needed)
- Batch templates for common operations
- Better error aggregation
- Retry logic for failed transactions
- Progress callbacks for long batches

### Decision Point
After 3-6 months of usage:
- If low usage: Consider deprecating
- If high usage with issues: Consider Phase 2 enhancements
- If high usage with satisfaction: Keep as-is

---

## Files to Create/Modify

### New Files
1. `mcp-accumulate/server/tools_batch.go` - Batch handling logic
2. `mcp-accumulate/server/tools_batch_test.go` - Tests
3. `mcp-accumulate/docs/BATCHING-USER-GUIDE.md` - User guide (already created)

### Modified Files
1. `mcp-accumulate/server/tool_definitions.go` - Add tool definition
2. `mcp-accumulate/server/server.go` - Wire up handler
3. `mcp-accumulate/README.md` - Add batching section
4. `mcp-accumulate/CHANGELOG.md` - Document new feature

---

## Implementation Notes

### Reuse Existing Code
- ✅ Use existing `SubmitEnvelope()` method
- ✅ Reuse transaction builder functions
- ✅ Leverage existing wallet integration
- ✅ Use existing error handling patterns

### Keep It Simple
- ❌ No state management (Phase 2 stateful tools)
- ❌ No complex batch lifecycle
- ❌ No batch templates (yet)
- ✅ Just build, sign, and submit in one call

### Clear Warnings
- Tool description includes limitations
- Documentation emphasizes partial execution
- Examples show failure handling
- Set correct user expectations

---

## Go / No-Go Checklist

Before merging:
- [ ] Unit tests pass
- [ ] Integration tests pass (devnet)
- [ ] Partial failure demonstrated in tests
- [ ] Documentation complete with warnings
- [ ] Code review approved
- [ ] No performance regressions
- [ ] README updated
- [ ] CHANGELOG updated

---

## Summary

**Approved:** Phase 1 - Simplified Batch Tool

**Timeline:** 2 weeks (1 week implementation + 1 week testing/docs)

**Value:** Convenience - single API call for multiple transactions

**Limitations:** No fee savings, no atomicity, partial execution possible

**Approach:** Simple, honest, well-documented

**Next Step:** Begin implementation of `server/tools_batch.go`

---

**Status:** ✅ READY TO IMPLEMENT
**Date:** 2025-10-20
**Approved By:** User
