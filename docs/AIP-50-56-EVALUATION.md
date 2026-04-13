# AIP-50 and AIP-56 Evaluation Report

**Date:** 2026-04-09  
**Evaluator:** Claude Code  
**Scope:** Specification compliance, implementation completeness, code quality, security analysis

---

## Executive Summary

| AIP | Status | Exists? | Implementation | Issues | Severity |
|-----|--------|---------|-----------------|--------|----------|
| AIP-56 | ❌ N/A | **NO** | — | Specification missing | Critical |
| AIP-50 | Draft | YES | **Partial** | 6 critical gaps | High |

**AIP-56 Does Not Exist** — Only AIPs 001-050 present in governance repository.

**AIP-50 Partially Implemented** — Core feature (user-specified fees) missing; escrow and distribution mechanisms incomplete.

---

## AIP-56 Analysis

### **Finding: AIP-56 Does Not Exist**

**Investigation Result:** No AIP-56 specification found in the codebase.

**Location Checked:** `/home/paul/go/src/gitlab.com/AccumulateNetwork/governance/aip/AIP/`

**AIPs Present:**
- 001, 010, 012, 016, 029, 030, 031, 032, 033, 035, 043, 048, 049, 050
- **Highest numbered AIP: 050**

**Conclusion:**
AIP-56 does not exist. The AIP governance system currently has 14 numbered AIPs, with 050 being the most recent. No specification or code for AIP-56 exists in the repository.

---

## AIP-50 Analysis: User-Specified Transaction Fees

### **Specification**

**File:** `governance/aip/AIP/050-user-transaction-fees.md`  
**Status:** Draft  
**Version:** Current

**Requirements:**
1. Allow users to specify a fee that must be paid as a prerequisite to executing transactions
2. Fees are submitted as signatures and placed in escrow with active signature set
3. Fee handling:
   - **On expiration:** Fees returned to payer
   - **On failure:** Fees returned to payer
   - **On success:** Fees distributed to recipient account(s) specified by transaction
4. Use cases: Asset exchange, service provider fees

---

### **Implementation Status: PARTIAL**

#### **✅ Implemented Components**

**1. CreditPayment Message Type**
```
File: protocol/types_gen.go
Type: CreditPayment
Fields:
  - Paid (Fee amount)
  - Payer (Account URL)
  - Initiator (Boolean)
  - TxID (Transaction ID)
  - Cause (Signature TxID)
```
✅ **Status:** Fully implemented

**2. CreditPayment Executor**
```
File: internal/core/execute/v2/block/msg_credit_payment.go
Responsibility: Record payments in transaction history
```
✅ **Status:** Fully implemented

**3. User Signature Processing**
```
File: internal/core/execute/v2/block/sig_user.go (lines 500-521)
Responsibility: Create CreditPayment when user signature processed
```
✅ **Status:** Fully implemented for basic flow

**4. Fee Schedule Infrastructure**
```
File: protocol/fee_schedule.go
Responsibility: Compute fees based on transaction type, signature size, data size
```
✅ **Status:** Fully implemented

---

#### **❌ Not Implemented / Incomplete**

| Feature | Spec Requirement | Current Implementation | Status |
|---------|-----------------|------------------------|--------|
| User-specified fees | Users choose fee amount | Only fixed fees from schedule | ❌ MISSING |
| Escrow mechanism | Hold fees pending transaction | Immediate credit debit | ❌ MISSING |
| Fee distribution | Distribute to recipient | No distribution logic | ❌ MISSING |
| Multiple payers | Anyone can pay fees | Only initiator supported | ⚠️ LIMITED |
| Expiration refunds | Return fees on TTL | No explicit logic | ❌ UNCLEAR |
| Failure refunds | Return fees on error | Partial (capped at 0.01) | ⚠️ INCOMPLETE |

---

### **Critical Issues**

#### **ISSUE #1: User-Specified Fees Not Implemented (Severity: CRITICAL)**

