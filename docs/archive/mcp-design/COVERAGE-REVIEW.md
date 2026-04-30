# MCP Coverage Review - Gap Analysis

## Executive Summary

This document reviews the complete MCP implementation and design to identify missing functionality and gaps.

**Date:** 2025-10-20
**Reviewer:** Based on comprehensive analysis of implementations and design specs

## Quick Answer: Envelope Support

### Question: Do we include envelope construction and submission?

**API MCP Server (Existing Implementation):**
- ✅ **YES** - Full envelope support implemented
- ✅ Envelope construction via `SubmitEnvelope()` method
- ✅ Multi-transaction envelopes (tested up to 10 transactions)
- ✅ Comprehensive tests (4 envelope integration tests)
- ✅ All transaction tools build and submit in one operation

**Design Spec (docs/mcp/):**
- ⚠️ **PARTIAL** - Mentions submit/validate but incomplete envelope builder tools
- ✅ Has `accumulate_submit_transaction` (submit signed envelope)
- ✅ Has `accumulate_validate_transaction` (validate envelope)
- ❌ Missing: Explicit envelope construction/batching tools
- ❌ Missing: Multi-transaction envelope builder
- ❌ Gap in documentation vs implementation

---

## Complete Gap Analysis

### 1. API MCP Server Tools

#### Implemented (mcp-accumulate repo) - 40 Tools

**Wallet Management (7):**
1. ✅ wallet_init
2. ✅ wallet_vault_open
3. ✅ wallet_vault_lock
4. ✅ wallet_generate_key
5. ✅ wallet_list_keys
6. ✅ wallet_set_network
7. ✅ wallet_get_status

**Query Operations (11):**
8. ✅ accumulate_query_account
9. ✅ accumulate_query_tx
10. ✅ accumulate_query_chain
11. ✅ accumulate_query_data
12. ✅ accumulate_query_directory
13. ✅ accumulate_query_pending
14. ✅ accumulate_query_keybook (EXTRA - not in design)
15. ✅ accumulate_query_keypage (EXTRA - not in design)
16. ✅ accumulate_query_minor_block
17. ✅ accumulate_query_major_block
18. ✅ accumulate_search_public_key

**Transaction Operations (15):**
19. ✅ accumulate_send_tokens
20. ✅ accumulate_create_lite_account (EXTRA - helper)
21. ✅ accumulate_create_adi
22. ✅ accumulate_create_data_account
23. ✅ accumulate_create_token_account
24. ✅ accumulate_create_keypage
25. ✅ accumulate_create_keybook
26. ✅ accumulate_create_token
27. ✅ accumulate_write_data
28. ✅ accumulate_generate_key (EXTRA - helper)
29. ✅ accumulate_add_credits (EXTRA - not in design)
30. ✅ accumulate_update_keypage
31. ✅ accumulate_update_account_auth
32. ✅ accumulate_issue_tokens
33. ✅ accumulate_burn_tokens

**Network & Status (4):**
34. ✅ accumulate_node_info
35. ✅ accumulate_network_status
36. ✅ accumulate_consensus_status
37. ✅ accumulate_metrics

**Search & Faucet (3):**
38. ✅ accumulate_search_public_key_hash
39. ✅ accumulate_search_anchor
40. ✅ accumulate_faucet

#### Designed (docs/mcp/) - 28 Tools

**Network Tools (5):**
1. ✅ accumulate_node_info - IMPLEMENTED
2. ❌ **accumulate_find_service** - MISSING
3. ✅ accumulate_network_status - IMPLEMENTED
4. ✅ accumulate_consensus_status - IMPLEMENTED
5. ✅ accumulate_metrics - IMPLEMENTED

