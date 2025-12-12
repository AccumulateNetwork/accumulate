# Atomicity Correction

## Critical Error #2: Envelopes Do NOT Provide Atomicity

**WRONG CLAIM:** "Envelopes guarantee atomicity - all transactions execute together or fail together"

**REALITY:** Envelopes provide NO atomicity guarantees. Transactions are processed independently.

---

## The Code Evidence

### 1. Each Transaction Gets Its Own Batch

From `internal/core/execute/v2/block/msg_transaction.go:229-231`:

```go
func (x TransactionMessage) Process(batch *database.Batch, ctx *MessageContext) (_ *protocol.TransactionStatus, err error) {
	batch = batch.Begin(true)              // Create CHILD batch
	defer func() { commitOrDiscard(batch, &err) }()  // Commit or discard THIS batch only
	// ...
}
```

**Key Point:** Each transaction gets its own isolated child batch.

### 2. Independent Commit/Discard

From `internal/core/execute/v2/block/msg_common.go:303-311`:

```go
func commitOrDiscard(batch *database.Batch, err *error) {
	if *err != nil {
		batch.Discard()  // Discard ONLY this transaction's changes
		return
	}
	e := batch.Commit()  // Commit ONLY this transaction's changes
	*err = errors.UnknownError.Skip(1).Wrap(e)
}
```

**Key Point:** Each transaction's batch commits or discards independently. No rollback of previous transactions.

### 3. Sequential Processing with No Rollback

From `internal/core/execute/v2/block/exec_process.go:182-197`:

```go
// Process each message
for _, msg := range d.messages {
	ctx := &MessageContext{bundle: d, message: msg}
	st, err := d.callMessageExecutor(b.Batch, ctx)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)  // Only internal DB errors stop processing
	}

	// Some executors may not produce a status at this stage
	if st != nil {
		statuses = append(statuses, st)  // Collect status (including errors)
	}

	d.additional = append(d.additional, ctx.additional...)
	d.produced = append(d.produced, ctx.produced...)
}
```

**Key Points:**
- Messages processed in a simple for loop
- Only UnknownError (internal DB errors) stops the loop
- Client errors (insufficient funds, etc.) are captured in status
- Processing continues to next message regardless

### 4. Error Handling Per Transaction

From `internal/core/execute/v2/block/msg_transaction.go:264-274`:

```go
// Update the status
switch {
case err == nil:
	// DO NOT update the status code. The status code should only be updated
	// when the transaction is executed.

case errors.Code(err).IsClientError():
	status.Set(err)  // Set error status but RETURN NORMALLY

default:
	return nil, errors.UnknownError.Wrap(err)  // Only internal errors stop processing
}

err = batch.Transaction2(ctx.message.Hash()).Status().Put(status)
if err != nil {
	return nil, errors.UnknownError.WithFormat("store status: %w", err)
}

return status, nil  // Returns normally even with client errors
```

**Key Point:** Client errors are stored in the status and the function returns normally (not an error). This allows the next message to process.

---

## Concrete Example: 3-Transaction Envelope with Failure

**Scenario:**
```
Envelope {
  Transaction[0]: SendTokens(Alice -> Bob, 100 ACME)
  Transaction[1]: SendTokens(Bob -> Charlie, 1000 ACME)  ← Will FAIL (insufficient balance)
  Transaction[2]: WriteData(Alice, "Payment complete")
}
```

**Processing:**

1. **Normalize envelope** → [TxnMsg0, SigMsg0, TxnMsg1, SigMsg1, TxnMsg2, SigMsg2]

