# Bootstrap v2: anchor-walk over DN, with a (DN, BVN) target

Supersedes the original `minimum-data-node-bootstrap.md` and replaces
the first draft of this file. The revised model emerged from
explicit code-archaeology of how Accumulate actually anchors, signs,
and stores its proof structure (#3953 → #3984 → review →
re-think). What follows is the load-bearing model. Earlier sections
of v2 (account puller, BPT-root convergence, run-handoff,
advertisement, heartbeat) survive nearly unchanged; the trust phase
needs significant rework.

## The model

A node always runs both **DN and one BVN**. Bootstrap targets the
(DN, BVN) pair atomically — you can't bootstrap one half. Today
there's one BVN; the design generalizes when more land.

Like Bitcoin's headers, **anchors are the entire historical proof
structure**. Transaction-execution rules are the protocol's problem
during build-time; the launcher does not re-run them. Everything
hangs on the anchors:

- Each Accumulate **major block** is the canonical checkpoint for a
  partition's state at that boundary (~12 hr cadence). Major-block
  count for mainnet's lifetime is ~thousands, not millions.
- At each DN major-block boundary, the DN executor produces a
  `DirectoryAnchor` (DA) per BVN destination. Each DA carries DN's
  `PartitionAnchor` (with DN's `RootChainAnchor` and `StateTreeAnchor`
  — invariant across destinations at a given boundary) plus the
  per-BVN `Receipts[]`.
- DN's validators sign `seq.Hash()` of the `SequencedMessage`
  wrapping each DA. (`internal/core/crosschain/anchoring.go:212–221`
  + `internal/core/execute/v2/block/msg_block_anchor.go:204–207`.)
- BVN→DN `BlockValidatorAnchor`s also exist — they carry the BVN's
  `StateTreeAnchor` and live in `dn.acme/anchors`'s main chain. They
  are not the trust-phase artifact; they're proven *transitively*
  via DN once Phase A succeeds.

## The proof, in one sentence

> A peer's claimed (DN, BVN) state at the latest major-block
> boundary is correct iff (a) walking back through DN-validator-
> signed DAs to genesis terminates at the binary-pinned DN
> `StateTreeAnchor`, *and* (b) the launcher can locally reconstruct
> the DN BPT to match the latest verified DN `StateTreeAnchor`,
> *and* (c) it can locally reconstruct the BVN BPT to match the
> `StateTreeAnchor` of the BVN→DN anchor sitting in trusted DN state.

That's it. No genesis-snapshot pin, no validator-set-hash pin, no
per-account back-walk, no historical user-signature replay.

## Where the bytes live

The proof structure is sharded across BVN anchor pools. Concretely
for an Apollo-targeted bootstrap:

- DAs DN sent to Apollo: `apollo.acme/anchors`, **main chain**.
- DN-validator signatures on each of those DAs:
  `apollo.acme/anchors`'s transaction signature chain for that DA
  txn (received and recorded via `BlockAnchor` messages).
- DN's outgoing-anchor sequence numbers: tracked on
  `dn.acme/anchors`'s `AnchorSequenceChain` — informational; the
  signed artifact + signatures live on the BVN side.
- BVN→DN anchors for any BVN: `dn.acme/anchors`, main chain.
- DN operators key page: `dn.acme/operators/1`.
- DN major-block index chain: `dn.acme/anchors`'s `major-block`
  chain. Useful for big-jump pagination during Phase A — but the
  *signed material* the walker verifies is on the chosen BVN's
  side.

## Phases

### Phase A — DN trust

Walk backward the chosen BVN's anchor pool main chain, filtering
for entries whose source is `dn.acme` (i.e., DAs received from DN).
For each:

