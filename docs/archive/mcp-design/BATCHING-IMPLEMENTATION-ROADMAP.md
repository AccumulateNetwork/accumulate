# Implementation Roadmap: Envelope Batching Tools

## Overview

This document provides a practical roadmap for adding envelope batching capabilities to the existing `mcp-accumulate` implementation.

## Current State

**Existing Implementation (mcp-accumulate):**
- ✅ 40 tools working
- ✅ Full SDK integration
- ✅ Wallet support
- ✅ Individual transaction tools (build+sign+submit)
- ✅ Internal `SubmitEnvelope()` method
- ❌ No user-facing batch tools

**Gap:**
Users can't easily submit multiple transactions in one API call via MCP tools.

**Value Proposition:**
Convenience - reduce API calls, simplify client code, better organization.

## Implementation Strategy

### Phase 1: Quick Win - Simplified Batch Tool (1 week)

Add one simple tool that handles batching in a single call.

#### Tool: `accumulate_submit_batch`

**File:** `server/tools_batch.go` (new)

```go
package server

func (s *Server) handleSubmitBatch(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    var params struct {
        Transactions []struct {
            Type   string                 `json:"type"`
            Params map[string]interface{} `json:"params"`
        } `json:"transactions"`
        SigningMethod string `json:"signing_method"` // "wallet" or "manual"
        KeyName       string `json:"key_name"`       // if wallet
        Password      string `json:"password"`       // if wallet
    }

    if err := json.Unmarshal(request.Params.Arguments, &params); err != nil {
        return nil, err
    }

    // 1. Build all transactions
    var txns []*protocol.Transaction
    var sigs []protocol.Signature

    for _, txSpec := range params.Transactions {
        // Build transaction based on type
        txn, sig, err := s.buildTransaction(ctx, txSpec.Type, txSpec.Params, params.KeyName, params.Password)
        if err != nil {
            return nil, fmt.Errorf("failed to build transaction %s: %w", txSpec.Type, err)
        }
        txns = append(txns, txn)
        sigs = append(sigs, sig)
    }

    // 2. Submit as batch using existing SubmitEnvelope
    hashes, err := s.client.SubmitEnvelope(ctx, txns, sigs)
    if err != nil {
        return nil, fmt.Errorf("failed to submit batch: %w", err)
    }

    // 3. Return results
    result := map[string]interface{}{
        "transaction_count": len(txns),
        "transaction_hashes": hashes,
        "status": "submitted",
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
    switch txType {
    case "sendTokens":
        return s.buildSendTokens(ctx, params, keyName, password)
    case "createIdentity":
        return s.buildCreateIdentity(ctx, params, keyName, password)
    case "writeData":
        return s.buildWriteData(ctx, params, keyName, password)
    // ... add other transaction types
    default:
        return nil, nil, fmt.Errorf("unsupported transaction type: %s", txType)
    }
}
```

**Tool Definition:** `server/tool_definitions.go`

```go
{
    "name": "accumulate_submit_batch",
    "description": "Submit multiple transactions in one batch for atomic execution and convenience",
    "inputSchema": map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "transactions": map[string]interface{}{
                "type": "array",
                "description": "Array of transactions to batch",
                "items": map[string]interface{}{
                    "type": "object",
                    "properties": map[string]interface{}{
                        "type": map[string]interface{}{
                            "type": "string",
                            "description": "Transaction type (sendTokens, createIdentity, etc.)",
                        },
                        "params": map[string]interface{}{
                            "type": "object",
                            "description": "Transaction-specific parameters",
                        },
                    },
                    "required": []string{"type", "params"},
                },
            },
            "signing_method": map[string]interface{}{
                "type": "string",
                "enum": []string{"wallet"},
                "default": "wallet",
            },
            "key_name": map[string]interface{}{
                "type": "string",
                "description": "Wallet key name to sign with",
            },
            "password": map[string]interface{}{
                "type": "string",
                "description": "Vault password",
            },
        },
        "required": []string{"transactions", "key_name", "password"},
    },
},
```

**Test:** `server/tools_batch_test.go`

