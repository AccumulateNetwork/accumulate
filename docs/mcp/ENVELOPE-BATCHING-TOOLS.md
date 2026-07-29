# MCP Tools for Envelope Batching

## Overview

This document specifies MCP tools to help users batch multiple transactions into envelopes for efficient submission to the Accumulate network.

## Why Batch Transactions?

**Primary Benefit: Convenience**

Batching allows you to submit multiple transactions in a single API call, simplifying your workflow:

- ⚡ **Single API Call** - Submit 3 transactions with one request instead of three
- 📦 **Logical Grouping** - Organize related operations together (e.g., "October Payroll")
- 🔧 **Simpler Code** - Less HTTP overhead, easier error handling
- 📋 **Better Organization** - Track related transactions as a batch

**Important Limitations:**

- ❌ **No Fee Savings** - Each transaction is charged individually (3 transactions = 3 fees)
- ❌ **No Atomicity** - Transactions execute independently (partial execution is possible)
- ⚠️ **Partial Failures** - If transaction 2 fails, transactions 1 and 3 may still succeed

**Use Cases:**
- Payroll submissions (all payments in one call)
- Account setup workflows (multiple operations together)
- Bulk updates (many changes in one submission)
- Related operations (payment + audit log)

## Proposed MCP Tools

### Tool 1: `accumulate_batch_create`

**Description:** Create a new batch envelope for multiple transactions

**Parameters:**
```typescript
{
  "description": string,  // Optional: Description of batch purpose
  "network": string       // Optional: "mainnet", "testnet", or custom URL
}
```

**Returns:**
```typescript
{
  "batch_id": string,          // Unique batch identifier
  "transaction_count": 0,      // Number of transactions (starts at 0)
  "created_at": timestamp
}
```

**Example:**
```json
{
  "description": "Monthly payroll batch",
  "network": "mainnet"
}
```

**Response:**
```json
{
  "batch_id": "batch_abc123",
  "transaction_count": 0,
  "created_at": "2024-10-20T12:00:00Z"
}
```

---

### Tool 2: `accumulate_batch_add_transaction`

**Description:** Add a transaction to an existing batch

**Parameters:**
```typescript
{
  "batch_id": string,         // Required: Batch identifier
  "transaction_type": string, // Required: Transaction type
  "params": object           // Required: Transaction-specific parameters
}
```

**Supported Transaction Types:**
- `"sendTokens"` - Send tokens
- `"createIdentity"` - Create ADI
- `"createTokenAccount"` - Create token account
- `"createDataAccount"` - Create data account
- `"writeData"` - Write data entry
- `"updateKeyPage"` - Update key page
- `"issueTokens"` - Issue tokens
- `"burnTokens"` - Burn tokens
- And 60+ other types

**Returns:**
```typescript
{
  "batch_id": string,
  "transaction_count": number,
  "transaction_index": number,  // Index of added transaction
  "required_signers": string[]  // Updated list of required authorities
}
```

**Example - Add Token Send:**
```json
{
  "batch_id": "batch_abc123",
  "transaction_type": "sendTokens",
  "params": {
    "from": "acc://company.acme/payroll",
    "to": "acc://employee1.acme/tokens",
    "amount": "5000000000"
  }
}
```

**Response:**
```json
{
  "batch_id": "batch_abc123",
  "transaction_count": 1,
  "transaction_index": 0,
  "required_signers": ["acc://company.acme/book/1"]
}
```

**Example - Add Data Write:**
```json
{
  "batch_id": "batch_abc123",
  "transaction_type": "writeData",
  "params": {
    "account": "acc://company.acme/audit",
    "data": "Payroll batch 2024-10-20"
  }
}
```

---

### Tool 3: `accumulate_batch_info`

**Description:** Get information about a batch

**Parameters:**
```typescript
{
  "batch_id": string  // Required: Batch identifier
}
```

**Returns:**
```typescript
{
  "batch_id": string,
  "description": string,
  "transaction_count": number,
  "transactions": Array<{
    "index": number,
    "type": string,
    "principal": string,
    "summary": string
  }>,
  "required_signers": string[],
  "estimated_fee": string,
  "ready_to_submit": boolean
}
```

**Example:**
```json
{
  "batch_id": "batch_abc123"
}
```

