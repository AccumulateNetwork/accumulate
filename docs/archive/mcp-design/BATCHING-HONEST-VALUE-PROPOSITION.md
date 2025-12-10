# Batching: Honest Value Proposition

## Purpose: Convenience for Submitting Multiple Transactions

Transaction batching in Accumulate allows you to submit multiple transactions in a single API call, making your workflow more convenient.

---

## Primary Benefit: Convenience

**What batching does:**
- ✅ Submit 5 transactions with **one API call** instead of five
- ✅ Simpler client code with less HTTP overhead
- ✅ Organize related operations with a single batch ID
- ✅ Better tracking and logging

**What it looks like:**

```javascript
// Without batching - 3 API calls
await submitTransaction(payment1);
await submitTransaction(payment2);
await submitTransaction(payment3);

// With batching - 1 API call
await submitBatch([payment1, payment2, payment3]);
```

---

## What Batching Does NOT Provide

### ❌ No Fee Savings

Each transaction in a batch is charged individually:

```
3 transactions separately: 3 × $0.03 = $0.09
3 transactions in batch:    3 × $0.03 = $0.09
SAVINGS: $0.00
```

**Why:** The Accumulate protocol charges fees per transaction, not per envelope.

### ❌ No Atomicity

Transactions execute independently. Partial execution is normal:

```
Batch with 3 transactions:
  Transaction 1: ✓ SUCCESS
  Transaction 2: ✗ FAILED (insufficient funds)
  Transaction 3: ✓ SUCCESS

Result: 2 transactions executed, 1 failed
```

**Why:** Each transaction gets its own database batch and commits/discards independently.

### ❌ No Guarantees

- No all-or-nothing execution
- No rollback if one transaction fails
- No dependency enforcement between transactions
- No guaranteed success for the batch

---

## When to Use Batching

### ✅ Good Use Cases

**1. Bulk Submissions for Convenience**
```
Payroll: Submit payments to 10 employees in one call
Updates: Modify multiple accounts together
Bulk operations: Many similar transactions
```

**2. Simplifying Client Code**
```
// Cleaner code
const result = await submitBatch(allTransactions);

// vs multiple submissions
for (const txn of allTransactions) {
    await submitTransaction(txn);
}
```

**3. Organizational Benefits**
```
- Single batch ID tracks all related operations
- Easier audit trails
- Better logging and monitoring
```

### ❌ When NOT to Batch

**1. Need Atomicity**
```
If you need all-or-nothing execution:
  → Use a single transaction type with multiple operations
  → Example: SendTokens with multiple recipients (atomic within one transaction)
```

**2. Critical Operations**
```
Where partial failure is unacceptable:
  → Submit individually and verify each
  → Implement application-level rollback
```

**3. Sequential Dependencies**
```
Where transaction N depends on transaction N-1 succeeding:
  → Submit sequentially and check status between submissions
```

---

## Handling Partial Failures

Since batching doesn't guarantee atomicity, your application must handle partial execution:

### Example: Payroll Batch

```javascript
const result = await submitBatch([
    payment1,  // Alice
    payment2,  // Bob
    payment3   // Charlie
]);

// Check individual statuses
const statuses = await Promise.all([
    getTransactionStatus(result.hashes[0]),
    getTransactionStatus(result.hashes[1]),
    getTransactionStatus(result.hashes[2])
]);

// Handle partial failure
const failed = statuses.filter(s => s.code !== 'Delivered');
if (failed.length > 0) {
    console.log(`${failed.length} payments failed`);
    // Application logic to handle:
    // - Notify admin
    // - Retry failed payments
    // - Update payroll records
}
```

---

## MCP Tools Design

### Tool 1: `accumulate_submit_batch` (Simplified)

**Purpose:** Submit multiple transactions in one call

**Parameters:**
```json
{
    "transactions": [
        {"type": "sendTokens", "params": {...}},
        {"type": "sendTokens", "params": {...}},
        {"type": "writeData", "params": {...}}
    ],
    "signing_method": "wallet",
    "key_name": "my-key",
    "password": "vault-password"
}
```

