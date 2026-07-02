# Collection-proof cross-partition sync — state of work (#4048)

_Last updated: 2026-07-02._

## Where the code is

- Branch `4048-collection-proof-sync`, off current `main`.
- Baseline commit pulled the **proof + sync/recovery core (11 non-test files)** from
  branch `3660-activate-collection-proofs` (#3660): `proof_service.go`,
  `proof_integration.go`, `conductor_proof.go`, `conductor_gap_recovery.go`,
  `conductor_recovery.go`, `recovery_core.go`, `recovery_health.go`,
  `recovery_session.go`, `destination_state.go`, `sequence_tracker_simple.go`,
  `types.go`. This is a **reference baseline**, not a working port.

## Triage (build against current main)

`go build ./internal/core/execute/v2/crosschain/...` → 11 errors, two roots:

1. **Everything is methods on `CrossChainConductor`** — defined in the *un-pulled*
   `conductor.go` (22 KB) on #3660, plus `MessageType` from `unified_transport.go`.
2. **A new protocol message type** — `messaging.RecoveryRequest` /
   `MessageTypeRecoveryRequest = 13` — added on #3660, **absent from `main`**.

Conclusion: #3660 built a **parallel `CrossChainConductor`**, and the proof/recovery
files are entangled with it.

## Architecture decision: rework onto the existing path

Do **not** import the parallel conductor. Re-attach the collection-proof +
gap-recovery *logic* to the existing `internal/core/crosschain.Conductor` + the v2
executor. The `recovery_*` / `conductor_gap_recovery` files are salvageable as
logic; `conductor.go` / `unified_transport.go` are **not** imported.

## Dual-mode (individual + collection proofs): FEASIBLE

Goal: nodes support **both** the current per-message individual proof and the new
collection proof, so we ship compatibly and switch to batching later.

- Both proof types terminate at the **same trust root** — a DN anchor accepted via
  threshold-signed `BlockAnchor`s. An individual `merkle.Receipt` is the degenerate
  tail of a `merkle.ReceiptList`, so one validator check covers either.
- **Blocker #3660 never solved:** the wire type `protocol.AnnotatedReceipt` carries
  only `*merkle.Receipt` — no `ReceiptList`. #3660's `createCollectionProof` is a
  **stub**: it builds a `ReceiptList` then discards `MerkleState`, `Elements`, and
  `ContinuedReceipt`, shipping only the terminal `Receipt` (`IsCollection` is
  cosmetic). So its "collection proofs" were never real on the wire, and
  `proof_service.go`'s collection path is **not portable as-is**.
- Dual-mode requires: (a) a **wire variant** carrying an optional
  `*merkle.ReceiptList`; (b) a **validator branch** in `msg_synthetic.go`
  (`ReceiptList.Validate` + `Included(msgHash)` + the shared "anchor ∈ DN anchor
  pool" check); (c) **executor-version gating**.

## Indices are proven — no extra machinery

`ReceiptList` carries the counted `merkle.State` (`State.Count`). Replaying
`Elements` onto that counted start state to reproduce the committed anchor binds each
element's **absolute index** (`Start + j`). This closes the "a merkle state has
multiple theoretical solutions" ambiguity → **sequence numbers are proven for free**.
The `receipt_list.go:74-78` comment ("does not necessarily prove the indices… salt
with index") is **stale** for this construction and should be corrected.

## Recovery model

The latest **cumulative** collection proof commits to every past entry's hash, order,
and index; validate it **once** against the DN anchor the node already trusts.
Recovery is then **signature-free data collection**: pull the missing raw entries,
verify each via `Included`, apply in proven order. This dissolves the recovery bugs
found in the #4047 review — synthetic-before-anchor terminal failure, single-signature
healing that can't reach threshold (the "tx already exists in cache" flood), and the
strict in-order wedge.

## Rollout (phased, ExecutorVersion-gated)

Add a new `ExecutorVersionV2<Name>` (current head: `V2Jiuquan = 8`, `VNext = 9`).

1. **Dormant binary rollout** — accept-both + emit-collection code shipped but gated
   behind `V2<Name>Enabled()`; network stays on individual proofs. Rolling, safe.
2. **Activate accept-both** — operator-quorum `ActivateProtocolVersion` (signed by
   `dn.acme/operators`) flips the network-wide global `ExecutorVersion` atomically at
   one block. Because the version is a network global, cross-partition (A→B) is safe:
   both sides activate together, no emit/accept skew.
3. **Switch emission to batching** — same activation or a later `N+1` for margin.

Individual proofs remain a **permanent** valid fallback. Rule: **accept must never lag
emit** — gate accept at version N, emit at N (atomic) or N+1 (conservative).

## Next steps

1. Design the wire variant (optional `ReceiptList` on the proof) + codegen.
2. Implement a **real** collection-proof builder (thread the merkle chain into
   `GetReceiptList`; #3660's is a stub).
3. Add the `msg_synthetic.go` validator branch (accept both proof forms).
4. Salvage `recovery_*` / `conductor_gap_recovery` onto the existing `Conductor`.
5. Add `ExecutorVersionV2<Name>` + the `*Enabled()` gates (accept vs emit split).
6. Fix the terminal-failure trap in `msg_synthetic.go` (record missing-anchor as
   *pending/retryable*, not *failed*) so recovery can re-apply.
7. Add bounds on `ReceiptList` length / the pending set (DoS).

Related: #3660 (activate collection proofs), #4047 (synthetic delivery + discovery).