2. **Loop through messages:**

   **Message 0: Transaction 0 (Alice → Bob, 100)**
   ```
   batch0 = parentBatch.Begin(true)       // Create child batch
   Execute: Transfer 100 ACME from Alice to Bob
   Result: SUCCESS
   commitOrDiscard(batch0, nil)           // Commit batch0 ✓
   status[0] = Delivered
   Continue to next message...
   ```

   **Message 1: Signature 0**
   ```
   Process signature...
   Continue to next message...
   ```

   **Message 2: Transaction 1 (Bob → Charlie, 1000)**
   ```
   batch1 = parentBatch.Begin(true)       // Create child batch
   Execute: Transfer 1000 ACME from Bob to Charlie
   Result: FAIL - Insufficient funds
   status.Set("insufficient funds")
   commitOrDiscard(batch1, error)         // Discard batch1 ✗
   status[1] = Error("insufficient funds")
   Return status normally                 // NOT an error return!
   Continue to next message...            // ← CONTINUES!
   ```

   **Message 3: Signature 1**
   ```
   Process signature...
   Continue to next message...
   ```

   **Message 4: Transaction 2 (WriteData)**
   ```
   batch2 = parentBatch.Begin(true)       // Create child batch
   Execute: Write data "Payment complete"
   Result: SUCCESS
   commitOrDiscard(batch2, nil)           // Commit batch2 ✓
   status[2] = Delivered
   Continue to next message...
   ```

3. **Final Result:**
   ```
   Transaction 0: ✓ EXECUTED (100 ACME transferred)
   Transaction 1: ✗ FAILED (rejected, no changes)
   Transaction 2: ✓ EXECUTED (data written saying "Payment complete")
   ```

**Problem:** Alice sent 100 ACME to Bob, and wrote "Payment complete", but Bob never sent anything to Charlie. Partial execution occurred!

---

## What About Block-Level Atomicity?

**Important Distinction:** While envelope transactions are NOT atomic, the entire block IS atomic:

From `internal/core/execute/v2/block/block.go`:

```go
type closedBlock struct {
	Block
	valUp []*execute.ValidatorUpdate
}

func (s *closedBlock) Commit() error {
	if s.IsEmpty() {
		s.Discard()
		return nil
	}

	err := s.Executor.EventBus.Publish(execute.WillCommitBlock{
		Block: s,
	})
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	return s.Batch.Commit()  // ENTIRE BLOCK commits or fails
}

func (s *closedBlock) Discard() {
	s.Batch.Discard()  // ENTIRE BLOCK discarded
}
```

**Block Atomicity:**
- If a **database error** occurs, the entire block fails
- All transactions in the block would be discarded
- This is for consensus/database integrity, not application logic

**Transaction Atomicity:**
- Application-level errors (insufficient funds, invalid operations) do NOT cause block failure
- Each transaction succeeds or fails independently
- No rollback of other transactions in the envelope

---

## Comparison Table

| Atomicity Level | Behavior |
|----------------|----------|
| **Envelope transactions** | ❌ NOT atomic - each transaction independent |
| **Transaction failure propagation** | ❌ Does NOT stop other transactions |
| **Rollback on failure** | ❌ NO - other transactions stay committed |
| **All-or-nothing guarantee** | ❌ NO - partial execution is normal |
| **Block (consensus)** | ✅ YES - database errors fail entire block |

---

## So What ARE the Benefits of Batching?

After removing fee savings and atomicity claims, what's left?

### 1. ✅ Single API Call (Convenience)

**Before batching:**
```
POST /v3/submit - Transaction 1
POST /v3/submit - Transaction 2
POST /v3/submit - Transaction 3
```

**With batching:**
```
POST /v3/submit - Envelope with 3 transactions
```

**Benefit:** Less client code, one API call instead of three.

### 2. ✅ Same Block Execution

All transactions in an envelope are processed in the same block.

**Benefit:**
- Guaranteed execution order within the block
- Same block timestamp
- Consistent block context

**Not a benefit:**
- ❌ Does NOT guarantee all succeed
- ❌ Does NOT prevent partial execution

### 3. ✅ Logical Grouping

Envelope provides a way to group related transactions together conceptually.

**Benefit:**
- Easier to track related operations
- Single envelope hash/ID
- Better organization

**Not a benefit:**
- ❌ Does NOT enforce any relationship
- ❌ Does NOT guarantee consistency
- ❌ Just packaging, no semantic meaning

