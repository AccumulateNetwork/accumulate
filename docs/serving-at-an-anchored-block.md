# Serving an account and a BPT page as of an anchored block

_What a peer answers when a query names a past minor block, what that answer
commits to, and what it costs to be able to give it. Issue #4361; the AIP-58
work on `main` (`docs/protocol/historical-account-state.md`) ported to this
line and changed where this line differs._

A query for an account may set `ForHeight` to ask what the account held at a
past minor block, and a `BptPageQuery` may do the same for a page of the tree.

## Why this line needs it

`executor.md`, "Sync", §2:

> Serving an account **as of an anchored block**, rather than as of the peer's
> own current block, is a capability this line does not have and the join
> cannot work without.

A peer that can only answer as of its own current block hands a joining node a
receipt to a root the Directory has not anchored. The node must then wait for
the anchor — and for an account that changes every block the wait never ends,
because by the time an anchor for that block arrives the account has moved on
and the node is holding a leaf no anchor covers. Cold accounts settle, hot ones
never do, and a restart, which needs only the hot ones, converges on nothing.

## What the answer is

Both answers are **as of the last state-changing block at or before the
requested height**, and `Receipt.ForHeight` reports that resolved block.

Resolving backward is exact, not an approximation. A partition indexes only the
blocks that changed state, and a block that changed nothing carries its
predecessor's BPT root — so the last state-changing block at or before H holds
precisely the state as of H. Resolving *forward* would return state containing
changes that had not happened at H, which for a caller checking a signature
against the key page version it was made under is a confident, checkable, wrong
answer.

### The account

```
main state @ B  ──►  account BPT entry @ B     the retained state receipt
BPT entry @ B   ──►  BPT root @ B              BPT membership, retained nodes
```

The response carries **the body as of B**, and the receipt starts at a plain
SHA-256 of that body, so a caller recomputes the starting point from what it
was handed instead of taking the server's word for it. `StartsAtMainState`
reports this and is set on every historical answer.

`Directory`, `Pending` and `Sighted` are **not** carried on a historical
answer. They are not retained per block, and a present-tense value beside a
past body is the same mistake this exists to stop.

### The BPT page

`BptPageQuery.ForHeight` returns the page the tree held at B, and `BptRoot` is
the root as of B. The walk is the current one with node loads redirected to
retained versions (`bpt.BPT.GetRangeAt`), so there is one page implementation,
not two.

**A page carries no proof, so the server checks the tree twice on the reader's
behalf.** Every block loaded below the root must hash to what the block above
it records for it — a boundary node is written once inside its parent's block,
which keeps its hash, and once as a block of its own — and the root recomputed
from those blocks must equal the root the ledger recorded. The root check alone
is not enough: the root is recomputed from the hashes its own block records, so
a stale block further down whose parent's version at that height is intact
passes it. A receipt refuses such a tree anyway, being rebuilt from the leaf
upward; a page is a read of leaves and has no such arithmetic in it.

## Where the receipt terminates, and why it differs from `main`

**It ends at the BPT root as of B.** On `main` it does not: the receipt
continues through the ledger's `bpt` chain to the partition's *current* root,
on the grounds that "an arbitrary block's root is not something a client can
check".

That reason does not hold here, and the opposite is what the join needs.

- **It is checkable.** The root as of B is exactly the `StateTreeAnchor` the
  partition's anchor for block B carries: the anchor is built in the following
  block's `BeginBlock` from the BPT root as it stood at the end of B
  (`internal/core/crosschain/anchoring.go`). A joining node validates that
  anchor against the operators' key book before it pulls anything
  (`executor.md`, "Sync", §1), so the terminus is a value it already holds.