```go
func TestSubmitBatch(t *testing.T) {
    srv := NewTestServer(t)

    request := map[string]interface{}{
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

    result, err := srv.handleSubmitBatch(context.Background(), request)
    require.NoError(t, err)
    require.NotNil(t, result)

    // Verify 2 transaction hashes returned
    var data map[string]interface{}
    json.Unmarshal([]byte(result.Content[0].Text), &data)
    require.Equal(t, 2, data["transaction_count"])
    require.Len(t, data["transaction_hashes"], 2)
}
```

**Effort:** 1 week
**Impact:** High - Users can immediately batch transactions
**Complexity:** Low - Reuses existing code

---

### Phase 2: Stateful Batch Tools (2-3 weeks)

Add full batch management with state.

#### Step 1: Batch State Management

**File:** `server/batch.go` (new)

```go
package server

import (
    "sync"
    "time"
)

type Batch struct {
    ID            string
    Description   string
    Network       string
    Transactions  []*TransactionSpec
    Signatures    []protocol.Signature
    CreatedAt     time.Time
    SignedAt      *time.Time
    SubmittedAt   *time.Time
}

type TransactionSpec struct {
    Type   string
    Params map[string]interface{}
}

type BatchManager struct {
    batches map[string]*Batch
    mu      sync.RWMutex
    timeout time.Duration
}

func NewBatchManager() *BatchManager {
    return &BatchManager{
        batches: make(map[string]*Batch),
        timeout: 1 * time.Hour,
    }
}

func (m *BatchManager) Create(description, network string) *Batch {
    m.mu.Lock()
    defer m.mu.Unlock()

    batch := &Batch{
        ID:          generateBatchID(),
        Description: description,
        Network:     network,
        CreatedAt:   time.Now(),
    }

    m.batches[batch.ID] = batch

    // Start cleanup timer
    go m.cleanupAfter(batch.ID, m.timeout)

    return batch
}

func (m *BatchManager) Get(id string) (*Batch, error) {
    m.mu.RLock()
    defer m.mu.RUnlock()

    batch, ok := m.batches[id]
    if !ok {
        return nil, fmt.Errorf("batch not found: %s", id)
    }

    return batch, nil
}

func (m *BatchManager) AddTransaction(id string, spec TransactionSpec) error {
    m.mu.Lock()
    defer m.mu.Unlock()

    batch, ok := m.batches[id]
    if !ok {
        return fmt.Errorf("batch not found: %s", id)
    }

    if batch.SubmittedAt != nil {
        return fmt.Errorf("batch already submitted")
    }

    batch.Transactions = append(batch.Transactions, &spec)
    return nil
}

func (m *BatchManager) Sign(id string, signatures []protocol.Signature) error {
    m.mu.Lock()
    defer m.mu.Unlock()

    batch, ok := m.batches[id]
    if !ok {
        return fmt.Errorf("batch not found: %s", id)
    }

    batch.Signatures = signatures
    now := time.Now()
    batch.SignedAt = &now
    return nil
}

func (m *BatchManager) Delete(id string) {
    m.mu.Lock()
    defer m.mu.Unlock()
    delete(m.batches, id)
}

func (m *BatchManager) cleanupAfter(id string, duration time.Duration) {
    time.Sleep(duration)
    m.Delete(id)
}

func generateBatchID() string {
    return fmt.Sprintf("batch_%d", time.Now().UnixNano())
}
```

#### Step 2: Add Batch Tools

**File:** `server/tools_batch_stateful.go` (new)

```go
package server

func (s *Server) handleBatchCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    var params struct {
        Description string `json:"description"`
        Network     string `json:"network"`
    }

    if err := json.Unmarshal(request.Params.Arguments, &params); err != nil {
        return nil, err
    }

    batch := s.batchMgr.Create(params.Description, params.Network)

    result := map[string]interface{}{
        "batch_id": batch.ID,
        "transaction_count": 0,
        "created_at": batch.CreatedAt,
    }

    return marshalResult(result)
}

func (s *Server) handleBatchAddTransaction(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    var params struct {
        BatchID         string                 `json:"batch_id"`
        TransactionType string                 `json:"transaction_type"`
        Params          map[string]interface{} `json:"params"`
    }

    if err := json.Unmarshal(request.Params.Arguments, &params); err != nil {
        return nil, err
    }

    spec := TransactionSpec{
        Type:   params.TransactionType,
        Params: params.Params,
    }

    if err := s.batchMgr.AddTransaction(params.BatchID, spec); err != nil {
        return nil, err
    }

    batch, _ := s.batchMgr.Get(params.BatchID)

    result := map[string]interface{}{
        "batch_id": batch.ID,
        "transaction_count": len(batch.Transactions),
        "transaction_index": len(batch.Transactions) - 1,
    }

    return marshalResult(result)
}

// Add handlers for:
// - handleBatchInfo
// - handleBatchSign
// - handleBatchSubmit
// - handleBatchCancel
// - handleBatchExport
```