**Returns:**
```json
{
    "transaction_hashes": ["hash1", "hash2", "hash3"],
    "transaction_count": 3,
    "status": "submitted"
}
```

**Note:** Check individual transaction statuses to verify all succeeded.

### Tools 2-8: Stateful Batch Management (Optional)

For more complex workflows:
- `accumulate_batch_create` - Initialize batch
- `accumulate_batch_add_transaction` - Add transactions
- `accumulate_batch_info` - Review before submitting
- `accumulate_batch_sign` - Sign batch
- `accumulate_batch_submit` - Submit batch
- `accumulate_batch_cancel` - Cancel batch
- `accumulate_batch_export` - Export as JSON

---

## Comparison: Individual vs Batch

### Individual Submission

```javascript
// 3 API calls
await submitTransaction(txn1);  // POST /v3/submit
await submitTransaction(txn2);  // POST /v3/submit
await submitTransaction(txn3);  // POST /v3/submit

// Result: 3 HTTP requests, 3 fees
```

### Batch Submission

```javascript
// 1 API call
await submitBatch([txn1, txn2, txn3]);  // POST /v3/submit (with envelope)

// Result: 1 HTTP request, 3 fees
```

**Benefit:** Fewer API calls, simpler code
**Not a benefit:** Same total fees, no atomicity

---

## Real-World Example

### Scenario: Monthly Payroll

**Requirements:**
- Pay 3 employees
- Record audit log
- Update payment tracker

**Option A: Individual Submissions (4 API calls)**
```javascript
await sendTokens(alice, 50);
await sendTokens(bob, 60);
await sendTokens(charlie, 55);
await writeData(auditLog, "Payroll complete");
```

**Option B: Batched (1 API call)**
```javascript
await submitBatch([
    {type: "sendTokens", params: {to: alice, amount: 50}},
    {type: "sendTokens", params: {to: bob, amount: 60}},
    {type: "sendTokens", params: {to: charlie, amount: 55}},
    {type: "writeData", params: {data: "Payroll complete"}}
]);
```

**Advantages of Batching:**
- ✅ 1 API call instead of 4 (convenience)
- ✅ Single batch ID for tracking
- ✅ Cleaner code

**What you still need to handle:**
- ⚠️ Verify all 4 transactions succeeded
- ⚠️ Handle case where payments succeed but audit log fails
- ⚠️ Or payments fail but audit log succeeds
- ⚠️ Implement retry logic for failed transactions

---

## Summary

### Batching IS:
- ✅ A convenience feature for submitting multiple transactions
- ✅ Useful for reducing API calls
- ✅ Good for organization and tracking

### Batching is NOT:
- ❌ A way to save on transaction fees
- ❌ A way to get atomic execution
- ❌ A guarantee of all-or-nothing behavior
- ❌ A substitute for proper error handling

### Use Batching When:
- You have multiple transactions to submit
- You want simpler client code
- Convenience outweighs partial failure risks
- You have proper error handling in place

### Don't Use Batching When:
- You need atomic all-or-nothing execution
- Partial failure is unacceptable
- Sequential dependencies exist between transactions
- You only have one transaction (no benefit)

---

## Implementation Recommendation

**Phase 1: Simple Batch Tool (Recommended)**
- Implement `accumulate_submit_batch` for basic convenience
- 1 week effort, immediate value
- Low complexity, reuses existing code

**Phase 2: Stateful Tools (Optional)**
- Add full batch management if demand exists
- 2-3 weeks effort, higher complexity
- Provides more control and flexibility

**Documentation Emphasis:**
- Focus on convenience, not guarantees
- Be clear about limitations
- Provide examples of error handling
- Set correct expectations

---

**Value Proposition:**
"Submit multiple transactions in one API call for convenience"

**NOT:**
"Save fees and get atomic execution"

---

**Date:** 2025-10-20
**Status:** Corrected and Honest
**Focus:** Convenience