1. Resolve the DA txn (the entry's value when expanded).
2. Read the txn's signature chain → `BlockAnchor` messages, each
   wrapping a single DN-validator's `KeySignature` over the DA's
   `SequencedMessage`.
3. Reconstruct the `SequencedMessage` (it's directly inside the
   `BlockAnchor`); verify ≥2/3 of DN's *current* operators key page
   signed `seq.Hash()`.
4. Extract DN's `PartitionAnchor.StateTreeAnchor` from the DA — this
   is the value Phase B will converge against for the latest
   verified entry.
5. Continue walking back. Terminate at DN major-block 1: the
   genesis DA's `PartitionAnchor.StateTreeAnchor` must equal the
   binary-pinned value for this network.

The major-block index chain on `dn.acme/anchors` lets the walker
skip in major-block-sized increments rather than minor-block
increments — same proof shape, far fewer iterations.

When operators rotate (none have happened yet on mainnet), a
signature-verification failure at some boundary triggers
re-resolving DN operators state at that older time and continuing
with the older set. `keybookat.ApplyDelta` already provides the
delta application; the source-side `OperatorsDeltaAt` extracts
`UpdateKeyPage` operations from the operators-page main chain in
the right block range.

### Phase B — DN data

Pull DN's complete account set into a local database. Run
`UpdateBPT()`. The root must equal the `StateTreeAnchor` of the
latest verified DA from Phase A. Single byte comparison; fail
closed otherwise.

This is exactly the convergence v2's `convergence` package already
implements — applied to DN.

### Phase C — BVN data

Read the BVN→DN anchor for the chosen BVN out of *now-trusted* DN
state. (It's an entry in `dn.acme/anchors`'s main chain; its
content is committed to DN's BPT, which we just verified matches
the verified `StateTreeAnchor`.) That anchor's
`PartitionAnchor.StateTreeAnchor` is the BVN's BPT root, trusted
by transitivity.

Pull the BVN's complete account set. Run `UpdateBPT()`. The root
must equal that anchor's `StateTreeAnchor`. Second convergence,
same shape.

### What gets persisted

```
bootpersist.Artifact {
  Network                string
  BVN                    string  // partition name (e.g., "Apollo")
  DNGenesisStateTreeAnchor [32]byte  // mirrors the binary pin
  DNVerifiedAnchor       [32]byte
  DNVerifiedMajorBlock   uint64
  BVNVerifiedAnchor      [32]byte
  BVNVerifiedMajorBlock  uint64
  State                  ...        // BOOTING/ACTIVE/COMPLETE
  Cursors                ...
}
```

`accumulated run` reads this artifact and restores the nodestate
machine in ACTIVE iff both anchors are populated. Pin override at
load time compares against `DNGenesisStateTreeAnchor`.

## Pin shape

`pinned.Pin` reduces to one field: the DN's `StateTreeAnchor` at
major-block 1. This is per-network, BVN-agnostic — same pin works
no matter which BVN the launcher is targeting. Operator override
via `--genesis-state-tree-anchor` flag for development networks
where the binary pin is empty.

## Mapping onto existing v2 code

What survives nearly unchanged:

- `bootstrap/completeness` — the contract pinning what the puller
  must round-trip. Independent of trust model.
- `bootstrap/pull` (`Source` / `DBSource` / `APISource`) — works
  for any account set; called twice (once for DN, once for BVN).
- `bootstrap/convergence` — `Verify(batch, expected)` is fine
  shape. Called twice.
- `bootstrap/bootpersist` — schema additions; the
  Save/Load/Peek/atomic-write machinery is right.
- `bootstrap/nodestate` — orthogonal to trust model.
- `bootstrap/keybookat` — `ApplyDelta` and `EncodeOperation` work
  unchanged. Just narrowed to DN operators.
- `cmd/accumulated/run/bootstrap.go` — detection + restore + the
  advertisement + heartbeat plumbing. Schema rename of artifact
  fields propagates here.
- The wire-format `BootstrapAdvertisement` on NodeInfo. The
  advertisement's `VerifiedAnchor` field can hold either DN or BVN
  — needs a small decision (probably DN, since that's the
  network-shared anchor).

What needs significant rework:

- `bootstrap/headerwalk`:
  - `Header` shape: carries DN's `PartitionAnchor` + the `SequencedMessage` for verification. `AnchorTxHash`-as-CanonicalHash is wrong; replace with explicit `Signable` field that's the `SequencedMessage`.
  - `HeaderSignature` gains a `Signable` field for live-network use; raw-fields path stays for synthetic tests.
  - `verifySig` delegates to `KeySignature.Verify(s, sig.Signable)` when `KeySignature` is non-nil.
  - `APISource` constructor takes the chosen-BVN's anchor pool URL. Walks it backward filtering DAs by `Source == dn.acme`. Operators page is hardcoded to `dn.acme/operators/1`. Major-block index chain on `dn.acme/anchors` is used only for skip-pagination, not as the primary source.
  - The current "MajorAnchor"-flavored API I sketched earlier doesn't fit; back to per-major-block iteration but with the corrected source.
  - `OperatorsDeltaAt` queries DN's operators-page main chain in the relevant block range. Today: returns nil because no rotations have occurred.

- `bootstrap/pipeline`:
  - `Bootstrap()` runs A → B → C in sequence.
  - Returns two `(StateTreeAnchor, MajorBlockIndex)` pairs in the result.
  - The single `pull.Source` becomes two pulls against two account sets — the DN's minimum set and the BVN's. `Options` accommodates that.

- `pinned`:
  - `Pin{ValidatorSetHash, PinnedHeight}` → `Pin{DNGenesisStateTreeAnchor [32]byte}`.

- `cmd/accumulated/cmd_bootstrap.go`:
  - Flags: `--network`, `--data-dir`, `--bvn` (replaces `--partition`; defaults to the only BVN), `--genesis-state-tree-anchor` (override). Drop `--pinned-hash` / `--pinned-height` / `--skip-quorum` (the last because validator quorum check is no longer optional in production; dev networks use the override flag).
  - Runs the two-phase pipeline. Saves the new-shape artifact.

- `bootpersist.Artifact`:
  - Drop `PinnedValidatorSetHash`, `PinnedHeight`, single
    `VerifiedAnchor`/`VerifiedHeight`.
  - Add `BVN`, `DNGenesisStateTreeAnchor`, `DNVerifiedAnchor`,
    `DNVerifiedMajorBlock`, `BVNVerifiedAnchor`,
    `BVNVerifiedMajorBlock`.
  - `FormatMajor` bumps to 2.

- Tests: `headerwalk` synthetic-fixture tests adapt to new
  `Header` shape; `pipeline` integration test runs two phases;
  `bootstrap_lifecycle_test` saves the new artifact shape;
  Docker E2E asserts both anchors.

## Execution plan

Phase 0 — this doc (done with this commit).

Phase 1 — `pinned` shape change. Smallest unit; foundation for all
later phases.

Phase 2 — `headerwalk.Header` + `HeaderSignature` shape change.
Update walker tests to match.

Phase 3 — `headerwalk.APISource` rewrite around the BVN-side
walking pattern. New tests against fake querier.

Phase 4 — `bootpersist` schema bump and field rework.

Phase 5 — `pipeline.Bootstrap` two-phase wiring; pipeline tests
adapt.

Phase 6 — `cmd/accumulated/cmd_bootstrap.go` rewrite. Flag and
artifact-shape updates.

Phase 7 — `cmd/accumulated/run/bootstrap.go` artifact-field
rename; lifecycle test updates.

Phase 8 — Docker E2E script update for new artifact assertions.

Each phase is a separate commit + test green at the end. The
`completeness`, `pull`, `convergence`, `nodestate`, `keybookat`
packages should not move during this rewrite — they're already
shape-correct.

## Out of scope for this rewrite

- Pin table population — release-process item, not code.
- Live mainnet smoke — depends on populated pin or operator
  override.
- Cursor updates during the walk for crash-resume mid-bootstrap.
- Multi-BVN selection logic; the launcher takes a single `--bvn`
  flag and uses it.
- The "rotation has happened" path in keybookat. Code is in place
  but exercised only by unit tests. When mainnet first rotates
  validators, this path will need real-network smoke.