#### Step 3: Update Server Initialization

**File:** `server/server.go`

```go
type Server struct {
    name     string
    version  string
    state    *State
    wallet   *wallet.Client
    batchMgr *BatchManager  // NEW
}

func NewServer() *Server {
    // ... existing code ...

    return &Server{
        name:     "mcp-accumulate",
        version:  "0.3.0",  // Bump version
        state:    state,
        wallet:   walletClient,
        batchMgr: NewBatchManager(),  // NEW
    }
}
```

**Effort:** 2-3 weeks
**Impact:** High - Full batch workflow support
**Complexity:** Medium - State management

---

### Phase 3: Advanced Features (Future)

#### 3.1 Batch Templates

Pre-configured batch templates for common operations:

```go
templates := map[string]BatchTemplate{
    "payroll": {
        Description: "Monthly payroll",
        Transactions: []TransactionTemplate{
            {Type: "sendTokens", Fields: []string{"to", "amount"}},
            {Type: "sendTokens", Fields: []string{"to", "amount"}},
            {Type: "writeData", Fields: []string{"data"}},
        },
    },
    "account_setup": {
        Description: "New user account setup",
        Transactions: []TransactionTemplate{
            {Type: "createIdentity", Fields: []string{"url", "publicKey"}},
            {Type: "createTokenAccount", Fields: []string{"url"}},
            {Type: "createDataAccount", Fields: []string{"url"}},
        },
    },
}
```

#### 3.2 Batch Scheduling

Schedule batches for future execution:

```go
func (s *Server) handleBatchSchedule(ctx context.Context, request mcp.CallToolRequest) {
    var params struct {
        BatchID   string    `json:"batch_id"`
        ExecuteAt time.Time `json:"execute_at"`
    }

    // Schedule batch submission
    go func() {
        time.Sleep(time.Until(params.ExecuteAt))
        s.handleBatchSubmit(ctx, ...)
    }()
}
```

#### 3.3 Batch Analytics

Track batch usage statistics:

```go
type BatchStats struct {
    TotalBatches        int
    TotalTransactions   int
    AverageBatchSize    float64
    AtomicityBenefits   int  // Batches that needed atomicity
}

func (s *Server) handleBatchStats(ctx context.Context) (*BatchStats, error) {
    // Return batch usage statistics
}
```

---

## Testing Strategy

### Unit Tests

```bash
# Test batch manager
go test ./server -run TestBatchManager

# Test individual tools
go test ./server -run TestBatchCreate
go test ./server -run TestBatchAddTransaction
go test ./server -run TestBatchSubmit
```

### Integration Tests

```bash
# Test against devnet
go test ./server -tags=integration -run TestBatchIntegration

# Test with wallet
go test ./server -tags=integration -run TestBatchWithWallet
```

### User Acceptance Tests

```bash
# Manual testing with Claude Desktop
1. Create batch via MCP
2. Add 3 transactions
3. Review batch
4. Sign and submit
5. Verify on explorer
```

---

## Documentation Updates

### 1. Update README.md

Add section on batching:

```markdown
## Batching Transactions

Submit multiple transactions in one API call for convenience:

### Quick Example
```
> "Send 10 ACME to Alice, 20 to Bob, and 15 to Charlie"

AI creates batch with 3 payments, submits in one call!
```

### Available Tools
- `accumulate_submit_batch` - Simple one-call batching
- `accumulate_batch_create` - Start a batch
- `accumulate_batch_add_transaction` - Add to batch
- `accumulate_batch_submit` - Submit batch

See [BATCHING-USER-GUIDE.md](docs/BATCHING-USER-GUIDE.md) for details.
```

