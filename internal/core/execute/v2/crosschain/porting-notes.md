# Collection-proof cross-partition sync — porting notes (#4048)

## Provenance

The `.go` files in this directory were extracted verbatim from branch
`3660-activate-collection-proofs` (issue #3660, "Activate Collection Proofs")
as the **starting baseline** for the rework tracked in #4048. They are the
proof + sync/recovery core of that branch's `crosschain` package — NOT the
whole CrossChainConductor. The conductor inbound/outbound/transport/http/pause/
metrics files and the ~30 test files remain on #3660 for reference.

Files pulled:

- `proof_service.go`          — collection-proof creation/validation (`GetReceiptList`)
- `proof_integration.go`      — integration layer
- `conductor_proof.go`        — proof creation methods
- `conductor_gap_recovery.go` — gap detection + recovery driver
- `conductor_recovery.go`     — recovery orchestration
- `recovery_core.go` / `recovery_health.go` / `recovery_session.go`
- `destination_state.go`      — per-destination sequence state
- `sequence_tracker_simple.go`— sequence tracking
- `types.go`                  — shared types

## Status: BASELINE, does not build as-is

These files depend on `crosschain` conductor files that were intentionally
NOT pulled, and on #3660 APIs that may have drifted from current `main`. This
commit is a reference baseline to rework against, not a working port. Do not
expect `go build ./...` to pass on this commit.

## The rework (see #4048)

Model:
- Steady-state partition messaging carries **1+ transactions** per message,
  batched under one collection proof.
- **Healing = pulling chain + entries to fit into an existing (collection)
  proof.** The latest cumulative proof already commits to every past entry's
  hash, order, and index; recovery only re-collects raw data to fill gaps,
  verified via `merkle.ReceiptList.Included`. No per-message signatures, no
  threshold re-gathering, no per-anchor dependency, no in-order wedge.

Substrate already in `main`: `pkg/database/merkle/receipt_list.go`
(`ReceiptList`, `GetReceiptList`, `Included`, `ContinuedReceipt`). Indices are
proven via the counted `MerkleState` (`State.Count`) — the `receipt_list.go`
"does not prove indices" comment is stale for this construction.

Work items:
1. Verify/repair the `GetReceiptList` merkle-chain wiring (`COLLECTION_PROOF_FIX.md`
   on #3660 documented a nil-manager fallback; branch HEAD passes
   `req.SourceChain.Inner()` — confirm).
2. Build & extend the cumulative proof as head advances; keep `ContinuedReceipt`
   chained to the current DN anchor.
3. Signature-free data-sync endpoint ("serve chain entries X..Y") + retention
   guarantee; receiver apply-path decoupled from the per-message signature/
   anchor executor path.
4. Executor-version gate + mainnet compatibility.
5. Fix the terminal-failure trap in `msg_synthetic.go` (record missing-anchor
   as pending/retryable, not failed) so recovery can re-apply.

Related: #3660, #4047.