**Spec Requirement:**  
"Allow users to specify a fee that must be paid as a prerequisite to executing transactions"

**Current Behavior:**  
Fees are computed automatically by the protocol based on fixed fee schedule.

**Evidence:**

**File:** `protocol/fee_schedule.go` (Lines 125-224)
```go
func (fs *FeeSchedule) ComputeSignatureFee(sigType SignatureType, sigSize int) Fee {
    base := fs.SignatureCost[sigType]
    // ... computed from fixed schedule
    return // Fixed amount
}

func (fs *FeeSchedule) ComputeTransactionFee(dataSize int) Fee {
    return fs.TransactionCost + Fee(len(data)/1000)
}
```

**File:** `internal/core/execute/v2/block/sig_user.go` (Lines 530-558)
```go
// No user-specified fee field; always uses ComputeSignatureFee()
fee := batch.FeeSchedule.ComputeSignatureFee(...)
fee += batch.FeeSchedule.ComputeTransactionFee(...) // Automatic addition
```

**What's Missing:**
- No field in `TransactionHeader` or signature to accept user fee
- No validation for user-specified fee amounts
- No mechanism to override computed fees
- Fee schedule hard-coded, not user-controllable

**Impact:** 
- Spec requirement not met
- Users cannot implement custom fee strategies
- Service provider use case impossible

**Recommendation:**  
Add optional fee field to transaction headers or signatures:
```go
// In signature or transaction header
UserSpecifiedFee *Fee `json:"userSpecifiedFee,omitempty"`

// In fee computation
if sig.UserSpecifiedFee != nil {
    fee = *sig.UserSpecifiedFee
} else {
    fee = batch.FeeSchedule.ComputeSignatureFee(...)
}
```

---

#### **ISSUE #2: No Escrow Mechanism (Severity: CRITICAL)**

**Spec Requirement:**  
"Tokens will be placed in escrow along with the active signature set"

**Current Behavior:**  
Fees are immediately debited from signer credits, no escrow holding period.

**Evidence:**

**File:** `internal/core/execute/v2/block/sig_user.go` (Lines 413-424)
```go
// Immediate debit, no escrow
err := e.DebitSigningAccount(sig, sigFee)
if err != nil {
    return err
}
```

**What's Missing:**
- No holding mechanism for fees pending transaction completion
- No conditional release on success/failure
- No timeout/expiration handling for escrow
- Fees immediately removed from account

**Impact:**
- If transaction fails, user might not receive refund
- Double-spending possible if escrow added later without proper atomicity
- Risk of fee loss due to unexpected transaction failure

**Recommendation:**
Implement escrow system:
```go
// Reserve (not debit) fees in escrow
func (e *Executor) ReserveSigningFees(sig Signature, amount Fee) error {
    // Create escrow entry with transaction ID
    escrow := &EscrowEntry{
        Amount: amount,
        TxID: e.txID,
        Expiration: e.txExpiration,
    }
    return e.EscrowAccount.ReserveFunds(escrow)
}

// On transaction completion, release or refund
func ReleaseEscrow(txID *TxID, success bool) error {
    if success {
        return escrow.Debit() // Transfer fees to recipient
    } else {
        return escrow.Refund() // Return to payer
    }
}
```

---

#### **ISSUE #3: No Fee Distribution Mechanism (Severity: HIGH)**

**Spec Requirement:**  
"Fees distributed to recipient account(s) specified by the original transaction"

**Current Behavior:**  
Fees are collected but never distributed; no recipient mechanism exists.

**Evidence:**

**File:** `internal/core/execute/v2/block/msg_credit_payment.go`
```go
// Records payment but doesn't distribute it
type CreditPayment struct {
    Paid Fee      // Amount
    Payer *url.URL // Who paid
    // NO field for where fees go
}
```

**What's Missing:**
- No fee recipient field in transaction
- No logic to route collected fees to specified accounts
- No fee distribution executor

