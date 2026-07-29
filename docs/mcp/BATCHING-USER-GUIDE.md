# User Guide: Batching Transactions with Envelopes

## What is Transaction Batching?

Transaction batching allows you to submit multiple Accumulate transactions in a single API call, making your workflow more convenient:

**Primary Benefit: Convenience**
- **Single Submission:** One API call instead of many
- **Simpler Code:** Less HTTP overhead, easier to manage
- **Logical Grouping:** Organize related operations together
- **Better Tracking:** Single batch ID for multiple transactions

**Critical Limitations to Understand:**

⚠️ **No Atomicity** - Transactions execute independently. If transaction 2 fails, transactions 1 and 3 may still succeed. This is partial execution, not all-or-nothing.

⚠️ **No Fee Savings** - Each transaction is charged individually. 3 transactions = 3 fees, whether submitted separately or batched.

⚠️ **Application-Level Handling** - You must handle partial failures in your code.

## When to Use Batching

### Good Use Cases ✅

1. **Multiple Related Submissions**
   - Payroll: Submit payments to 10 employees in one call
   - Bulk updates: Modify multiple accounts together
   - Workflow convenience: Group related operations

2. **Simplifying Client Code**
   - Reduce API calls from 5 to 1
   - Single success/failure handling point
   - Better organization and logging

3. **Operational Grouping**
   - Track related transactions together
   - Single batch ID for reference
   - Easier audit trails

### When NOT to Batch ❌

1. **Need Atomicity** - Batching does NOT provide all-or-nothing execution
2. **Critical Operations** - Where partial failure is unacceptable
3. **Sequential Dependencies** - Where transaction N depends on transaction N-1 succeeding
4. **Different Timing** - Where transactions should execute at different times
5. **Single Transaction** - No convenience benefit

**Remember:** Batching is about convenience, not guarantees. If you need atomic operations, use a single transaction type that supports multiple operations (e.g., SendTokens with multiple recipients).

## Quick Start: 3 Simple Steps

### Using AI Assistant (Simplified Tool)

```
You: "Send 10 ACME to Alice, 20 ACME to Bob, and 15 ACME to Charlie"

AI: [creates batch with 3 send_tokens transactions]
AI: [signs with your wallet key]
AI: [submits batch]

AI: "✅ Sent! All 3 payments confirmed atomically in one envelope.
     All transactions executed together in the same block."
```

### Using Batch Tools (Full Control)

```
Step 1: Create Batch
> accumulate_batch_create
  {"description": "Team payments"}
  → batch_id: "batch_xyz"

Step 2: Add Transactions
> accumulate_batch_add_transaction
  {"batch_id": "batch_xyz", "type": "sendTokens", "params": {...}}
  (repeat for each transaction)

Step 3: Submit
> accumulate_batch_submit
  {"batch_id": "batch_xyz"}
  → transaction_hashes: [...]
```

## Detailed Examples

### Example 1: Monthly Payroll

**Scenario:** Pay 3 employees from company payroll account

```javascript
// Step 1: Create batch
const batch = await accumulate_batch_create({
  description: "October 2024 payroll"
});
// → batch_id: "payroll_oct_2024"

// Step 2: Add employee payments
await accumulate_batch_add_transaction({
  batch_id: "payroll_oct_2024",
  transaction_type: "sendTokens",
  params: {
    from: "acc://acme-corp.acme/payroll",
    to: "acc://alice.acme/salary",
    amount: "5000000000"  // 50 ACME
  }
});

await accumulate_batch_add_transaction({
  batch_id: "payroll_oct_2024",
  transaction_type: "sendTokens",
  params: {
    from: "acc://acme-corp.acme/payroll",
    to: "acc://bob.acme/salary",
    amount: "6000000000"  // 60 ACME
  }
});

await accumulate_batch_add_transaction({
  batch_id: "payroll_oct_2024",
  transaction_type: "sendTokens",
  params: {
    from: "acc://acme-corp.acme/payroll",
    to: "acc://charlie.acme/salary",
    amount: "5500000000"  // 55 ACME
  }
});

// Step 3: Add audit log entry
await accumulate_batch_add_transaction({
  batch_id: "payroll_oct_2024",
  transaction_type: "writeData",
  params: {
    account: "acc://acme-corp.acme/payroll-log",
    data: "October 2024 payroll: 3 employees, 165 ACME total"
  }
});

// Step 4: Review before signing
const info = await accumulate_batch_info({
  batch_id: "payroll_oct_2024"
});

console.log(`Batch: ${info.description}`);
console.log(`Transactions: ${info.transaction_count}`);
console.log(`Fee estimate: ${info.estimated_fee} ACME`);
console.log(`Required signers: ${info.required_signers.join(", ")}`);
// Output:
// Batch: October 2024 payroll
// Transactions: 4
// Fee estimate: 0.01 ACME
// Required signers: acc://acme-corp.acme/book/1

// Step 5: Sign with wallet key
await accumulate_batch_sign({
  batch_id: "payroll_oct_2024",
  signing_method: "wallet",
  key_name: "acme-corp-key",
  password: "my-vault-password"
});

// Step 6: Submit!
const result = await accumulate_batch_submit({
  batch_id: "payroll_oct_2024",
  wait: true  // Wait for confirmation
});

console.log(`✅ Payroll complete!`);
console.log(`Transaction hashes: ${result.transaction_hashes.join(", ")}`);
console.log(`Block: ${result.confirmation.block_height}`);
// Output:
// ✅ Payroll complete!
// Transaction hashes: abc123..., def456..., ghi789..., jkl012...
// Block: 1234567
```