**Query Tools (11):**
6. ✅ accumulate_query_account - IMPLEMENTED
7. ✅ accumulate_query_transaction - IMPLEMENTED (as query_tx)
8. ✅ accumulate_query_chain - IMPLEMENTED
9. ✅ accumulate_query_data - IMPLEMENTED
10. ✅ accumulate_query_directory - IMPLEMENTED
11. ✅ accumulate_search_accounts - IMPLEMENTED (as search_public_key)
12. ✅ accumulate_query_block - IMPLEMENTED (as query_minor/major_block)
13. ✅ accumulate_query_anchors - IMPLEMENTED (as search_anchor)
14. ✅ accumulate_query_key_index - IMPLEMENTED (as search_public_key)
15. ✅ accumulate_query_public_key_hash - IMPLEMENTED (as search_public_key_hash)
16. ❌ **accumulate_query_delegate** - MISSING
17. ✅ accumulate_query_pending - IMPLEMENTED

**Transaction Tools (9):**
18. ❌ **accumulate_submit_transaction** - NOT SEPARATELY IMPLEMENTED (integrated into transaction tools)
19. ❌ **accumulate_validate_transaction** - MISSING
20. ✅ accumulate_faucet - IMPLEMENTED

**Transaction Builders (6):**
21. ✅ accumulate_build_send_tokens - IMPLEMENTED (integrated)
22. ✅ accumulate_build_create_account - IMPLEMENTED (multiple variants)
23. ✅ accumulate_build_update_account - IMPLEMENTED
24. ✅ accumulate_build_write_data - IMPLEMENTED
25. ✅ accumulate_build_token_issuance - IMPLEMENTED (as issue_tokens)
26. ✅ accumulate_build_burn_tokens - IMPLEMENTED (as burn_tokens)

**Event Tools (1):**
27. ❌ **accumulate_subscribe_events** - MISSING

**Snapshot Tools (1):**
28. ❌ **accumulate_list_snapshots** - MISSING

### 2. Critical Gaps Identified

#### Gap #1: Envelope Construction Tools ⚠️ MAJOR

**Problem:** Design spec mentions envelope submission but lacks explicit envelope building tools

**What's Missing:**
- ❌ `accumulate_create_envelope` - Build multi-transaction envelope
- ❌ `accumulate_add_to_envelope` - Add transaction to envelope
- ❌ `accumulate_batch_transactions` - Create batch envelope

**What Exists (Implementation):**
- ✅ Internal `SubmitEnvelope()` in client code
- ✅ Tests show envelope creation works
- ✅ Each transaction tool handles its own envelope

**Impact:** High - Envelope batching is core Accumulate feature

**Recommendation:**
Add explicit envelope tools to design spec:

```markdown
#### Tool: `accumulate_create_envelope`
**Description:** Create a multi-transaction envelope for batch submission

**Parameters:**
- `transactions` (array, required): Array of unsigned transactions
  - Each transaction is JSON object with header and body

**Returns:**
- `envelope`: Unsigned envelope ready for signing
- `required_signers`: Array of authority URLs needed
- `transaction_count`: Number of transactions in envelope

#### Tool: `accumulate_sign_envelope`
**Description:** Sign an envelope with provided keys

**Parameters:**
- `envelope` (object, required): Unsigned envelope
- `signatures` (array, required): Array of signatures
  - `signer`: Authority URL
  - `private_key`: Private key (if not using wallet)
  - `key_name`: Wallet key name (if using wallet)

**Returns:**
- `signed_envelope`: Fully signed envelope
- `submission_ready`: Boolean

#### Tool: `accumulate_submit_envelope`
**Description:** Submit a signed envelope (batched or single)

**Parameters:**
- `envelope` (object, required): Signed envelope
- `check_only` (boolean, optional): Validate without submitting

**Returns:**
- `transaction_hashes`: Array of transaction hashes
- `status`: Submission status
```

#### Gap #2: Separate Submit/Validate Tools ⚠️ MEDIUM

**Problem:** Design spec wants separation between build/submit, implementation integrates them

**Design Philosophy:**
- **Design Spec:** Separate `build_*` and `submit_transaction` for security
- **Implementation:** All-in-one tools (build + sign + submit)

**What's Missing:**
- ❌ Standalone `accumulate_submit_transaction` (takes pre-signed envelope)
- ❌ `accumulate_validate_transaction` (dry-run validation)

**What Exists:**
- ✅ Each transaction tool does build+sign+submit
- ✅ Implementation has internal submission code