**Response:**
```json
{
  "batch_id": "batch_abc123",
  "description": "Monthly payroll batch",
  "transaction_count": 3,
  "transactions": [
    {
      "index": 0,
      "type": "sendTokens",
      "principal": "acc://company.acme/payroll",
      "summary": "Send 50 ACME to employee1"
    },
    {
      "index": 1,
      "type": "sendTokens",
      "principal": "acc://company.acme/payroll",
      "summary": "Send 60 ACME to employee2"
    },
    {
      "index": 2,
      "type": "writeData",
      "principal": "acc://company.acme/audit",
      "summary": "Write audit log entry"
    }
  ],
  "required_signers": [
    "acc://company.acme/book/1"
  ],
  "estimated_fee": "0.03",
  "ready_to_submit": false
}
```

---

### Tool 4: `accumulate_batch_sign`

**Description:** Sign a batch with wallet keys or provided signatures

**Parameters:**
```typescript
{
  "batch_id": string,           // Required: Batch identifier
  "signing_method": "wallet" | "manual",

  // If using wallet:
  "key_name": string,           // Wallet key name
  "password": string,           // Vault password

  // If using manual:
  "signatures": Array<{
    "signer": string,           // Authority URL
    "public_key": string,       // Public key hex
    "signature": string,        // Signature hex
    "timestamp": number         // Timestamp (nanoseconds)
  }>
}
```

**Returns:**
```typescript
{
  "batch_id": string,
  "signed": boolean,
  "signatures_count": number,
  "ready_to_submit": boolean
}
```

**Example - Wallet Signing:**
```json
{
  "batch_id": "batch_abc123",
  "signing_method": "wallet",
  "key_name": "company-key",
  "password": "vault-password"
}
```

**Example - Manual Signing:**
```json
{
  "batch_id": "batch_abc123",
  "signing_method": "manual",
  "signatures": [
    {
      "signer": "acc://company.acme/book/1",
      "public_key": "0123456789abcdef...",
      "signature": "fedcba9876543210...",
      "timestamp": 1697123456000000000
    }
  ]
}
```

---

### Tool 5: `accumulate_batch_submit`

**Description:** Submit a signed batch to the network

**Parameters:**
```typescript
{
  "batch_id": string,      // Required: Batch identifier
  "wait": boolean,         // Optional: Wait for confirmation (default: false)
  "dry_run": boolean       // Optional: Validate without submitting (default: false)
}
```

**Returns:**
```typescript
{
  "batch_id": string,
  "submitted": boolean,
  "transaction_hashes": string[],
  "status": "pending" | "confirmed" | "failed",
  "confirmation": {        // If wait=true
    "block_height": number,
    "timestamp": string
  }
}
```

**Example:**
```json
{
  "batch_id": "batch_abc123",
  "wait": true
}
```

**Response:**
```json
{
  "batch_id": "batch_abc123",
  "submitted": true,
  "transaction_hashes": [
    "abc123...",
    "def456...",
    "789abc..."
  ],
  "status": "confirmed",
  "confirmation": {
    "block_height": 1234567,
    "timestamp": "2024-10-20T12:05:00Z"
  }
}
```

---

### Tool 6: `accumulate_batch_cancel`

**Description:** Cancel a batch before submission

**Parameters:**
```typescript
{
  "batch_id": string  // Required: Batch identifier
}
```

**Returns:**
```typescript
{
  "batch_id": string,
  "cancelled": boolean
}
```

---

### Tool 7: `accumulate_batch_export`

**Description:** Export batch as JSON envelope (for manual submission)

**Parameters:**
```typescript
{
  "batch_id": string,     // Required: Batch identifier
  "format": "envelope" | "transactions" | "human_readable"
}
```

**Returns:**
```typescript
{
  "batch_id": string,
  "format": string,
  "data": object | string
}
```

**Example - Export as Envelope:**
```json
{
  "batch_id": "batch_abc123",
  "format": "envelope"
}
```

**Response:**
```json
{
  "batch_id": "batch_abc123",
  "format": "envelope",
  "data": {
    "transaction": [
      {
        "header": {
          "principal": "acc://company.acme/payroll"
        },
        "body": {
          "type": "sendTokens",
          "to": [{"url": "acc://employee1.acme/tokens", "amount": "5000000000"}]
        }
      }
    ],
    "signatures": [
      {
        "type": "ed25519",
        "publicKey": "...",
        "signature": "...",
        "signer": "acc://company.acme/book/1",
        "signerVersion": 1,
        "timestamp": 1697123456000000000
      }
    ]
  }
}
```

