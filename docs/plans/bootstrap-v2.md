# Bootstrap v2: validator-quorum-anchored, no historical replay

Supersedes `minimum-data-node-bootstrap.md`. Captures the corrected
model arrived at after #3953 — #3984 landed and we audited what the
proof actually proves.

## Why v2

v1 took "back-walk to genesis" as the core proof. That walk verifies
*user* signatures on historical *account-chain* transactions one at a
time. Three problems with that:

1. The trust anchor for state is the validator quorum on the block
   that committed the state, not user signatures on individual
   transactions. The protocol verified user signatures at the moment
   the transaction executed; the validators' quorum on the block is
   their attestation that all such checks passed. Re-running them
   from a launcher detects only protocol bugs, which the launcher
   has no recourse against.
2. Walking *every account's* main chain forces the launcher to fetch
   historical transactions, which is exactly the data the
   "minimum-data" goal was about not fetching.
3. Genesis as a hard terminator implies the launcher must reach it
   on every account that contains state. For the system ledger and
   long-lived ADIs, that is not minimal.

The right anchor is the validator quorum on a recent block. Once the
launcher trusts that block, the block's `StateTreeAnchor` is trusted,
and every account state whose hash sits in the BPT under that anchor
is trusted along with it. No per-account, per-transaction work needed.

## The model

Two phases that run concurrently, plus a one-line convergence check:

**Trust phase.** Walk block headers, verifying the validator quorum
signature on each. Walk direction is open — back from current to a
genesis or pin terminator, or forward from a pin to current — same
proof either way. Track operators-keybook state across the walk: when
an operators-keybook-touching transaction appears in a block, apply
its delta (UpdateKeyPage / AddCredits / etc.) so the next block's
quorum is verified against the current set. **No user signatures are
checked anywhere.** The block's quorum signature already certifies
that the operators-keybook update was protocol-valid.

Output: a verified current-block `StateTreeAnchor` and the sequence
of validator-set states.

**Data phase.** Pull every account's complete state. For each account
also request a BPT-membership proof. Don't validate the proof yet;
just store it. Multi-source: paginated BPT enumeration backfills the
whole minimum set; live-traffic listener catches any account
referenced by streaming transactions but missed by enumeration.
Convergence is on completeness, not on per-leaf checks.

**Convergence.** Run `UpdateBPT()` over the locally pulled state.
Read `GetBptRootHash()`. It must equal the trust phase's verified
`StateTreeAnchor`. One equality, fail closed.

When the equality holds:

- Every leaf in the local BPT was the network's leaf at that height
- Every account state in those leaves contains its chain anchors
  (Merkle roots per chain), so every entry hash on every chain is
  committed
- Every entry hash commits its transaction and signatures, so the
  whole transaction graph is implicitly proven without ever hashing
  it locally

That is the entire proof. No recursion, no terminator, no historical
signature replay.

## Architecture

```
                  ┌────────────────────────────┐
                  │  Block-header walker       │
                  │  (validator quorum +       │
                  │   keybookat operators      │
                  │   delta application)       │
                  └──────────────┬─────────────┘
                                 │ verified
                                 │ StateTreeAnchor
                                 ▼
  pull complete                ┌──────────────┐
  state for every       ─────▶ │ Convergence  │
  account in min set    ─────▶ │ (BPT match)  │ ─▶ ACTIVE
  + traffic listener    ─────▶ │              │
  + paginated BPT       ─────▶ └──────────────┘
                                 ▲
                                 │ local UpdateBPT
                                 │ root hash
                  ┌──────────────┴─────────────┐
                  │  Complete account puller   │
                  │  (Main + Directory +       │
                  │   Pending + chains)        │
                  └────────────────────────────┘
```

Four components. Three of them are essentially new. The fourth
(complete account puller) is what the existing `pullAccount` was
trying to be but stopped short of.

## What survives from v1

- `keybookat` forward replay, narrowed to operators-keybook deltas.
  The recursive-resolution / signature-checking half goes away.
  `ApplyKeyPageOperation` is the actual primitive — already exported.
- `bptproof.GetLeaf` and `bptproof.GetPage`. These are still the
  protocol-level primitives a peer serves to deliver leaves and the
  proofs the launcher stashes.
- `hydrator` skeleton + `loadtrack` — multi-source orchestration of
  the data phase. Throw out only the proof-checking parts; keep the
  scheduling.
- `bootpersist` — versioned envelope, `Peek` / `Load` / `Save`.
  Format adds a `VerifiedAnchor` field; everything else stays.
- `nodestate` machine + advertisement — orthogonal to the trust model
- `accumulated run` handoff — orthogonal
- `pinned` package — repurposed: stores `(height, validator-set-hash)`
  not genesis hash. Still empty until release-time population.
- Pipeline orchestration shell — `pipeline.Run` is just rewired

