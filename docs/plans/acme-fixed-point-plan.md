# Fixed-Point Amounts for Accumulate

**Status:** Draft
**Owner:** Paul Snow
**Scope:** Replace `*big.Int` with a fixed-width 256-bit unsigned amount across
ACME balances, supply, and related arithmetic.
**Non-goals:** Credit balances (stay `uint64`), EVM `ChainID` in
`TypedDataSignature` (stays `*big.Int`), user-token precision cap (already 18).

---

## 1. Motivation

`big.Int` is wrong for value-bearing state on a deterministic consensus chain:

- **Allocations in the hot path.** Every `Add`, `Sub`, `Mul`, `Div` heap-allocates
  a fresh `big.Int`. Block execution touches these thousands of times per block.
- **Unbounded input risk.** Nothing stops a peer from sending a 4 KB "amount".
  The encoder writes whatever `v.Bytes()` produces; the decoder accepts
  whatever length was prefixed. A fixed-width upper bound eliminates a class of
  DoS.
- **Variable wire size.** Fees, gas, block-size accounting all depend on tight
  serialization bounds. `big.Int` gives us a 1-byte amount or a 64-byte amount.
- **Auditability.** Reviewers have to reason about arbitrary-precision overflow
  in reward/oracle math. A 256-bit hard ceiling removes the question.

The realistic ACME supply is `5 × 10¹⁶` base units. That fits in 56 bits. A
256-bit container gives us `~10⁶²` headroom — enough for `balance × rate`
products in reward math without spilling to a wider intermediate.

## 2. Design decisions

### 2.1. Type choice

Adopt `github.com/holiman/uint256` (battle-tested from go-ethereum). Wrap it in
a protocol-level alias so callers never import the vendor path directly:

```go
// package protocol
type Amount = uint256.Int  // value type, 32 bytes
```

Rejected alternatives:
- Writing our own `uint128`/`uint256`: real work, no benefit.
- Using two `uint64`s: hostile to readers, no arithmetic API.
- `uint128`: too tight. `balance × rate` in reward math can exceed 128 bits
  even when both inputs fit.

### 2.2. Wire format: stay byte-compatible with `big.Int`

`encoding.Writer.WriteBigInt` writes `v.Bytes()` — big-endian, *no leading
zeros*, length-prefixed. This is *hash-stable*: the same numeric value encodes
to the same bytes regardless of container type.

The new `WriteAmount` must produce byte-identical output:

```go
func (w *Writer) WriteAmount(n uint, v *Amount) {
    // strip leading zero bytes, big-endian
    b := v.Bytes()                    // uint256.Bytes() returns 0..32 bytes, no leading zeros
    w.WriteBytes(n, b)
}
```

Verify the contract: `uint256.Int.Bytes()` returns the same bytes as
`big.Int.SetBytes(uint256.Int.Bytes()).Bytes()` — i.e. minimal big-endian
representation. This is true in the current `holiman/uint256` release; pin to a
version and add a compat test.

**Consequence:** Merkle roots of pre-migration accounts stay the same. No
consensus break from the encoding change alone.

**Input-side guard:** `ReadAmount` rejects any field > 32 bytes.

### 2.3. Sign and nil

`big.Int` is signed; `uint256.Int` is unsigned. The existing writer already
rejects negative values (`writer.go:227`). Migrating to unsigned is strictly
tighter.

Nullable fields (`TokenIssuer.SupplyLimit`, `SyntheticDepositCredits.AcmeRefundAmount`,
`CreateToken.SupplyLimit`) currently use `*big.Int`. Options:

- **(A)** Model as `*Amount` — keeps explicit "unset" semantics.
- **(B)** Model as `Amount` with a separate `HasSupplyLimit bool` flag.

**Choice: (A).** Pointer-nullable matches existing call sites (`if i.SupplyLimit == nil`)
and the YAML `optional: true` generator convention. A 32-byte value behind a
pointer is still 40 bytes total — fine.

### 2.4. JSON format unchanged

JSON representation is already a decimal string via `BigintToJSON`. New helpers
`AmountToJSON`/`AmountFromJSON` produce/accept the same string format. Public
API contract is preserved. SDK regeneration is cosmetic.

## 3. Affected surface

### 3.1. Schema (YAML-driven codegen)

17 `bigint` fields in `protocol/*.yml`. Add a new YAML type `amount` with the
semantics in §2. Map existing ACME/token fields to `amount`; leave one (EVM
`ChainID`) as `bigint`.