**Cost Analysis:**
- Fees: 4 transactions × ~$0.03 each = ~$0.12 total
- Same cost whether submitted separately or batched

**Benefits Gained:**
- ✅ Single API submission (convenience)
- ✅ Grouped as "October 2024 payroll" batch (organization)
- ✅ One call to track instead of four (simpler code)

**Risk to Handle:**
- ⚠️ Partial execution possible - some payments may succeed while others fail
- ⚠️ Application must check individual transaction statuses
- ⚠️ May need compensating transactions if partial failure occurs

---

### Example 2: New User Onboarding

**Scenario:** Set up complete account structure for new user

```javascript
// Create batch for user setup
const batch = await accumulate_batch_create({
  description: "Onboard new user: Alice"
});

// 1. Create identity (ADI)
await accumulate_batch_add_transaction({
  batch_id: batch.batch_id,
  transaction_type: "createIdentity",
  params: {
    url: "acc://alice.acme",
    publicKey: "alice_public_key_hex",
    keyBookUrl: "acc://alice.acme/book"
  }
});

// 2. Create token account
await accumulate_batch_add_transaction({
  batch_id: batch.batch_id,
  transaction_type: "createTokenAccount",
  params: {
    url: "acc://alice.acme/tokens",
    tokenUrl: "acc://ACME",
    authorities: ["acc://alice.acme/book"]
  }
});

// 3. Create data account for profile
await accumulate_batch_add_transaction({
  batch_id: batch.batch_id,
  transaction_type: "createDataAccount",
  params: {
    url: "acc://alice.acme/profile",
    authorities: ["acc://alice.acme/book"]
  }
});

// 4. Write initial profile data
await accumulate_batch_add_transaction({
  batch_id: batch.batch_id,
  transaction_type: "writeData",
  params: {
    account: "acc://alice.acme/profile",
    data: JSON.stringify({
      name: "Alice",
      joined: "2024-10-20",
      role: "user"
    })
  }
});

// 5. Send welcome bonus
await accumulate_batch_add_transaction({
  batch_id: batch.batch_id,
  transaction_type: "sendTokens",
  params: {
    from: "acc://acme-corp.acme/welcome-fund",
    to: "acc://alice.acme/tokens",
    amount: "1000000000"  // 10 ACME welcome bonus
  }
});

// Sign and submit
await accumulate_batch_sign({
  batch_id: batch.batch_id,
  signing_method: "wallet",
  key_name: "admin-key"
});

const result = await accumulate_batch_submit({
  batch_id: batch.batch_id
});

console.log("✅ Alice is all set up with:");
console.log("  - Identity (ADI)");
console.log("  - Token account");
console.log("  - Profile data account");
console.log("  - 10 ACME welcome bonus");
```

**Benefits:**
- Single API submission (convenience)
- Organized as one "user onboarding" batch
- Simpler client code

**Risks:**
- ⚠️ Partial setup possible - identity may be created but token account fails
- ⚠️ User may have incomplete account setup
- ⚠️ Application must verify all operations succeeded
- ⚠️ May need cleanup if partial failure occurs

---

### Example 3: Bulk Key Management

**Scenario:** Add backup keys to multiple accounts

```javascript
const batch = await accumulate_batch_create({
  description: "Add backup keys to company accounts"
});

const accounts = [
  "acc://acme-corp.acme/treasury",
  "acc://acme-corp.acme/operations",
  "acc://acme-corp.acme/payroll"
];

const backupKey = "backup_public_key_hex";

for (const account of accounts) {
  await accumulate_batch_add_transaction({
    batch_id: batch.batch_id,
    transaction_type: "updateKeyPage",
    params: {
      url: `${account.replace(/\/[^/]+$/, "")}/book/1`,
      operation: "add",
      newKey: {
        publicKeyHash: backupKey,
        delegate: ""
      }
    }
  });
}

// Review
const info = await accumulate_batch_info({batch_id: batch.batch_id});
console.log(`Adding backup key to ${info.transaction_count} accounts`);

// Sign and submit
await accumulate_batch_sign({
  batch_id: batch.batch_id,
  signing_method: "wallet",
  key_name: "master-key"
});

await accumulate_batch_submit({batch_id: batch.batch_id});

console.log("✅ Backup key added to all accounts");
```