---

## Complete Workflow Example

### Use Case: Monthly Payroll (3 employees)

```
Step 1: Create Batch
→ Tool: accumulate_batch_create
  Input: {"description": "October payroll"}
  Output: {"batch_id": "batch_oct2024"}

Step 2: Add Payment #1
→ Tool: accumulate_batch_add_transaction
  Input: {
    "batch_id": "batch_oct2024",
    "transaction_type": "sendTokens",
    "params": {
      "from": "acc://company.acme/payroll",
      "to": "acc://alice.acme/tokens",
      "amount": "5000000000"
    }
  }
  Output: {"transaction_count": 1, "transaction_index": 0}

Step 3: Add Payment #2
→ Tool: accumulate_batch_add_transaction
  Input: {
    "batch_id": "batch_oct2024",
    "transaction_type": "sendTokens",
    "params": {
      "from": "acc://company.acme/payroll",
      "to": "acc://bob.acme/tokens",
      "amount": "6000000000"
    }
  }
  Output: {"transaction_count": 2, "transaction_index": 1}

Step 4: Add Payment #3
→ Tool: accumulate_batch_add_transaction
  Input: {
    "batch_id": "batch_oct2024",
    "transaction_type": "sendTokens",
    "params": {
      "from": "acc://company.acme/payroll",
      "to": "acc://charlie.acme/tokens",
      "amount": "5500000000"
    }
  }
  Output: {"transaction_count": 3, "transaction_index": 2}

Step 5: Review Batch
→ Tool: accumulate_batch_info
  Input: {"batch_id": "batch_oct2024"}
  Output: {
    "transaction_count": 3,
    "required_signers": ["acc://company.acme/book/1"],
    "estimated_fee": "0.03"
  }

Step 6: Sign Batch
→ Tool: accumulate_batch_sign
  Input: {
    "batch_id": "batch_oct2024",
    "signing_method": "wallet",
    "key_name": "company-key",
    "password": "***"
  }
  Output: {"signed": true, "ready_to_submit": true}

Step 7: Submit Batch
→ Tool: accumulate_batch_submit
  Input: {
    "batch_id": "batch_oct2024",
    "wait": true
  }
  Output: {
    "submitted": true,
    "transaction_hashes": ["abc...", "def...", "ghi..."],
    "status": "confirmed"
  }

Result: ✅ 3 payments submitted atomically in 1 envelope!
```

---

## AI Assistant Conversation Example

```
User: "I need to pay 3 employees: Alice 50 ACME, Bob 60 ACME, Charlie 55 ACME"

AI: "I'll create a batch transaction for all 3 payments to save on fees."

[AI calls: accumulate_batch_create]
AI: "Created batch: batch_oct2024"

[AI calls: accumulate_batch_add_transaction x3]
AI: "Added all 3 payments:
     - Alice: 50 ACME
     - Bob: 60 ACME
     - Charlie: 55 ACME

     Total: 165 ACME
     Fees: 3 transactions × $0.03 = $0.09 total

     Ready to sign with acc://company.acme/book/1"

User: "Sign it with key 'company-key'"

[AI calls: accumulate_batch_sign]
AI: "Batch signed successfully!"

[AI calls: accumulate_batch_submit]
AI: "✅ All payments submitted!
     Transaction hashes:
     - abc123... (Alice)
     - def456... (Bob)
     - ghi789... (Charlie)

     All confirmed in block 1234567"
```

---

## Implementation Requirements

### Server-Side State Management

The MCP server needs to maintain batch state:

```typescript
interface Batch {
  id: string;
  description: string;
  network: string;
  transactions: Transaction[];
  signatures: Signature[];
  createdAt: Date;
  signedAt?: Date;
  submittedAt?: Date;
}

class BatchManager {
  private batches: Map<string, Batch>;

  createBatch(description: string): Batch;
  addTransaction(batchId: string, txn: Transaction): void;
  sign(batchId: string, signatures: Signature[]): void;
  submit(batchId: string): Promise<SubmitResult>;
  export(batchId: string): Envelope;
}
```

### Integration with Existing Tools

These batch tools complement existing single-transaction tools:

**Single Transaction (Current):**
```
accumulate_send_tokens → builds + signs + submits one transaction
```

