# Fast validator deployment — header-first sync with account-state proofs

Status: design reviewed with Paul (2026-07-12), issue #4058. Decisions: trust
anchor = pinned genesis hash with a full major-spine walk; validator-quorum
verification on the major spine and the sync-epoch anchor — everything else is
carried by collection proofs into those verified roots (per-anchor quorum
checks on the dense minor run add nothing once account proofs terminate in a
quorum-verified root); services on the private sequencer service (0xF001),
public promotion later if wanted; closes the documented remaining gap of #4057
("catch-up past the GC horizon needs ledger replay or state sync"). The result is a deployment path: bring up
a new (or long-offline) node without replaying history, with cryptographic
verification at every step.

## The method

Three phases, coarse to fine:

1. **Major-block spine.** Starting from a trust anchor, validate the signature
   headers of the major blocks forward to the present. Major blocks are few
   (2/day on mainnet), so this walk is cheap regardless of chain age.
2. **Minor blocks to present.** From the last major block, extend verification
   to the current tip: quorum-verify the sync-epoch anchor and bind it into
   the spine by receipt. Intermediate minor blocks need no individual quorum
   checks — the account proofs of phase 3 terminate in these verified roots.
3. **Account states with proofs.** Iterate the account states, building each
   account's proof up to the latest verified minor-block root — a collection
   proof proves all the elements of an account into that root — while
   continuing to consume new blocks as transactions are issued, so the node
   converges on the live tip instead of chasing it.

At the end the node holds: a verified header spine from the trust anchor to the
tip, and a database whose every account state is proven into the state-tree root
(`StateTreeAnchor`) of a verified block. It then joins consensus normally.

## What the chain actually gives us (code reality)

### Major blocks are unsigned index entries — the signed object is the anchor

- The major-block chain is an index chain on the anchor pool
  (`Account.MajorBlockChain()`, internal/database/model_gen.go:492), entries are
  `protocol.IndexEntry{Source, RootIndexIndex, BlockIndex, BlockTime}`
  (protocol/types_gen.go:460), recorded by `recordMajorBlock`
  (internal/core/execute/v2/block/block_major.go:63). **No root hash and no
  signatures in the entry itself.**
- The signed artifact is the per-minor-block anchor transaction:
  `PartitionAnchor{RootChainAnchor, StateTreeAnchor, RootChainIndex,
  MinorBlockIndex, MajorBlockIndex}` (protocol/types_gen.go:666), wrapped in a
  `messaging.SequencedMessage` whose hash the validators ED25519-sign
  (`signTransaction`, internal/core/crosschain/anchoring.go:259). The quorum is
  archived as `AccountTransaction.ValidatorSignatures()`
  (internal/database/model_gen.go:1277); the threshold check is `txnIsReady`
  (internal/core/execute/v2/block/msg_block_anchor.go:270) against
  `GlobalValues.ValidatorThreshold` (pkg/types/network/validators.go:82).

So "validate the signature header of a major block" concretely means: **verify
the validator-quorum-signed anchor of the minor block that closed the major
block** (reachable via the index entry's `RootIndexIndex` → root index chain,
same resolution as `getMajorBlockBounds`, internal/api/v3/utils.go:84).

### Validation rule: quorum on the spine, collection proofs for everything
### else (revised 2026-07-12)

Quorum signatures are verified where they carry trust; Merkle proofs carry
everything else:

- **Spine anchors (quorum-verified):** verify the archived `BlockAnchor`
  signatures against the validator set *as of that block*. The validator set is
  a versioned data entry on `dn.acme/network` (`GlobalValues.ParseNetwork`
  enforces monotonic versions, pkg/types/network/globals.go:188), and every
  change travels inside `DirectoryAnchor.Updates` (protocol/types_gen.go:326).
  The set-at-any-height comes from the network account itself: `dn.acme/network`
  is an account like any other, so **one collection proof over its elements
  yields the complete, ordered, spine-proven validator-set timeline** — updates
  landing between major boundaries included. Verify each spine anchor against
  the timeline as of its height; the timeline's own proof terminates in the
  spine, closing the induction. No "validator set at height N" API needed.
- **The sync-epoch anchor (quorum-verified):** the recent block whose
  `StateTreeAnchor` all account proofs target is verified against the *current*
  validator set from the timeline.
- **Everything between (proof-carried):** no per-anchor quorum verification of
  the dense minor run. An account's collection proof (`merkle.ReceiptList`,
  pkg/database/merkle/receipt_list.go:98) proves **all the elements of the
  account** — every chain entry, in order, at proven absolute indices — up to a
  DN-anchored root, and that root is bound by continuation receipt into a
  spine- or epoch-verified anchor. Because every proof terminates in a
  quorum-verified root, independently re-checking the tens of millions of
  intermediate anchor quorums adds nothing: tampering with any intermediate
  block would break the Merkle path. Receipts carry the structure; the spine
  and epoch quorums carry the trust.