**Impact:** Medium - Security model difference

**Recommendation:**
Add to implementation:
- Secure mode flag that disables private key acceptance
- Separate submission tools for pre-signed envelopes
- Keep existing tools for convenience

#### Gap #3: Service Discovery ⚠️ LOW

**What's Missing:**
- ❌ `accumulate_find_service` - DHT-based service discovery

**Impact:** Low - Not commonly used by end users

**Recommendation:** Low priority, add if needed

#### Gap #4: Delegation Queries ⚠️ LOW

**What's Missing:**
- ❌ `accumulate_query_delegate` - Query delegation relationships

**Impact:** Low - Niche use case

**Recommendation:** Add to implementation when delegation features are more widely used

#### Gap #5: Event Subscriptions ⚠️ MEDIUM

**What's Missing:**
- ❌ `accumulate_subscribe_events` - Real-time event streaming

**Impact:** Medium - Useful for monitoring, but requires WebSocket support

**Recommendation:**
- Phase 2 feature
- Requires streaming MCP support
- WebSocket connection management

#### Gap #6: Snapshot Management ⚠️ LOW

**What's Missing:**
- ❌ `accumulate_list_snapshots` - List available snapshots

**Impact:** Low - Administrative feature

**Recommendation:** Low priority

### 3. Database MCP Server

**Status:** ✅ Fully designed (24 tools), ready for implementation

**No gaps identified** - Design is comprehensive for read-only database access

### 4. Resources

#### API MCP Resources

**Implemented:**
- ✅ `wallet://config`
- ✅ `wallet://state`
- ✅ `wallet://keys`

**Designed:**
- ❌ `accumulate://account/{url}` - MISSING
- ❌ `accumulate://transaction/{txid}` - MISSING
- ❌ `accumulate://chain/{url}/{chain}` - MISSING
- ❌ `accumulate://directory/{url}` - MISSING
- ❌ `accumulate://block/{partition}/{height}` - MISSING
- ❌ `accumulate://network/{network}` - MISSING

**Impact:** Medium - Resources provide convenient read access

**Recommendation:** Add `accumulate://` resources to implementation

#### Database MCP Resources

**Designed:**
- database://{session_id}/info
- database://{session_id}/account/{url}
- database://{session_id}/chain/{url}/{chain}
- database://{session_id}/bpt
- database://{session_id}/transaction/{txid}

**Status:** Not yet implemented (database MCP server pending)

### 5. Documentation Gaps

#### Missing Documentation

1. ❌ **Envelope Construction Guide**
   - How to build multi-transaction envelopes
   - Signing requirements
   - Best practices

2. ❌ **Security Model Comparison**
   - Integrated approach (current impl) vs separated approach (design)
   - When to use each
   - Security trade-offs

3. ❌ **Resource Usage Guide**
   - How to use MCP resources
   - Resource URI templates
   - Examples

4. ✅ **Database Server** - Fully documented

#### Documentation That Exists

- ✅ Complete API reference
- ✅ Implementation guide
- ✅ Quick start
- ✅ Database architecture
- ✅ Tool catalog
- ✅ Existing implementation analysis

---

## Priority Matrix

### Critical (Implement Now)

1. **Envelope Construction Tools** 🔴
   - Add explicit envelope builder tools to design
   - Document current SubmitEnvelope() implementation
   - Create MCP tools for batch operations

2. **accumulate:// Resources** 🔴
   - Implement 6 resource types from design spec
   - Add to existing implementation
   - Test with Claude Desktop

### High Priority (Next Release)

3. **Separate Submit Tool** 🟡
   - Add standalone submit_transaction tool
   - Support pre-signed envelopes
   - Enable secure mode (no private keys)

4. **Validate Transaction Tool** 🟡
   - Add dry-run validation
   - Return estimated fees
   - Check authorization

5. **Event Subscriptions** 🟡
   - WebSocket support
   - Real-time monitoring
   - Event filtering

### Medium Priority (Future)

6. **Service Discovery** 🟢
   - accumulate_find_service
   - DHT queries
   - Service routing