**Batch Transactions (New):**
```
accumulate_batch_create → initialize batch
accumulate_batch_add_transaction → add to batch
accumulate_batch_sign → sign all at once
accumulate_batch_submit → submit all at once
```

Users can choose based on needs:
- **Single operation:** Use existing tools (simpler)
- **Multiple operations:** Use batch tools (more efficient)

---

## Alternative: Simplified Approach

If state management is too complex, provide a simpler single-call tool:

### Tool: `accumulate_submit_batch`

**Description:** Submit multiple transactions in one batch (all-in-one)

**Parameters:**
```typescript
{
  "transactions": Array<{
    "type": string,
    "params": object
  }>,
  "signing_method": "wallet" | "manual",
  "key_name": string,        // If wallet
  "signatures": Signature[]  // If manual
}
```

**Example:**
```json
{
  "transactions": [
    {
      "type": "sendTokens",
      "params": {
        "from": "acc://company.acme/payroll",
        "to": "acc://alice.acme/tokens",
        "amount": "5000000000"
      }
    },
    {
      "type": "sendTokens",
      "params": {
        "from": "acc://company.acme/payroll",
        "to": "acc://bob.acme/tokens",
        "amount": "6000000000"
      }
    }
  ],
  "signing_method": "wallet",
  "key_name": "company-key"
}
```

**Returns:**
```typescript
{
  "transaction_hashes": string[],
  "status": "confirmed"
}
```

**Pros:**
- Simpler to implement (no state)
- One-shot operation
- Easier for users

**Cons:**
- Less flexible
- Can't review before submission
- All-or-nothing approach

---

## Recommendation

**Implement Both Approaches:**

1. **Stateful Batch Tools** (7 tools) - For complex workflows
   - Full control
   - Review before submission
   - Export capability

2. **Single-Call Batch Tool** (1 tool) - For simple cases
   - Quick and easy
   - No state management
   - Good for small batches

Users choose based on complexity:
- **2-3 transactions:** Use `accumulate_submit_batch`
- **Complex workflows:** Use full batch tools

---

## Testing Strategy

### Unit Tests
- Batch creation
- Transaction addition
- Signature validation
- Envelope construction

### Integration Tests
- Submit batch to devnet
- Verify all transactions execute
- Test with 1, 3, 5, 10 transactions
- Test multi-signer scenarios

### Edge Cases
- Empty batch
- Unsigned batch submission attempt
- Invalid transaction in batch
- Duplicate batch IDs
- Batch timeout/expiration

---

## Security Considerations

1. **Batch Isolation**
   - Each batch has unique ID
   - Batches are user-scoped
   - Can't access other users' batches

2. **Signature Validation**
   - Verify all required signers
   - Check signature timestamps
   - Validate public keys

3. **Transaction Limits**
   - Max transactions per batch (e.g., 20)
   - Max batch size in bytes
   - Timeout after 1 hour

4. **Audit Trail**
   - Log all batch operations
   - Track submission attempts
   - Record transaction hashes

---

## Documentation Needed

- [ ] User guide: "How to Batch Transactions"
- [ ] Tutorial: "Payroll Example"
- [ ] API reference: All 7 batch tools
- [ ] Integration guide: Add to mcp-accumulate
- [ ] Test examples: Integration tests

---

## Summary

**Tools Designed:** 7 stateful + 1 simplified = 8 tools

**Benefits:**
- ✅ **Convenience** - Single API call for multiple transactions
- ✅ **Workflow Simplification** - Easy batching tools for users
- ✅ **Organization** - Logical grouping of related operations
- ✅ **Flexibility** - Review before submit, export capability
- ✅ **Control** - Build, review, sign, and submit in separate steps

**Non-Benefits (Clarified):**
- ❌ **No fee reduction** - Each transaction charged individually
- ❌ **No atomicity** - Transactions execute independently
- ❌ **No guaranteed success** - Partial failures can occur

**Implementation Effort:**
- Stateful approach: Medium (state management + 7 tools)
- Simplified approach: Low (1 tool, no state)
- Recommended: Both (best of both worlds)

**Next Steps:**
1. Implement simplified tool first (quick win)
2. Add stateful tools later (full feature set)
3. Document with examples
4. Add to mcp-accumulate repo

---

**Version:** 1.0
**Date:** 2025-10-20
**Status:** Design Complete