### 2. Add Examples

Create `examples/batching/` directory with:

- `payroll.js` - Payroll example
- `account_setup.js` - Account creation
- `atomic_payment.js` - Payment with receipt

### 3. Update Tool Catalog

Add batching section to tool list in README.

---

## Migration Path

### For Existing Users

**No breaking changes** - All existing tools continue to work:

```
Before (still works):
> accumulate_send_tokens {...}
> accumulate_send_tokens {...}
> accumulate_send_tokens {...}

After (new option):
> accumulate_submit_batch {
    transactions: [
      {type: "sendTokens", ...},
      {type: "sendTokens", ...},
      {type: "sendTokens", ...}
    ]
  }
```

### For AI Assistants

AI can detect batch opportunities:

```python
def should_batch(operations):
    if len(operations) >= 3:
        return True
    if all_same_type(operations) and all_same_signer(operations):
        return True
    return False
```

---

## Performance Considerations

### Memory

Each batch stores:
- Transaction specs: ~1KB each
- Signatures: ~100 bytes each
- Metadata: ~500 bytes

**100 active batches × 10 txns each = ~1.5MB**

### Cleanup

Auto-cleanup after 1 hour:
- Prevents memory leaks
- User can re-create if needed
- Submitted batches deleted immediately

### Concurrency

Batch manager uses read-write locks:
- Multiple reads allowed
- Writes are serialized
- No race conditions

---

## Rollout Plan

### Week 1: Development
- Implement `accumulate_submit_batch` tool
- Add tool definition
- Write unit tests

### Week 2: Testing
- Integration tests against devnet
- Manual testing with Claude Desktop
- Fix bugs

### Week 3: Documentation
- Update README
- Create user guide
- Add examples

### Week 4: Release
- Merge to main
- Tag version 0.3.0
- Announce to users

### Future: Stateful Tools
- Weeks 5-7: Implement full batch manager
- Week 8: Testing and refinement
- Week 9: Release v0.4.0

---

## Success Metrics

### Technical
- ✅ All batch tests passing
- ✅ Integration tests with devnet passing
- ✅ No memory leaks
- ✅ Performance acceptable (<100ms per batch operation)

### User
- ✅ At least 10% of multi-transaction scenarios use batching
- ✅ Average batch size: 3-5 transactions
- ✅ Reduced API calls (convenience metric)
- ✅ User satisfaction: Positive feedback on ease of use

### AI Assistant
- ✅ Claude successfully creates batches
- ✅ Automatic batch detection working
- ✅ Batch explanations clear to users

---

## Risk Mitigation

### Risk 1: Batch State Loss

**Mitigation:**
- Persist batches to disk (optional)
- Provide export functionality
- Clear timeout warnings

### Risk 2: Complex Signatures

**Mitigation:**
- Start with single-signer batches
- Add multi-signer support later
- Clear error messages

### Risk 3: User Confusion

**Mitigation:**
- Excellent documentation
- Clear examples
- AI assistant patterns

---

## Summary

### Phase 1 (Quick Win) - 1 Week
**Tool:** `accumulate_submit_batch`
- Simple one-call batching
- Reuses existing code
- Immediate value to users

### Phase 2 (Full Feature) - 2-3 Weeks
**Tools:** Full batch management (7 tools)
- Stateful batch creation
- Review before submit
- Export capability

### Phase 3 (Future) - TBD
- Templates
- Scheduling
- Analytics

### Total Effort
- Minimum: 1 week (simplified tool)
- Recommended: 4 weeks (simplified + stateful)
- Complete: 8 weeks (all features)

### Recommendation

**Start with Phase 1** (simplified tool):
- Quick to implement
- Immediate user value
- Validates approach
- Low risk

Then add Phase 2 if usage is high.

---

## Next Steps

1. **Review this roadmap** with team
2. **Create GitLab issue** for Phase 1
3. **Assign developer** to implement
4. **Set milestone** for v0.3.0
5. **Begin implementation**

---

**Status:** Ready for Implementation
**Priority:** High
**Complexity:** Low (Phase 1) / Medium (Phase 2)
**Value:** High