- **It is bindable anyway.** This line anchors the ledger's `bpt` chain into
  the root chain (#4272, Kourou-gated,
  `internal/core/execute/v2/block/block_end.go`), so *any* root on that chain —
  a past one included — extends to a root-chain anchor and binds to a directory
  root through `ProofService.AnchorReceipt` (#4274/#4276). A caller wanting
  `main`'s reach makes that second call with this receipt's `Anchor`.
- **Terminating at the current root is the defect**, which is #4361 itself.

`Receipt.Complete` is therefore false on a historical answer from every
partition, the Directory included: a past directory root is not the current
directory root that `Complete` promises.

## A coherent answer or a refusal

`main` degrades when it cannot start the proof at the main state: it returns a
proof rooted at the whole BPT entry and reports `StartsAtMainState` false. That
is correct on `main`, which serves the current body regardless.

Here the response carries the body the receipt proves, so there is no half
answer. Where the node holds the account's BPT entry for a block but not the
state behind it, the request is refused. The alternative would be a past
receipt beside a present body: a caller that checks refuses it, and a caller
that does not check keeps state no anchor covers.

## Retention

| where | setting | default |
|---|---|---|
| DAG-BFT service config | `BPTHistoryDepth` | 1024 |
| simulator | `simulator.BPTHistoryDepth(n)` | 0 |

`main`'s default is zero — historical proofs are an operator feature there.
Here the default is **1024 minor blocks**, because the join cannot work without
it and this line ships fresh installs rather than gates.

The number comes from what the join has to do inside the window, not from a
storage budget. A joining node fixes on the newest block it holds a verified
anchor for and must still be able to ask for *that* block when the round
finishes, so the window has to cover the anchor lag plus one pull round:

- the anchor lag is bounded on this line at 64 root-chain positions
  (`anchorSearchWindow`, `internal/api/v3/proof.go`) — a root the Directory has
  not covered within that is already treated as not coming;
- a round is the block-ledger walk over `(R, Q]` and the accounts it names;
- blocks are about a second under load, so 1024 blocks is roughly seventeen
  minutes.

What it costs, per node, stated so a run has a prediction to check:

- one BPT block-write per state-changing block. Measured on `main` at 34 KB for
  a small tree and 71 KB at a million accounts, of which the 34 KB top block is
  rewritten every time and does not grow.
- `dirty accounts per block × depth × 146` bytes of retained state receipts.
- `dirty accounts per block × depth × |marshalled body|` bytes of retained main
  state — the addition this line makes over AIP-58, and the larger of the three
  for an account with a big body.

Measuring it against BlockchainDB is #4165's soak.

Retention writes only under `("BPT", "History", …)` and the account's
`RetainedStateReceipt`/`RetainedMainState` records. Nothing under the existing
key shapes changes, the BPT root is unchanged, and no migration runs — so
enabling it is not a consensus change and needs no executor-version gate.

### The retained range is predictive

`RetainedBlockRange` reports the blocks the node can answer for: every height
inside it is answerable and every height below it is refused.

- The earliest end is read from what the node **actually retained**, never from
  its configured depth — raising the depth does not retroactively create
  history — and is rounded **up** to the first indexed block at or after it,
  because a height between the horizon and the next state-changing block would
  resolve below the horizon and be refused.
- The latest end is the newest block whose root is on the ledger's `bpt` chain,
  which runs one entry shorter than the root index chain: it records the
  *previous* block's state hash, so the newest indexed block's root lands only
  when the next state-changing block commits.

A range that over-promised would be worse than none at all, because a client
would plan around it.

## Refusals

A client branches on the status code without parsing prose.

| status | meaning |
|---|---|
| `IncompleteChain` (414) | a capability limit: the height precedes this node's earliest indexed block, or is indexed but no BPT history is retained for it, or the node holds the BPT entry but not the state behind it, or it cannot rebuild the account's leaf for that block. The message names the boundary |
| `NotReady` (504) | the height is beyond this node's latest indexed block — "not yet", not "never"; retry later |
| `NotFound` | **this node's index** has no record of the account at that height. Not proof of absence: a requester counts it as a miss and asks somebody else |
| `BadRequest` | `ForHeight` was zero, which means the current state and is not a historical request |

Issue #4361's done-when says a block outside the window answers "`NotFound`
that says so". It answers `IncompleteChain`, and the distinction is what keeps
the join honest: on this line a requester reads `NotFound` as a fact about the
RECORD — `join/sources.go` makes a `NotFound` that every peer gives the
network's answer — and everything else as a fact about the PEER, which sends it
to the next one.

**So nothing on this path turns a local failure into `NotFound`.** The height is
judged before the account is, so a height of zero or one outside this node's
indexed range never becomes an answer about the account. A node that cannot
read the beginning of its own main index chain — which is every node that
joined, for any account whose index chain is longer than one 256-entry mark
block — says "I cannot tell" and answers from the BPT instead of reporting the
account missing. And a leaf this node could not rebuild for the block is
`IncompleteChain`, because the reconstruction is checked against the recorded
root only after the leaf is found, so a leaf that failed to rebuild looks
exactly like one that was never there.

**A node that cannot prove the past says so.** There is no fallback to the
current root or the current body anywhere on this path.

## What the proof does not commit to

**It does not reach past this node's horizon.** A node restored from a snapshot
has no record of anything before its restore point, and the protocol has no
incarnation concept that would say whether an earlier block belonged to the
same network at all. A height below the horizon is refused, never resolved
forward.

**It proves what the records held, not who was entitled to write them.** A
membership proof says an account's entry had a particular hash at a particular
block. It says nothing about whether the transaction that produced that state
was authorised.

**It is a body, not an account.** The retained state receipt collapses the
account's secondary state, chain anchors and pending list into sibling hashes,
so the proof of the body is complete — but the *components* at that block are
not retained and are not served. A node that means to write the pulled account
into its own state tree and reproduce the leaf locally needs those too, from
the chain and pending queries, at the same block. That is the consuming side's
problem (#4362).