7. **Delegation Queries** 🟢
   - accumulate_query_delegate
   - Authority chains
   - Delegation tracking

8. **Snapshot Management** 🟢
   - accumulate_list_snapshots
   - Snapshot metadata
   - Version management

### Low Priority (Nice to Have)

9. **Advanced Analytics** 🔵
   - Transaction flow analysis
   - Account activity patterns
   - Network health metrics

---

## Recommendations by Component

### For API MCP Server (mcp-accumulate)

#### Immediate Actions

1. **Add Envelope Tools (Critical)**
   ```
   New Tools:
   - accumulate_create_envelope (batch builder)
   - accumulate_submit_envelope (submit batched txns)
   - accumulate_validate_envelope (dry run)
   ```

2. **Implement accumulate:// Resources (Critical)**
   ```
   Add Resources:
   - accumulate://account/{url}
   - accumulate://transaction/{txid}
   - accumulate://chain/{url}/{chain}
   - accumulate://directory/{url}
   - accumulate://block/{partition}/{height}
   - accumulate://network/{network}
   ```

3. **Add Missing Query Tools (High)**
   ```
   New Tools:
   - accumulate_find_service (DHT discovery)
   - accumulate_query_delegate (delegation chains)
   ```

4. **Document Envelope Usage (High)**
   ```
   New Docs:
   - Envelope construction guide
   - Batch transaction examples
   - Multi-signer workflows
   ```

#### Future Enhancements

5. **Secure Mode (Medium)**
   - Flag to disable private key parameters
   - Force external signing
   - Pre-signed envelope submission only

6. **Event Subscriptions (Medium)**
   - WebSocket client
   - Event streaming
   - Subscription management

### For Design Spec (docs/mcp/)

#### Updates Needed

1. **Add Envelope Tools to Spec (Critical)**
   - Document create_envelope tool
   - Document submit_envelope tool
   - Document validate_envelope tool
   - Add envelope construction section

2. **Reconcile with Implementation (High)**
   - Note tools that are integrated (build+submit)
   - Document implementation choices
   - Explain security model trade-offs

3. **Add Resource Examples (Medium)**
   - Show resource URI usage
   - Provide code examples
   - Integration with tools

### For Database MCP Server

**Status:** ✅ Design complete, no gaps

**Action:** Begin implementation

---

## Envelope Support - Detailed Analysis

### Current State (mcp-accumulate)

**Envelope Support: ✅ COMPREHENSIVE**

#### What Works

1. **SubmitEnvelope() Method**
   ```go
   func (c *Client) SubmitEnvelope(ctx context.Context,
       transactions []*protocol.Transaction,
       signatures []protocol.Signature) ([][]byte, error)
   ```

2. **Multi-Transaction Envelopes**
   - Tested with 3 transactions ✅
   - Tested with 10 transactions ✅
   - Multiple principals supported ✅
   - Different transaction types in same envelope ✅

3. **Integration Tests**
   - TestDevnetMultipleTokenSendsInEnvelope ✅
   - TestDevnetBatchTokenOperations ✅
   - TestDevnetEnvelopeWithMultipleRecipients ✅
   - TestDevnetEnvelopeLargeScale ✅

4. **Use Cases Validated**
   - Batch payments ✅
   - Multi-account operations ✅
   - Atomic execution ✅
   - Gas optimization ✅

#### What's Hidden

**Problem:** Envelope functionality exists but isn't exposed as MCP tools!

Current approach:
```
User → accumulate_send_tokens →
  Internal: Build txn + Sign + Create envelope + Submit
```

What users can't do via MCP:
```
User → accumulate_create_envelope([txn1, txn2, txn3]) →
  Get unsigned envelope →
    Sign externally →
      accumulate_submit_envelope(signed_envelope)
```

### What's Missing from MCP Tools

#### Missing Tool: `accumulate_create_batch`