- **The tail is role-dependent (decision 2026-07-12):** updates from
  transactions *beyond* the spine — after the last major block, not yet
  covered by any spine anchor — split by what the node is deploying as:
  - **Validator:** tail updates need **full quorum verification** on the
    anchors that carry them. A validator signs and validates against the
    current set and globals immediately; it cannot treat them as provisional.
  - **Follower (willing to wait):** tail updates are held as provisional and
    applied once the next major block's spine anchor covers them by proof.
    The follower serves verified state up to its spine coverage and simply
    lags the tail.

**Cost note:** this keeps the client O(spine anchors + one quorum at the epoch
+ receipts), instead of O(all anchors × quorum size). Mainnet's ~30.6M minor
blocks never need their quorums shipped or re-verified. It also sidesteps the
#4056 proof-healed-anchor case (an anchor authorized by proof has no archived
quorum) — irrelevant here, since intermediate quorums aren't consulted.

### Account states: the primitives exist, the API does not

- Account state → BPT root: `(*Account).StateReceipt()`
  (internal/database/bpt_account.go:67) = state hasher receipt combined with
  `BPT().GetReceipt(key)` (pkg/database/bpt/bpt_receipt.go:18).
- BPT root → block: the BPT root is **not** in the root chain; it is the
  `StateTreeAnchor` field, a signed sibling of `RootChainAnchor` in the anchor
  body (set together at internal/core/crosschain/anchoring.go:187-189). So the
  binding "account state → block" = BPT receipt to `StateTreeAnchor` **plus**
  the verified anchor for that block. No new Merkle commitment is required —
  phase 1/2 already verified the anchor.
- Hydration: `snapshot.FullRestore` / `RestoreVisitor.VisitAccount`
  (internal/database/snapshot/restore.go:34,58) write received account states
  directly into a `database.Beginner` — intact and reusable.
- Wire scaffolding: `BPTSyncRequest`/`BPTSyncResponse`/`BPTEntry`
  (pkg/consensus/gossip/bpt_sync.go:16-42) already define fetch-state-trie-
  entries-by-key-hash, but nothing serves it yet.

### Why this is needed at all (the GC horizon)

Cert-sync serves rounds only from the in-memory DAG
(`handleSyncRequest` → `dag.GetRound`, pkg/consensus/primary/cert_sync.go:420);
`DAGGCDepth` = 10,000 rounds (pkg/consensus/config/config.go + consensus.go:42)
≈ 16 minutes at 10 rounds/s. A node further behind gets "Sync request matched
nothing" forever. Certificates are broadcast exactly once. There is no fallback.

## Design

### Serving side — three new range services (model: `SequenceRange`)

The private sequencer service pattern (#4048's `SequenceRanger`,
internal/api/private/api.go:33, impl internal/api/v3/sequencer.go:412) is the
template: authenticated range request → one shared collection proof.

1. **`MajorHeaderRange(start, end)`** → for each major block: the `IndexEntry`,
   the closing minor block's anchor transaction (`DirectoryAnchor` /
   `BlockValidatorAnchor` body), and its archived `BlockAnchor` signature
   messages (recovered the way `loadTransactionSignaturesV2` does,
   internal/api/v3/load.go:184). Neither `MajorBlockRecord` nor any current
   query returns roots or signatures — this is new surface.
2. **`MinorRootRange(start, end)`** → root-chain entries for a run of minor
   blocks (each with its `StateTreeAnchor`), bound by one `ReceiptList` into a
   named spine- or epoch-verified anchor. Built from `GetReceiptList` +
   `getRootContinuation` (internal/api/v3/sequencer.go:360). No signature sets
   ride along — trust comes from the anchor the receipt lands in. In the
   minimal deployment flow this is only needed to bind the sync epoch's
   `StateTreeAnchor` (and any specific block a proof targets) into the spine;
   the epoch anchor's own quorum is served with it (archived `BlockAnchor`
   messages, recovered the way `loadTransactionSignaturesV2` does,
   internal/api/v3/load.go:184).
3. **`AccountStateRange(bptKeyStart, bptKeyEnd, blockRoot)`** → a page of BPT
   entries (full account state, not just hashes) each with its BPT receipt to
   the `StateTreeAnchor` of the named block. Enumeration via
   `IterateAccounts`/`ForEachAccount` (internal/database/bpt.go:84,91) + per-key
   receipts; batched like `ReceiptList` batches chain entries
   (`MaxReceiptListElements` = 4096 as the page-size precedent). For account
   *history*, the same call (or a follow-up request) serves a `ReceiptList`
   over the account's chains — one collection proof proving all the elements
   of the account into the verified root — so history fills in lazily after
   the node is running, with the same trust guarantee as the state itself.