| File | Field | Current | New |
|---|---|---|---|
| `protocol/accounts.yml:40` | `TokenAccount.Balance` | bigint | amount |
| `protocol/accounts.yml:73` | `LiteTokenAccount.Balance` | bigint | amount |
| `protocol/accounts.yml:155` | `TokenIssuer.Issued` | bigint | amount |
| `protocol/accounts.yml:157` | `TokenIssuer.SupplyLimit` (opt) | bigint | amount |
| `protocol/user_transactions.yml:130` | `SendTokens` recipient amount | bigint | amount |
| `protocol/user_transactions.yml:149` | `IssueTokens.Amount` | bigint | amount |
| `protocol/user_transactions.yml:160` | `BurnTokens.Amount` | bigint | amount |
| `protocol/user_transactions.yml:200` | `AddCredits.Amount` | bigint | amount |
| `protocol/user_transactions.yml:~` | `CreateToken.SupplyLimit` (opt) | bigint | amount |
| `protocol/synthetic_transactions.yml:51` | `SyntheticBurnTokens.Amount` | bigint | amount |
| `protocol/synthetic_transactions.yml:67` | `SyntheticDepositTokens.Amount` | bigint | amount |
| `protocol/synthetic_transactions.yml:78` | `AcmeRefundAmount` (opt) | bigint | amount |
| `protocol/system.yml:17` | `BlockValidatorAnchor.AcmeBurnt` | bigint | amount |
| `protocol/system.yml:108` | `SystemLedger.AcmeBurnt` | bigint | amount |
| `protocol/transaction_results.yml:20` | `AddCreditsResult.Amount` | bigint | amount |
| `protocol/general.yml:139` | `TokenRecipient.Amount` | bigint | amount |
| `protocol/signatures.yml:259` | `TypedDataSignature.ChainID` | bigint | **stays bigint** |

### 3.2. Hand-written protocol code

- `protocol/token_account.go` — `AccountWithTokens` interface + 4 method
  implementations on each of `TokenAccount` and `LiteTokenAccount`, plus
  `TokenIssuer.Issue`. Change signatures from `*big.Int` to `*Amount`.
- `protocol/format.go` — `FormatBigAmount` callers; add `FormatAmount(*Amount, uint64)`.

### 3.3. Executor arithmetic

**v2 (primary, DAG-BFT):**
- `internal/core/execute/v2/chain/send_tokens.go` — per-recipient sum, balance check.
- `internal/core/execute/v2/chain/issue_tokens.go` — `TokenIssuer.Issue` call, supply check.
- `internal/core/execute/v2/chain/burn_tokens.go` — debit + synthetic burn construction.
- `internal/core/execute/v2/chain/add_credits.go` — **the only non-mechanical site.**
  See §3.4.

**v1 (legacy, CometBFT):**
- Mirror files under `internal/core/execute/v1/chain/`. v1 is on the removal track
  (branch `issue-3910-remove-cometbft`); coordinate sequencing with that work.
  If v1 is removed before M3, skip. Otherwise, update in lockstep.

### 3.4. `add_credits.go` oracle math

Current (lines 67–77):

```go
minSpend := new(big.Int)
minSpend.SetUint64(FeeMinimumCreditPurchase.AsUInt64() * AcmeOraclePrecision * AcmePrecision)
minSpend.Div(minSpend, big.NewInt(int64(CreditUnitsPerFiatUnit*st.Globals.Oracle.Price)))

credits := big.NewInt(int64(CreditUnitsPerFiatUnit * st.Globals.Oracle.Price))
credits.Mul(credits, &body.Amount)
credits.Div(credits, big.NewInt(int64(AcmeOraclePrecision*AcmePrecision)))
```

Two concerns:

1. **Pre-multiply in `uint64`.** `FeeMinimumCreditPurchase * AcmeOraclePrecision * AcmePrecision`
   is `1e2 * 1e4 * 1e8 = 10¹⁴`. Fits in `uint64`. But this arithmetic happens *in
   Go `uint64` land before* being moved into `big.Int`, and the current code
   silently wraps on overflow. Oracle price is `uint64`; `CreditUnitsPerFiatUnit *
   Oracle.Price` can overflow for large oracle values. **This is a latent bug
   today.** Fix as part of M3: do the multiply in `Amount` space using
   `MulOverflow`.

2. **`credits.Int64()` on line 80** — truncates silently. Rewrite using
   `Amount.IsZero()`.

Rewrite (illustrative):

```go
minSpend := new(Amount).SetUint64(FeeMinimumCreditPurchase.AsUInt64())
minSpend.Mul(minSpend, NewAmountUint64(AcmeOraclePrecision))
minSpend.Mul(minSpend, NewAmountUint64(AcmePrecision))
divisor := new(Amount).SetUint64(CreditUnitsPerFiatUnit)
divisor.Mul(divisor, NewAmountUint64(st.Globals.Oracle.Price))
minSpend.Div(minSpend, divisor)

if body.Amount.Cmp(minSpend) < 0 { ... }

credits := new(Amount).Mul(divisor, &body.Amount)
scale := new(Amount).SetUint64(AcmeOraclePrecision)
scale.Mul(scale, NewAmountUint64(AcmePrecision))
credits.Div(credits, scale)
if credits.IsZero() { ... }
```

Every intermediate stays in 256 bits. No Go-level `uint64` multiplies.

### 3.5. Encoding

- `pkg/types/encoding/writer.go` — add `WriteAmount`.
- `pkg/types/encoding/reader.go` — add `ReadAmount` with 32-byte length guard.
- `pkg/types/encoding/json.go` — add `AmountToJSON`, `AmountFromJSON`.
- Codegen templates — emit `WriteAmount`/`ReadAmount` for `amount` YAML type.

### 3.6. API / SDK