**Impact:**
- Cannot implement "service provider fees" use case
- Cannot implement "asset exchange" use case
- Fees collected but stuck in protocol account

**Recommendation:**
Add fee recipient specification:
```go
// In transaction header
FeeRecipient *url.URL `json:"feeRecipient,omitempty"`

// In fee distribution
func (e *Executor) DistributeFees(txID *TxID, fees Fee) error {
    tx := e.GetTransaction(txID)
    if tx.FeeRecipient != nil {
        // Transfer fees to recipient
        return e.CreditAccount(tx.FeeRecipient, fees)
    }
    // Otherwise, send to protocol fee account
    return e.CreditAccount(e.ProtocolFeeAccount, fees)
}
```

---

#### **ISSUE #4: Incomplete Fee Refunds on Failure (Severity: HIGH)**

**Spec Requirement:**  
"If transaction executes but fails: fees will be returned"

**Current Behavior:**  
`FeeRefund` field exists but refund logic is incomplete; capped at 0.01 ACME.

**Evidence:**

**File:** `protocol/fee_schedule.go` (Lines 265-269)
```go
const FeeFailedMaximum = Fee(1_000_000) // 0.01 ACME

// If paid <= 0.01 ACME, no refund
if paid <= FeeFailedMaximum {
    return 0, nil // No refund!
}
// Only refund amount above 0.01
return paid - FeeFailedMaximum, nil
```

**Problem:**
- Users pay full fee but only get refunded if fee > 0.01 ACME
- Small transactions get no refund on failure
- Creates perverse incentive to craft failing transactions for small fees

**What's Incomplete:**
- Refund logic doesn't cover all failure scenarios
- Not all transaction failure paths verified to issue refunds
- Timing of refunds unclear (immediate vs. later?)

**Impact:**
- Users lose fees for failed transactions under 0.01 ACME
- Unfair to users with legitimate transaction failures
- Small fees essentially not refundable

**Recommendation:**
Remove fee cap on failures:
```go
// Full refund on any transaction failure
func ComputeFailedTransactionRefund(paid Fee) Fee {
    return paid // Return all fees, no cap
}
```

---

#### **ISSUE #5: Fee Return on Expiration Unclear/Missing (Severity: MEDIUM)**

**Spec Requirement:**  
"If transaction expires: fee(s) paid will be returned"

**Current Behavior:**  
`ExpiredTransaction` message exists but no explicit fee return logic found.

**Evidence:**

**File:** `internal/core/execute/v2/block/msg_transaction.go`
```go
// ExpiredTransaction message exists but fee handling unclear
type ExpiredTransaction struct {
    // No explicit fee return field
}
```

**What's Missing:**
- No verification that fees are returned when transactions expire (TTL exceeded)
- Expiration handler not explicitly reviewed for fee refunds
- No tests for expiration fee returns

**Impact:**
- Fees may be lost if transaction expires
- Unclear behavior for users whose transactions time out

**Recommendation:**
Add explicit fee return on expiration:
```go
func (e *Executor) HandleExpiredTransaction(tx *Transaction) error {
    // Return all fees paid on this transaction
    for _, sig := range tx.Signatures {
        if cp := e.FindCreditPayment(sig); cp != nil {
            return e.RefundFees(cp.Payer, cp.Paid)
        }
    }
    return nil
}
```

---

#### **ISSUE #6: Limited Payer Support (Severity: MEDIUM)**

**Spec Requirement:**  
Implicit support for flexible payer (not just initiator)

**Current Behavior:**  
Only the transaction initiator can pay fees.

**Evidence:**

**File:** `internal/core/execute/v2/block/sig_user.go` (Lines 504-509)
```go
// Explicit comment: we don't support non-initiator payers (yet)
if !didInit {
    return nil  // Only initiator pays
}
```

**What's Limited:**
- No support for delegated payers
- Cannot implement third-party fee payment
- Smart contracts cannot pay fees for users