**Historical-root problem:** `BptReceipt` works only on the current committed
BPT (bpt_account.go:54 rejects dirty state), and the BPT advances every block.
Rather than build receipt-at-height, the server pins a **sync epoch**: it
answers all `AccountStateRange` pages against one recent block's BPT (a held
batch/snapshot view), and the client closes the gap from that epoch to the tip
with block replay (phase 3's "updating accounts as transactions are issued").
The epoch only needs to be stable for the duration of one client's state pull.

### Client side — the `accumulated` deployment path

New subcommand (the vestigial slot is cmd/accumulated/cmd_sync.go; the CometBFT
`sync snapshot` path is inert under DAG-BFT) plus an automatic fallback:

- **Trust anchor:** the pinned genesis-snapshot hash per network, compiled into
  the binary — the only out-of-band trust input (same pinned artifact as
  #3953, but walked forward not backward). The full spine is always walked;
  an operator may supply an alternate anchor for private networks.
- **Phase 1:** pull `MajorHeaderRange` from genesis to present, plus the
  `dn.acme/network` account's collection proof (its elements ARE the
  validator-set timeline, spine-proven). Verify each spine anchor's quorum
  against the timeline as of its height. Output: verified spine +
  validator-set timeline + current globals.
- **Phase 2:** verify the sync-epoch anchor's archived quorum against the
  current validator set from the timeline, and bind its `StateTreeAnchor`
  into the spine via `MinorRootRange`. No per-block quorum walk. Output: a
  verified epoch root that all account proofs will target.
- **Phase 3:** page through `AccountStateRange` against the epoch root,
  verifying each BPT receipt, hydrating via `RestoreVisitor`. Concurrently
  buffer live blocks (join gossip immediately; `StateHashMessage` signed roots,
  pkg/consensus/types/state_verification.go:19, cross-check the tip). After
  hydration, replay buffered blocks epoch→tip; once within `DAGGCDepth` of the
  live round, the existing cert-sync/round-catchup path
  (`requestRoundCatchUp`, pkg/consensus/primary/vote_handler.go:406) finishes
  the job. Then start/resume consensus participation — applying the tail rule:
  a node joining as a **validator** quorum-verifies the anchors carrying any
  network-account updates beyond the spine before acting on them; a
  **follower** may hold tail updates as provisional until the next major
  block's spine anchor covers them by proof.
- **Automatic fallback:** in the out-of-window branch of `vote_handler.go`
  (:308-311), when the round gap exceeds `DAGGCDepth`, trigger this sync
  instead of the futile `RequestRounds` loop. That converts #4057's "wedged
  past the horizon" into a self-healing path — outage recovery and fresh
  deployment become the same code.

### Edge cases carried over from the exploration

- Empty minor blocks are absent from the BlockLedger — range walkers must
  tolerate index gaps (the `OmitEmpty` handling in queryMinorBlockRange2 is the
  precedent).
- Main chains span the 2025-07-13 database reorganization; the spine walk needs
  the same special-casing #3953 deferred.
- BVN nodes need this per-partition (DN + their BVN); the DN spine also
  receipts BVN roots (`PartitionAnchorReceipt.RootChainReceipt`), so a BVN
  node's minor-root verification can ride the DN spine.
- Never serve `AccountStateRange` from a dirty batch; pin the epoch view.

## Phasing

1. **Serve + verify headers** — `MajorHeaderRange` service, client-side spine
   walk with validator-set induction, unit tests against a simulated network
   with validator churn.
2. **Epoch binding** — `MinorRootRange`: bind the sync-epoch anchor (and its
   `StateTreeAnchor`) into the spine by `ReceiptList`; verify its quorum
   against the induced current validator set.
3. **State transfer** — epoch pinning, `AccountStateRange`, RestoreVisitor
   hydration, BPT-root equality check against the epoch `StateTreeAnchor`.
4. **Live convergence + consensus join** — buffered-block replay, cert-sync
   handoff, the vote_handler fallback trigger, and the docker outage test:
   re-run the #4057 pause test but keep a node down long enough to cross the
   GC horizon (>10,000 Directory rounds), then verify it rejoins.
5. **Deployment UX** — `accumulated` subcommand, pinned trust anchors per
   network, progress reporting.

## Relationship to prior work

- **#4057**: this is the documented remaining scope (catch-up past GC horizon).
- **#4048/#4056**: collection proofs are the proof carrier for phase 2 and the
  precedent (`SequenceRange`) for all three range services.
- **#3953 (closed)**: same goal, opposite direction — that design back-walked
  every main chain to genesis; this one walks the header spine forward and
  proves state into it, which is O(majors + minors-since-last-major + accounts)
  instead of O(full transaction graph).
- **bootstrap-v3**: the bootstrap server remains discovery-only; state flows
  through the node API/gossip services above, discovered via bootstrap.
