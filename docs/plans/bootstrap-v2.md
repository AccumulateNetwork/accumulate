# Bootstrap v2: sync to a tracking node, with a (DN, BVN) target

This doc supersedes the original `minimum-data-node-bootstrap.md` and
the prior drafts of this file. It describes the bootstrap launcher
as a **sync protocol**, not a one-shot proof. The earlier drafts
described a single-pass "verify, pull, converge" pipeline; the
real model is closer to how Bitcoin's IBD or Tendermint's state-sync
works — establish trust, then catch up to tip, then track.

## What the launcher is

A node that participates in Accumulate runs both **DN and one BVN**.
Bootstrap targets that (DN, BVN) pair atomically. Today there's one
BVN; the design generalizes when more land.

The launcher is a **passive gossip consumer plus selective puller**
during boot. It receives gossip from peers, applies messages to its
local state, fetches account state on demand, and enumerates the
long-tail BPT in the background. It does **not** participate in P2P
service routing or respond to peer requests; it consumes only.
"ACTIVE" is the moment it has enough state to flip to a full P2P
participant — at which point `accumulated run`'s existing node
machinery takes over.

## The proof model, restated

> Anchors are the cryptographic spine of the network. Walking back
> through DN-validator-signed major-block boundaries to a binary-
> pinned DN genesis snapshot anchor establishes that we're synced
> to the real DN. Once that's established, every BPT leaf in the
> DN's state can be Merkle-proven against the trusted current
> StateTreeAnchor — without us having to recompute or even fetch
> the underlying account state. The BVN's trusted root falls out of
> trusted DN state (the BVN→DN anchors stored in `dn.acme/anchors`),
> so BVN bootstrap reduces to enumeration + gossip — no second
> spine walk.

Genesis snapshot anchor: the BPT root committing all DN state at DN
major-block 1. One per network. Pinned in the binary at
`internal/core/bootstrap/pinned`. Operators on dev networks can
override via a flag.

## Account state vs chain entries

Two different categories of data, treated differently at boot:

- **Account state** = main state (`KeyBook`, `KeyPage`,
  `DataAccount`, `ADI`, etc.) + secondary state (Directory list,
  Pending txid list, etc.) + every chain's *head* (count, current
  anchor, the `merkle.State`). This is everything the production
  observer at `internal/core/execute/v2/internal/bpt_prod.go` reads
  to compute the BPT leaf hash. **Compact**. Fetchable cheaply per
  account.
- **Chain entries** = the historical 32-byte hashes that built up
  to the current chain head, plus the transactions / signatures
  they reference. **Bulky**. Only collected for the spine; never
  for non-spine accounts at boot.

For a non-spine account, having state-without-entries means we can
recompute its BPT leaf hash, can append future entries to its
chains via `merkle.AddEntry` (which only needs the previous head),
but cannot answer historical queries like "what was txn X." That's
fine — the launcher's job is to participate in current consensus,
not serve historical queries.

## The cryptographic spine

The DN accounts the launcher walks back through, in full (state +
all chain entries):

- `dn.acme/anchors` — the anchor pool. Major-block index chain
  here gives the major-block boundary positions.
- `dn.acme/ledger` — block index info.
- `dn.acme/operators` — the operators keybook.
- `dn.acme/operators/1` — the operators key page (the validator set).

These are the only accounts whose **chain entries** are pulled at
boot. Everything else gets BPT-leaf-only treatment.

The spine walk:

1. Start from the latest major-block boundary the network reports.
2. Walk back major-block by major-block. At each boundary, pull
   the DA from the chosen BVN's anchor pool (where DN-validator
   signatures land), verify ≥2/3 quorum on `seq.Hash()` of the
   wrapping `SequencedMessage`, extract DN's `PartitionAnchor`.
3. While walking, replay operators-keybook deltas (`UpdateKeyPage`
   transactions) so the validator set used to verify older
   boundaries reflects the set that was current then. Today no
   rotations have happened on mainnet; the steady-state path is a
   no-op.
4. Terminate at major-block 1: its `PartitionAnchor.StateTreeAnchor`
   must equal the binary-pinned DN genesis snapshot anchor. Fail
   closed otherwise.
5. The latest major-block boundary the spine walk verified is
   then the **trusted current StateTreeAnchor**. Every long-tail
   BPT leaf can be Merkle-proven against this.

Where validator signatures actually land: each DA is signed by
DN validators with a `protocol.KeySignature` over the
`SequencedMessage` hash; the signed material is stored on the
*receiving* BVN's anchor pool (the DA's principal is
`<bvn>.acme/anchors`). DN itself doesn't keep DN-validator
signatures on its own outgoing DAs — the launcher reads them from
the BVN side. (Verified at `internal/core/crosschain/anchoring.go:212–221`
and `internal/core/execute/v2/block/msg_block_anchor.go:204–207`.)

## The two-track sync protocol

After the spine walk produces a trusted current StateTreeAnchor,
two activities run **concurrently** until DN active:

**Track 1 — BPT enumeration.** Paginated `bptproof.GetPage` against
the network (any peer that can serve), pulling (key, value-hash)
pairs with Merkle membership proofs. Each verified pair populates
a leaf in the local DN BPT.

Pulling from the network as a whole rather than from a fixed
peer-snapshot lets the BPT advance under us during enumeration —
new accounts created mid-enumeration appear in later pages because
we're querying live state, not a frozen view. There's no
single-instant consistency requirement; each pair is consistent
with *some* recent root, and gossip will deliver any updates that
moved the root since.