## Kill list

Remove outright:

- `internal/core/bootstrap/backwalk/verify.go` — user signature path
- `internal/core/bootstrap/backwalk/synthetic.go` — synthetic signer
  discovery
- `internal/core/bootstrap/backwalk/quorum.go` for individual-tx
  quorum checks (validator quorum lives at block level only)
- `internal/core/bootstrap/backwalk/backwalk.go` — Walker concept
  itself goes; replaced by header walker
- The "two-rule classification" inside `verifyEntry`
- `signerForTransaction`, AccountAuth-based signer candidate lookup
- The per-tx signature pulling in `pipeline.pullAccount`
  (everything from #3977 about main-index chains for arbitrary
  accounts and SignatureSetEntry round-tripping)
- `IsUser` / `IsSynthetic` / `IsSystem` switch in the back-walker
- `BlockTimeFor` for arbitrary historical times — block headers
  carry their own time; we don't need a query
- SystemGenesis terminator detection (`isGenesisTerminator`)
- `keybookat`'s recursive verification path; keep only forward
  replay of operators-keybook deltas
- `bptproof` BPT-leaf-membership *re*-verification by the launcher
  outside of the convergence check (#3980 as written)

Reduce:

- `trustbundle.Bundle` collapses to {block header, validator
  signatures over header}. The `MinimumBootstrapSet`,
  `PerPartitionAnchors`, `ValidatorSet` payload all drop out — the
  validator set is reconstructed by the header walker from operators-
  keybook deltas, the bootstrap set is operator-configurable, and
  there is exactly one anchor (the verified current-block
  StateTreeAnchor) instead of a per-partition list.
- The trust-bundle service surface (#3983) reduces to "give me the
  signed current header." Producer/Cache shape stays but holds a
  much smaller payload.

Keep, with narrower scope:

- `keybookat.Resolve` / forward replay — operators-keybook only
- `bptproof.GetLeaf` / `GetPage` — used by data phase only
- `bootpersist` — add `VerifiedAnchor [32]byte` to `PinBlock`

## What we add

1. **Block-header walker.** New package
   `internal/core/bootstrap/headerwalk`. Single Walk function that
   takes a starting (block, validator-set) — either the pin or the
   tip — and produces a verified terminal block plus the validator-
   set rotation log. Per block: verify quorum signatures, then if
   the block contains operators-keybook ops, apply them via
   keybookat to produce the next block's validator set.

2. **Complete account puller.** Replaces `pullAccount`. Pulls
   `Main`, `Directory`, `Pending`, every chain's current state and
   anchor, and any per-account-type state required by the
   production observer (the partition ledger's `Events` BPT root,
   for example). Whatever set of fields the production observer
   reads in `hashState` is what gets pulled; the puller is
   defined by what `bpt_prod.go` consumes.

3. **Local BPT reconstruction.** `database.Batch.UpdateBPT()` over
   the pulled state, reusing the production observer. No custom
   hashing routine — reuse production code by definition.

4. **Convergence.** `localRoot == verifiedAnchor`. Fail closed.

## Execution plan

Phase 0 — preparation:
- Branch `bootstrap-v2` off main
- Doc + kill list (this file)

Phase 1 — completeness baseline:
- Test that pins exactly which fields per account type the production
  observer hashes
- Identifies the puller's required surface

Phase 2 — complete account puller:
- Implementation against a real network
- Test: pull from a fixture, run UpdateBPT, BPT root matches the
  fixture's pre-computed root

Phase 3 — block-header walker:
- Validator-quorum check per block
- Operators-keybook delta application via narrowed keybookat
- Forward and backward walks

Phase 4 — pipeline rewire:
- New pipeline.Run runs trust phase + data phase concurrently,
  converges on BPT root match

Phase 5 — kill v1:
- Remove the wrong-scoped packages
- Reduce trust bundle to header + signatures
- Update bootpersist with VerifiedAnchor

Phase 6 — capstone:
- E2E against the docker stack (corrected from the broken script)

Each phase is its own branch + explicit-merge into bootstrap-v2,
matching the workflow we used on v1.

## Branch strategy

`minimum-bootstrap-launch` is preserved in history for reference but
not advanced. v2 work happens on `bootstrap-v2` branched off `main`.
Surviving v1 packages are imported by selective cherry-pick or
re-creation, not by inheriting the v1 branch.

## What this leaves out

- **Optional historical audit.** Operators who *want* to verify
  every user signature on every transaction since genesis can run
  the v1 back-walker as an audit tool. It's not part of bootstrap.
  Probably belongs in `tools/` or as a `--audit` flag on a separate
  command. Out of scope for v2.
- **Trust bundle relay.** A node that's not a validator caching and
  re-serving the latest signed header is straightforward but separate
  from launcher correctness.