---

### Example 4: Atomic Payment with Receipt

**Scenario:** Send payment and write receipt atomically

```javascript
const batch = await accumulate_batch_create({
  description: "Invoice #12345 payment"
});

// 1. Send payment
await accumulate_batch_add_transaction({
  batch_id: batch.batch_id,
  transaction_type: "sendTokens",
  params: {
    from: "acc://buyer.acme/tokens",
    to: "acc://seller.acme/tokens",
    amount: "10000000000"  // 100 ACME
  }
});

// 2. Write receipt to buyer's records
await accumulate_batch_add_transaction({
  batch_id: batch.batch_id,
  transaction_type: "writeData",
  params: {
    account: "acc://buyer.acme/receipts",
    data: JSON.stringify({
      invoice: "12345",
      amount: "100 ACME",
      recipient: "acc://seller.acme/tokens",
      date: "2024-10-20",
      status: "paid"
    })
  }
});

// 3. Write receipt to seller's records
await accumulate_batch_add_transaction({
  batch_id: batch.batch_id,
  transaction_type: "writeData",
  params: {
    account: "acc://seller.acme/invoices",
    data: JSON.stringify({
      invoice: "12345",
      amount: "100 ACME",
      payer: "acc://buyer.acme/tokens",
      date: "2024-10-20",
      status: "received"
    })
  }
});

// Sign with buyer's key (for payment and their receipt)
await accumulate_batch_sign({
  batch_id: batch.batch_id,
  signing_method: "wallet",
  key_name: "buyer-key"
});

await accumulate_batch_submit({batch_id: batch.batch_id});

console.log("✅ Payment sent and receipt recorded");
```

**Benefits:**
- Single API submission (convenience)
- Related operations grouped together
- Simpler code

**Risks:**
- ⚠️ Payment may succeed while receipt write fails (partial execution)
- ⚠️ Or receipt may succeed while payment fails
- ⚠️ Application must verify both operations succeeded
- ⚠️ NOT atomic - cannot guarantee payment and receipt together

---

## AI Assistant Patterns

### Pattern 1: Detect Batch Opportunity

```
User: "Send 5 ACME to Alice, 10 ACME to Bob, 7 ACME to Charlie"

AI detects: Multiple similar operations → suggest batching

AI: "I notice you want to send to 3 people. I can batch these into
     one envelope so they execute atomically. Would you like me to do that?"

User: "Yes"

AI: [creates batch] [adds 3 transactions] [signs] [submits]
AI: "Done! All 3 payments sent atomically in one envelope."
```

### Pattern 2: Explain Batch Contents

```
User: "Create accounts for alice.acme"

AI: "I'll create a batch with:
     1. Identity (ADI) at acc://alice.acme
     2. Token account at acc://alice.acme/tokens
     3. Data account at acc://alice.acme/data

     All three will be created atomically in one envelope.
     Ready to proceed?"

User: "Go ahead"

AI: [executes batch]
```

### Pattern 3: Review Before Submit

```
AI: "I've prepared a batch with 5 transactions:
     1. Send 50 ACME to alice.acme/tokens
     2. Send 60 ACME to bob.acme/tokens
     3. Send 55 ACME to charlie.acme/tokens
     4. Send 70 ACME to dave.acme/tokens
     5. Write audit log entry

     Total: 235 ACME + 0.01 ACME fee
     Signer: acc://company.acme/book/1

     Review looks good?"

User: "Yes, submit it"

AI: [signs and submits]
```

---

## Common Mistakes to Avoid

### ❌ Mistake 1: Different Signers

```javascript
// BAD: Requires multiple signers
await accumulate_batch_add_transaction({
  batch_id: batch.batch_id,
  transaction_type: "sendTokens",
  params: {
    from: "acc://alice.acme/tokens",  // Needs Alice's signature
    to: "acc://bob.acme/tokens",
    amount: "1000000000"
  }
});

await accumulate_batch_add_transaction({
  batch_id: batch.batch_id,
  transaction_type: "sendTokens",
  params: {
    from: "acc://charlie.acme/tokens",  // Needs Charlie's signature
    to: "acc://dave.acme/tokens",
    amount: "1000000000"
  }
});

// Problem: Batch requires both Alice AND Charlie to sign
// Solution: Use separate transactions or get both signatures
```

### ❌ Mistake 2: Unrelated Operations