- `internal/api/v2/types_gen.go` — balances cross as decimal strings already.
  Regenerate against new types.
- Public API contract: **unchanged.**

### 3.7. Tests

- `test/helpers/construct.go`, `test/simulator/factory.go`,
  `test/cmd/gen-testdata/main.go` — ~110 `big.NewInt(...)` call sites. Mechanical.
- New dedicated tests (§5).

## 4. Migration and rollout

No state-format migration is needed because §2.2 keeps byte-identical wire
encoding for all values currently representable (none exceed 2²⁵⁶). The change
is a code-level type swap.

**Version gate:** Still wrap the executor changes behind an
`ExecutorVersion` bump so a new node reading old state, and an old node reading
new state, both decode correctly. The wire bytes are the same, but the *input
validation* tightens (32-byte ceiling). Rolling upgrade order:

1. Deploy new binary to all nodes (hashes unchanged, old and new binaries
   produce the same block hash).
2. At a chosen block height, activate the new executor version (enforces 32-byte
   ceiling on deserialization).

This eliminates the "someone sends a 33-byte amount to an old node" split-brain
risk. Coordinate the activation height via the existing version activation
mechanism (same as prior breaking changes).

## 5. Test plan

**T1. Round-trip compat (gate for M1).** For 1000 random values in
`[0, 2²⁵⁶)`: encode with `WriteBigInt`, decode with `ReadAmount` — equal.
Encode with `WriteAmount`, decode with `ReadBigInt` — equal. Encode both ways —
byte-identical.

**T2. Hash stability.** Snapshot the current devnet's account state hash for
a known account (`foo.acme`, nonzero balance). Re-encode with new types. Assert
hash unchanged.

**T3. Oversize rejection.** Craft a wire message with a 33-byte amount field.
`ReadAmount` must error without panicking.

**T4. Oracle math equivalence.** For 10⁶ random (oracle_price, amount) pairs
in realistic ranges, assert old `add_credits` math and new math produce
identical `credits` values and identical `minSpend` comparisons.

**T5. Oracle overflow stress.** For `oracle_price = MaxUint64`, confirm the new
code returns a defined error (not silent wrap).

**T6. Executor conformance.** Existing executor-test corpus must pass
unchanged. Any test that breaks is a semantic regression, not a test artifact.

**T7. Property test.** `Issue`, `CreditTokens`, `DebitTokens`, `Send`
round-trip invariants (sum-of-balances = total_issued) on a generated workload.

## 6. Milestones and sequencing

Single engineer, sequential:

| # | Milestone | Estimate | Gate |
|---|---|---|---|
| M1 | `Amount` type, YAML `amount`, `WriteAmount`/`ReadAmount`, JSON helpers, codegen template | 2d | T1, T3 pass |
| M2 | Flip all `bigint` fields listed in §3.1 to `amount` in YAML; regenerate | 1d | Compiles; T2 passes on a snapshot |
| M3 | Hand-written protocol code (`token_account.go`, `format.go`) and v2 executor (`send_tokens`, `issue_tokens`, `burn_tokens`, `add_credits`) | 3d | T4, T5, T6, T7 pass |
| M4 | v1 executor mirror (conditional on CometBFT removal status) | 1d | T6 passes on v1 corpus |
| M5 | Test fixture update (~110 sites) | 1d | Full test suite green |
| M6 | Activation-height wiring + executor version bump | 0.5d | Version gate test |
| M7 | SDK regeneration + load test on devnet | 0.5d | Performance regression check |

**Total: 8–9 engineering days.**

## 7. Risk register

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| `uint256.Bytes()` diverges from `big.Int.Bytes()` for some value | Low | High (hash mismatch) | T1 compat test on gate; pin library version |
| Oracle math rewrite changes rounding in one edge case | Med | High (credit balance drift) | T4 differential test across 10⁶ cases |
| v1 executor still in use past M3 | Med | Med | M4 conditional; check CometBFT removal PR status before M3 |
| Hidden `big.Int` in signing / hashing paths (`ChainID` aside) | Low | High | Repo-wide grep after M3; audit `signatures.yml` |
| Nullable `*Amount` pointer ergonomics introduce nil-deref | Med | Med | Lint rule / helper `IsSet(a *Amount) bool` |
| Third-party consumers unmarshal into `*big.Int` from our binary | Low | Med | Decimal-string JSON unchanged; binary consumers are us |
| `uint256` vendoring / supply chain | Low | Low | Vendor; cache; review once |

## 8. Rollback

If a critical bug surfaces post-activation:

- Binary rollback is safe up to the activation height (wire format identical).
- After activation, rolling back requires a coordinated downgrade and a
  `ReadAmount`-produced-but-valid amount won't decode wider than 32 bytes on
  old nodes (old nodes never wrote >32 anyway — but this is why T3 matters).

No state surgery required in either direction.

## 9. Out of scope

- Credit balances (stay `uint64`).
- `ChainID` in `TypedDataSignature` (EVM semantic, stays `*big.Int`).
- User-defined token precision cap (already 18, unchanged).
- Public API contract changes.
- Replacement of `big.Int` in non-amount contexts (merkle math, signature
  scalars, etc.).