**Impact:**
- Reduces use cases for service providers
- Cannot implement sponsored transactions
- Limited flexibility for complex transactions

**Recommendation:**
Add support for delegated payers:
```go
// Allow transaction to specify fee payer
type TransactionHeader struct {
    FeeInitiator *url.URL // Optional: who should pay fees (defaults to initiator)
}

// In signature processing
payer := header.FeeInitiator
if payer == nil {
    payer = initiator // Default to initiator
}
```

---

### **Security Issues**

#### **SECURITY ISSUE #1: Insufficient Refund on Failure (Severity: MEDIUM)**

**Issue:** `FeeFailedMaximum = 0.01 ACME` cap means small fees are never refunded.

**Vulnerability:**
- Users with failed transactions lose fees
- Attackers could craft failing transactions as a form of forced payment
- Breaks assumption that "fees returned on failure"

**Attack Scenario:**
```
1. User submits transaction with 0.005 ACME fee (legitimate)
2. Transaction fails execution
3. User loses entire 0.005 ACME (< 0.01 cap)
4. Attacker repeats with many small transactions
5. Protocol slowly accumulates lost fees
```

**Recommendation:**
Remove or significantly raise the cap:
```go
// Option 1: No cap (recommended)
const FeeFailedMaximum = Fee(0)

// Option 2: Very high cap (if loss expected)
const FeeFailedMaximum = Fee(1_000_000_000) // 10 ACME, not 0.01
```

---

#### **SECURITY ISSUE #2: Double-Spending Risk (Severity: MEDIUM)**

**Issue:** Immediate fee debit without escrow creates risk.

**Vulnerability:**
- If future escrow implementation is added incorrectly, could allow double-spending
- Fees immediately removed; if transaction later succeeds and fees distributed, account becomes negative
- No atomic transaction for: fee debit + execute + fee distribution

**Attack Scenario:**
```
1. User has 1 ACME in account
2. Fee charged: 0.5 ACME (balance: 0.5)
3. Transaction executes and succeeds
4. Fees distributed to service provider: +0.5 ACME
5. If escrow mis-implemented: could credit twice or create negative balance
```

**Recommendation:**
Implement proper escrow with atomic transactions:
```go
func (e *Executor) ProcessTransactionWithEscrow(tx *Transaction) error {
    // 1. Atomic reserve (not debit)
    err := e.ReserveFees(tx)
    if err != nil { return err }
    
    // 2. Execute transaction
    err = e.Execute(tx)
    
    // 3. Atomic release/refund (single operation)
    if err == nil {
        return e.ReleaseFees(tx) // Success: distribute
    } else {
        return e.RefundFees(tx)  // Failure: return
    }
}
```

---

#### **SECURITY ISSUE #3: No Maximum Fee Validation (Severity: LOW)**

**Issue:** No upper limit on user-specified fees (once implemented).

**Vulnerability:**
- Malicious user could specify exorbitant fees as denial-of-service
- Could drain accounts faster than intended
- No rate limiting on fee amounts

**Attack Scenario:**
```
1. Attacker specifies fee of 1,000,000 ACME (if implementation allows)
2. Protocol accepts and debits account
3. Can be repeated unlimited times
4. Drains account quickly
```

**Recommendation:**
Add fee validation:
```go
func ValidateUserFee(fee Fee, txType TransactionType) error {
    maxFee := e.FeeSchedule.ComputeTransactionFee(txType) * 10 // 10x normal max
    if fee > maxFee {
        return errors.New("fee exceeds maximum")
    }
    if fee < e.FeeSchedule.ComputeTransactionFee(txType) {
        return errors.New("fee below minimum")
    }
    return nil
}
```

---

### **Missing Logic Analysis**

#### **Missing: User Fee Input Field**
**Where:** `TransactionHeader` or signature message  
**What:** No field to accept user-specified fee  
**Required For:** Entire AIP-50 feature set