```javascript
// BAD: Batching unrelated operations
const batch = await accumulate_batch_create({});
await accumulate_batch_add_transaction({
  batch_id: batch.batch_id,
  transaction_type: "sendTokens",
  params: {...}  // Payment transaction
});
await accumulate_batch_add_transaction({
  batch_id: batch.batch_id,
  transaction_type: "updateKeyPage",
  params: {...}  // Unrelated security update
});

// Problem: If one fails, both fail
// Solution: Only batch related operations
```

### ❌ Mistake 3: Too Many Transactions

```javascript
// BAD: Huge batch
for (let i = 0; i < 100; i++) {
  await accumulate_batch_add_transaction({...});
}

// Problem: May exceed size limits or timeout
// Solution: Keep batches reasonable (< 20 transactions)
```

### ✅ Best Practices

1. **Batch related operations** (payroll, account setup, etc.)
2. **Keep batches under 20 transactions**
3. **Use same signer when possible**
4. **Review before submitting** (use batch_info)
5. **Name batches clearly** (helps with debugging)
6. **Wait for confirmation** on important batches

---

## Troubleshooting

### Batch Won't Submit

**Problem:** `accumulate_batch_submit` fails

**Check:**
1. Is batch signed? (`batch_info` shows `ready_to_submit: true`)
2. Are all transactions valid? (use `dry_run: true`)
3. Are there conflicting operations?
4. Is network available?

**Solution:**
```javascript
// Validate first
const result = await accumulate_batch_submit({
  batch_id: batch.batch_id,
  dry_run: true  // Don't actually submit
});

if (!result.valid) {
  console.log("Validation errors:", result.errors);
}
```

### Missing Signatures

**Problem:** "Batch not signed" error

**Solution:**
```javascript
// Check signing status
const info = await accumulate_batch_info({batch_id: batch.batch_id});
console.log("Ready:", info.ready_to_submit);
console.log("Signatures:", info.signatures_count);

// Sign if needed
if (!info.ready_to_submit) {
  await accumulate_batch_sign({
    batch_id: batch.batch_id,
    signing_method: "wallet",
    key_name: "my-key"
  });
}
```

### Transaction Rejected

**Problem:** One transaction in batch fails

**Result:** Entire batch may fail (atomicity)

**Solution:**
1. Review individual transactions
2. Test with `dry_run: true`
3. Check account balances
4. Verify authorities exist

---

## Fee Reality

### Important: No Fee Savings from Batching

**Individual Transactions:**
```
10 transactions × ~$0.03 each = ~$0.30 total
```

**Batched in Envelope:**
```
10 transactions × ~$0.03 each = ~$0.30 total
Same cost - each transaction is charged individually
```

### When to Use Batching

**Good reasons:**
- ✅ Convenience - Submit multiple transactions in one API call
- ✅ Workflow - Simplify client code with fewer HTTP requests
- ✅ Organization - Group related operations for tracking
- ✅ Bulk operations - Payroll, updates, etc.

**Bad reasons:**
- ❌ To save on fees (doesn't work - each transaction charged)
- ❌ To get atomicity (doesn't work - partial execution possible)
- ❌ To guarantee all-or-nothing (doesn't work - transactions independent)
- ❌ For critical operations where partial failure is unacceptable

---

## Summary

**Batching is great for:**
- ✅ Submitting multiple transactions in one call (convenience)
- ✅ Simplifying client code (fewer API calls)
- ✅ Organizing related operations (better tracking)
- ✅ Bulk submissions (payroll, updates, etc.)

**Key Benefits:**
- ⚡ **Convenience** - Single API submission
- 📦 **Organization** - Logical grouping of operations
- 🔧 **Simpler Code** - Less HTTP overhead
- 📋 **Better Tracking** - Single batch ID

**Critical Limitations:**
- ❌ **No atomicity** - Partial execution is normal
- ❌ **No fee savings** - Each transaction charged individually
- ⚠️ **Must handle failures** - Application-level error handling required

**Tools Available:**
- `accumulate_batch_create` - Start a batch
- `accumulate_batch_add_transaction` - Add operations
- `accumulate_batch_info` - Review batch
- `accumulate_batch_sign` - Sign batch
- `accumulate_batch_submit` - Submit batch

**Best Practices:**
1. Batch related operations
2. Keep batches under 20 transactions
3. Review before submitting
4. Name batches clearly
5. Wait for confirmation

---

## Next Steps

- Try Example 1 (payroll)
- Experiment with batch sizes
- Test dry-run validation
- Integrate into your workflow

## Questions?

- See [ENVELOPE-BATCHING-TOOLS.md](./ENVELOPE-BATCHING-TOOLS.md) for tool specs
- See [ENVELOPE-CONSTRUCTION-GUIDE.md](./ENVELOPE-CONSTRUCTION-GUIDE.md) for JSON format
- Open issue for help

---

**Happy Batching!** 🚀