### 4. ✅ Simplified Signature Management

If multiple transactions from the same principal need the same signature, you can provide one signature that covers multiple transactions (if the signature covers all transaction hashes).

**Benefit:** Potentially fewer signature operations needed.

**Limitation:** Only works if:
- Same signer for all transactions
- Same principal for all transactions
- Signature properly covers all transaction hashes

---

## Honest Value Proposition

### When Batching Makes Sense

1. **Convenience** - You have multiple transactions to submit and want to make one API call
2. **Same-block execution** - You want all transactions in the same block (but NOT atomic)
3. **Logical grouping** - You want to track related operations together
4. **Signature simplification** - Multiple transactions from same principal/signer

### When Batching Does NOT Help

1. ❌ **Atomicity** - Do NOT batch if you need all-or-nothing execution
2. ❌ **Fee savings** - Do NOT batch to save on fees (doesn't work)
3. ❌ **Error handling** - Do NOT batch if you need to handle failures together
4. ❌ **State consistency** - Do NOT batch if partial execution is problematic

---

## If You Need Atomicity

Accumulate does NOT provide multi-transaction atomicity at the protocol level. If you need atomic operations:

### Option 1: Single Transaction

Combine multiple operations into a single transaction type. For example:
- Send tokens to multiple recipients in one SendTokens transaction (use the `To` array)
- This is atomic within the single transaction

### Option 2: Application-Level Logic

- Submit transactions individually
- Track which succeeded/failed
- Implement compensating transactions for rollback
- Handle partial failure at application level

### Option 3: Custom Transaction Type

- Define a new transaction type that encapsulates your atomic operation
- All logic within one transaction body
- Protocol handles it as one atomic unit

### What Does NOT Work

- ❌ Using envelopes (no atomicity guarantee)
- ❌ Batching transactions (independently processed)
- ❌ Assuming same-block = atomic (incorrect)

---

## Documentation That Needs Correction

All three batching documents need atomicity claims removed:

### 1. ENVELOPE-BATCHING-TOOLS.md
- Remove "Atomicity (all transactions execute together)" claims
- Remove "all-or-nothing" references
- Update benefits to reflect reality

### 2. BATCHING-USER-GUIDE.md
- Remove all atomicity guarantees
- Fix examples showing partial failures
- Update "Why Use Batching" section
- Correct payroll/onboarding examples

### 3. BATCHING-IMPLEMENTATION-ROADMAP.md
- Remove atomicity references
- Update value propositions
- Correct success metrics

---

## The Real Question

**Should we even recommend batching at all?**

Given:
- ❌ No fee savings
- ❌ No atomicity
- ✅ Only convenience + same-block

**Honest assessment:**
- Batching provides MINIMAL benefits
- Main benefit is single API call (convenience)
- Same-block execution has limited value
- May create false expectations of atomicity
- Could lead to bugs from partial execution

**Recommendation:**
- Document batching capability (it exists)
- Be very clear about limitations
- Do NOT promote it as a major feature
- Focus documentation on individual transaction submission
- Only use batching for legitimate convenience cases

---

## Summary

### Error #1: Fee Savings ✗
**Claimed:** "Save 75-90% on fees by batching"
**Reality:** Each transaction charged individually

### Error #2: Atomicity ✗
**Claimed:** "All transactions execute together or fail together"
**Reality:** Transactions processed independently, partial execution normal

### Actual Benefits ✓
1. Single API call (convenience)
2. Same-block execution (limited value)
3. Logical grouping (organizational)

### Critical Correction Needed
- Remove ALL atomicity claims
- Remove ALL all-or-nothing guarantees
- Remove examples showing atomic behavior
- Add warnings about partial execution
- Significantly downplay batching as a feature

---

**Status:** Critical Error - Requires Immediate Correction
**Impact:** High - Misleading users about fundamental behavior
**Priority:** URGENT

---

**Date:** 2025-10-20
**Type:** Critical Correction