```typescript
{
  "name": "accumulate_create_batch",
  "description": "Create a batch envelope containing multiple transactions",
  "inputSchema": {
    "type": "object",
    "properties": {
      "transactions": {
        "type": "array",
        "description": "Array of transaction specifications",
        "items": {
          "type": "object",
          "properties": {
            "type": {
              "type": "string",
              "enum": ["sendTokens", "createAccount", "writeData", "updateKeyPage"]
            },
            "params": {
              "type": "object",
              "description": "Transaction-specific parameters"
            }
          }
        }
      }
    }
  }
}
```

Example usage:
```json
{
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
    },
    {
      "type": "writeData",
      "params": {
        "account": "acc://alice.acme/data",
        "entries": ["Payment batch 2024-10-20"]
      }
    }
  ]
}
```

Returns:
```json
{
  "envelope": {...},
  "transaction_count": 3,
  "required_signers": [
    "acc://alice.acme/book/1"
  ],
  "total_fee_estimate": "0.03"
}
```

---

## Summary of Gaps

### Critical Gaps (Must Fix)

| Gap | Component | Impact | Effort |
|-----|-----------|--------|--------|
| Envelope tools | API MCP | High | Medium |
| accumulate:// resources | API MCP | High | Low |
| Envelope documentation | Docs | High | Low |

### High Priority (Should Fix)

| Gap | Component | Impact | Effort |
|-----|-----------|--------|--------|
| Standalone submit tool | API MCP | Medium | Low |
| Validate tool | API MCP | Medium | Low |
| find_service tool | API MCP | Low | Low |
| query_delegate tool | API MCP | Low | Low |

### Medium Priority (Nice to Have)

| Gap | Component | Impact | Effort |
|-----|-----------|--------|--------|
| Event subscriptions | API MCP | Medium | High |
| Secure mode | API MCP | Medium | Medium |
| Snapshot tools | API MCP | Low | Low |

### Implementation Complete (No Gaps)

- ✅ Database MCP design
- ✅ Core query tools
- ✅ Transaction tools (integrated approach)
- ✅ Wallet integration
- ✅ Network status tools

---

## Action Items

### Phase 1: Critical Fixes (Week 1)

- [ ] Add `accumulate_create_batch` tool to mcp-accumulate
- [ ] Add `accumulate_submit_envelope` tool (standalone)
- [ ] Add `accumulate_validate_envelope` tool
- [ ] Implement all 6 `accumulate://` resources
- [ ] Update design spec with envelope tools section
- [ ] Write envelope construction guide

### Phase 2: High Priority (Week 2-3)

- [ ] Add `accumulate_find_service` tool
- [ ] Add `accumulate_query_delegate` tool
- [ ] Add secure mode configuration option
- [ ] Update documentation with security model comparison
- [ ] Add resource usage examples to docs

### Phase 3: Future Enhancements

- [ ] Event subscription support (WebSocket)
- [ ] Snapshot management tools
- [ ] Advanced analytics tools
- [ ] Database MCP implementation

---

## Conclusion

### Overall Assessment

**Implementation: 8/10**
- 40 tools working in production
- Envelope support exists internally
- Comprehensive transaction coverage
- Excellent wallet integration

**Design Spec: 7/10**
- 28 tools designed
- Database MCP fully specified
- Missing envelope batching docs
- Resources not fully detailed

**Gap Severity: MEDIUM**
- Core functionality present
- Envelope support hidden (not exposed as tools)
- Missing some convenience features
- Documentation could be clearer

### Key Finding: Envelope Support

**YES - We DO have envelope construction and submission!**

But it's:
- ✅ Implemented internally in mcp-accumulate
- ✅ Tested comprehensively
- ✅ Working in production
- ❌ Not exposed as explicit MCP tools
- ❌ Not documented in design spec
- ❌ Not available for manual envelope creation

### Main Recommendation

**Expose existing envelope functionality as MCP tools:**

1. `accumulate_create_batch` - Build multi-transaction envelope
2. `accumulate_submit_envelope` - Submit pre-built envelope
3. `accumulate_validate_envelope` - Validate before submission

This gives users both:
- **Convenience tools** (current: send_tokens, create_adi, etc.)
- **Power tools** (new: create_batch, submit_envelope, etc.)

---

**Version:** 1.0
**Date:** 2025-10-20
**Status:** Complete Analysis