**Track 2 — Gossip ingestion.** Subscribe to DN block events via
`EventService.Subscribe` (or equivalent). Apply incoming messages
to local state:

- Validate against the latest trusted operators-keybook state.
- For each touched account, fetch its current state (head + chain
  heads + secondary) on demand if we don't already have it. The
  fetched head's hash must match the BPT-leaf value-hash we
  trust; mismatch means the network and our trusted root have
  drifted in a way we can't reconcile (peer lying, fork, bug).
- Append new chain entries forward via `merkle.AddEntry`. The
  predecessor entries don't need to exist locally; only the
  previous chain head (which we have).

DN ACTIVE = local DN BPT root tracks the network's signed
major-block anchors. Steady-state, not a single match. Once we're
producing matching roots, the DN side is done.

## BVN sync (no spine walk)

Once DN is active, the BVN's trusted root is read out of DN's
trusted state — the BVN→DN anchor sitting in `dn.acme/anchors`'s
main chain. That anchor's `PartitionAnchor.StateTreeAnchor` is the
BVN's BPT root, trusted transitively through DN.

BVN bootstrap is then exactly Track 1 + Track 2 against the BVN —
enumeration + gossip — without a separate spine walk. BVN ACTIVE
= local BVN BPT root tracks the BVN's StateTreeAnchor as recorded
in the trusted DN state (and updated on each DN major-block).

## Persisted artifact

```go
bootpersist.Artifact {
  Network                  string
  BVN                      string
  DNGenesisStateTreeAnchor [32]byte  // mirrors binary pin
  DNVerifiedAnchor         [32]byte  // latest StateTreeAnchor from spine walk
  DNVerifiedMajorBlock     uint64
  BVNVerifiedAnchor        [32]byte  // latest BVN StateTreeAnchor from trusted DN
  BVNVerifiedMajorBlock    uint64
  State                    StateRecord  // BOOTING / ACTIVE / COMPLETE
  Cursors                  Cursors      // reserved; not actively updated yet
}
```

`accumulated run`'s `detectBootstrapState` reads this on startup
and restores the nodestate machine in ACTIVE iff both anchors are
populated. Pin-mismatch (binary's `DNGenesisStateTreeAnchor`
disagrees with the artifact's) is fail-closed.

## What the launcher cannot do

- **Respond to peer requests over P2P during boot.** It doesn't
  have the state. If the launcher must answer API/P2P calls before
  ACTIVE, two implementation paths are open:
  - Forward calls to a COMPLETE or legacy peer (transparent proxy)
    and pass the response back.
  - Run with a transient libp2p identity that's not advertised
    until ACTIVE flips and the node adopts its real key.
- **Validate transactions whose preconditions touch accounts whose
  state we haven't pulled yet.** Gossip processing has to fetch
  state on demand or defer the message until state is available.
- **Serve historical queries.** Chain entries for non-spine
  accounts aren't kept locally. A `query "what was txid X in 2024"`
  would need to be re-fetched from a COMPLETE peer.

## Implementation map

This branch (`bootstrap-v3`) starts fresh, branched off `main`
without the v2-era pipeline / pull / headerwalk / completeness /
convergence / cmd_bootstrap baggage. Three packages came forward
from `bootstrap-v2` because they were shape-correct under the
sync model:

- `internal/core/bootstrap/bootpersist` — artifact, save/load,
  format-major guard.
- `internal/core/bootstrap/nodestate` — state machine, in-process
  advertisement type, restore.
- `internal/core/bootstrap/pinned` — DN genesis snapshot anchor
  per network.

Everything else is unbuilt yet. The build order, sync-first:

1. **BPT enumeration primitive** — server side: a v3 query that
   returns paginated `(key_hash, value_hash)` batches against the
   peer's BPT. Client side: a consumer that pulls pages and
   inserts each into a local BPT.
2. **Spine pull** — DN-side anchor pool / ledger / operators /
   operators/1 pulled in full (state + chain entries). Replaces
   the v2 puller with a primitive that knows how to set chain
   heads directly without replaying entries for non-spine
   accounts.
3. **Gossip subscriber** — wraps `EventService.Subscribe`,
   accepts incoming block events, fetches account state on
   demand for accounts in the events that we don't have, updates
   local BPT leaves.
4. **Local BPT-root tracker** — compares the locally produced
   BPT root to incoming signed StateTreeAnchors; flips ACTIVE
   when the local root catches up.
5. **Orchestrator** — DN: spine pull → start enumeration +
   gossip concurrently → ACTIVE when tracking. BVN: enumeration
   + gossip → ACTIVE when tracking.
6. **CLI + persisted-artifact handoff** — `accumulated bootstrap`
   wires the orchestrator; on success writes the new-shape
   artifact for `accumulated run` to detect.

Cryptographic-proof verification (validator-quorum on DAs,
genesis-pin terminator, operators-rotation tracking via
keybookat) is a phase-2 concern that comes after sync works.

## Out of scope

- Pin table population — release-process item, not code.
- Live mainnet smoke — depends on the populated pin (or operator
  override).
- Cryptographic proof verification at boot. Trust at boot comes
  from the chosen ACTIVE/COMPLETE peer; sync-time signature
  verification + genesis-pin termination are added in a later
  phase once the sync mechanism is solid.
- Validator-rotation hot path. Same deferral as above.
- Historical-query backfill (we don't keep non-spine chain
  entries; serving "what was txid X" is a follow-up).