#### **Missing: Fee Escrow Manager**
**Where:** Separate component or in execution layer  
**What:** No system to hold fees pending transaction completion  
**Required For:** Proper fee refunds, atomic transactions

#### **Missing: Fee Distribution Logic**
**Where:** Post-transaction completion handler  
**What:** No mechanism to route fees to recipient  
**Required For:** "Asset exchange" and "service provider" use cases

#### **Missing: Expiration Handler with Refunds**
**Where:** ExpiredTransaction executor  
**What:** No explicit fee refund on TTL expiration  
**Required For:** "Return fees on expiration" requirement

#### **Missing: Non-Initiator Payer Support**
**Where:** Signature processing and transaction routing  
**What:** Only initiator can pay; no delegation  
**Required For:** Sponsored transactions, service provider fees

#### **Missing: Comprehensive Test Coverage**
**Where:** Test suite  
**What:** No tests for user fee validation, refunds, distribution  
**Required For:** Confidence in implementation

---

### **Error Analysis**

| Error Type | Count | Severity | Example |
|-----------|-------|----------|---------|
| Missing feature | 6 | Critical | User fee input |
| Incomplete feature | 2 | High | Fee refunds capped, expiration unclear |
| Logic error | 1 | Medium | Double-spending escrow risk |
| Validation gap | 2 | Medium | No fee limit, no escrow atomicity |

---

## Recommendations by Priority

### **CRITICAL - Do Before Production**

1. **Implement user-specified fee input**
   - Add field to signature or transaction header
   - Implement validation
   - Update fee computation logic
   - Effort: ~2-3 days

2. **Implement proper escrow system**
   - Create fee reserve mechanism
   - Ensure atomicity with execution
   - Verify all success/failure paths
   - Effort: ~3-5 days

3. **Remove fee refund cap**
   - Change `FeeFailedMaximum` to 0
   - Test all failure refund paths
   - Effort: ~1 day

### **HIGH - Do Before DAG-BFT Production**

4. **Implement fee distribution**
   - Add fee recipient field to transaction
   - Create distribution executor
   - Effort: ~2-3 days

5. **Implement expiration fee returns**
   - Update ExpiredTransaction handler
   - Add comprehensive tests
   - Effort: ~1-2 days

6. **Add maximum fee validation**
   - Implement fee bounds checking
   - Set reasonable limits
   - Effort: ~1 day

### **MEDIUM - Do Before Mainnet**

7. **Support non-initiator payers**
   - Allow delegated fee payment
   - Effort: ~2-3 days

8. **Add comprehensive test coverage**
   - User fee validation tests
   - Escrow lifecycle tests
   - Refund scenario tests
   - Effort: ~2-3 days

9. **Update specification**
   - Move from Draft to Implementation Spec
   - Document fee behavior clearly
   - Effort: ~1 day

---

## Summary Table

| Aspect | Status | Grade | Notes |
|--------|--------|-------|-------|
| **Specification Completeness** | Draft | C+ | Missing details on user fee format |
| **Implementation Completeness** | Partial | D | 6 critical features missing |
| **Code Quality** | Good | B | Well-written, but incomplete |
| **Test Coverage** | Partial | C | Basic tests, missing scenarios |
| **Security** | At Risk | D+ | Escrow and refund issues |
| **Production Readiness** | No | F | Not ready; critical gaps |

---

## Conclusion

**AIP-50 Status:** Specification exists but implementation is incomplete. The core feature (user-specified fees) is missing entirely. Escrow mechanism, fee distribution, expiration handling, and non-initiator payers are not implemented. Current implementation only supports fixed automatic fees from protocol fee schedule.

**AIP-56 Status:** Does not exist.

**Recommendation:** AIP-50 should be marked as "Work in Progress" until critical gaps are addressed. Do not deploy to production mainnet without completing at least critical items 1-3.

---

**Report Generated:** 2026-04-09  
**Evaluation Depth:** Specification review + code analysis + security assessment  
**Confidence Level:** High (thorough investigation of codebase)
