# Historical account state proofs (AIP-58)

A query for an account may set `ForHeight` to ask what the account held at a past
minor block, rather than what it holds now. This document says what such a proof
commits to, what it does not, and what an operator must do for a node to be able
to serve one at all.

## What the proof is

The response's receipt runs from the account's BPT entry as of the resolved block
to the partition's **current** BPT root, passing through the historical root on
the way. It validates offline.

```
account BPT entry @ H  ──►  historical BPT root @ H      retained BPT nodes
historical root        ──►  ledger `bpt` chain anchor    chain receipt
`bpt` chain anchor     ──►  ledger account's BPT entry   the account hash commits
                                                          to every chain's anchor
ledger BPT entry       ──►  current BPT root             BPT membership
```

It terminates at the current root rather than the historical one because an
arbitrary block's root is not something a client can check. Only the roots that
were carried into a cross-partition anchor appear on an `anchor(<partition>)-bpt`
chain and are covered by a quorum signature, and those are a sparse subset — on
MainNet, roughly 13,700 of the 46,000 blocks the directory has recorded. The
current root is the one the network is about to anchor, so it is the one worth
reaching.

**Completing the binding is the client's step.** The receipt ends at this
partition's BPT root. To tie that to a quorum signature, look it up on the
directory's `anchor(<partition>)-bpt` chain. A root that has not yet been
anchored cannot be bound yet; the proof is re-derivable later, against a root
that has been.

## Resolution is backward, and exact

`ForHeight = H` resolves to the last **state-changing** block at or before H, and
`Receipt.ForHeight` reports that resolved block.

Resolving backward is not an approximation. A partition indexes only the blocks
that changed state, and a block that changed nothing carries its predecessor's
BPT root — so the last state-changing block at or before H holds precisely the
state as of H. Resolving *forward* would return state containing changes that had
not happened at H, which for a caller checking a signature against the key page
version it was made under is a confident, checkable, wrong answer.

## What the receipt starts at

It starts at the account's **whole BPT entry** — the merkle hash over its main
state, its secondary state, the anchor of every one of its chains, and its
pending transactions.

This differs from a current-state receipt, which starts at the account's **main
state** hash. The historical path cannot do the same, because rebuilding the
account hasher at a past block would need the account's main state at that block,
and only BPT nodes are retained, not account state. **A caller that assumes both
starts mean the same thing will compare the wrong value.**

## Retention: what an operator must enable

Nothing is retained by default. A node writes no historical records and refuses
every `ForHeight` query until an operator sets a depth:

| config | setting |
|---|---|
| `accumulate.toml` | `bpt-history-depth` on the `[accumulate]` section |
| run config | `BPTHistoryDepth` on the consensus app |

The depth is a number of minor blocks. Cost is roughly one BPT block-write per
state-changing block — measured at 34 KB for a small tree and 71 KB at a million
accounts, of which 34 KB is the top block, which is rewritten every time and does
not grow. On MainNet's BVN that is about 0.6 GB per year; on a busier network it
is proportionally more.

Retention writes only under a new key shape, `("BPT", "History", …)`. Existing
databases stay readable, no migration runs, and the BPT root is unchanged — so
enabling it is not a consensus change and needs no executor-version gate.

## The retained range, and why it is predictive

`ConsensusStatus.RetainedBlocks` reports the range of blocks the node can answer
for. It is **predictive**: every height inside it is answerable, and every height
below it is refused.

Both ends are computed from what will actually be served, which is not the same
as what was retained:

- The earliest end is read from what the node **actually retained**, never from
  its configured depth — raising the depth does not retroactively create
  history — and is then rounded **up** to the first indexed block at or after it,
  because a height between the horizon and the next state-changing block would
  resolve below the horizon and be refused.
- The latest end is the newest block whose root has been recorded. The ledger's
  `bpt` chain runs exactly one entry behind the root index chain, because it
  records the *previous* block's state hash, so the newest indexed block's root
  lands only when the next state-changing block commits.

The field is absent when the node retains nothing. A client cannot distinguish
that from a node too old to report it, which does not matter: the safe action is
the same.

A range that over-promised would be worse than none at all, because a client
would plan around it.

## Refusals

A client can branch on the status code without parsing prose.

| status | meaning |
|---|---|
| `IncompleteChain` (414, JSON-RPC `-33414`) | the height is indexed but no BPT history is retained for it; the message names the retained range |
| `NotFound` | the height is below this node's earliest indexed block, or above its latest, or the account had no record at that height — the three read differently |
| `BadRequest` | `ForHeight` was zero, which means the current state and is not a historical request |

**A node that cannot prove the past says so.** There is no fallback to the
current root anywhere on this path. Answering a question about the past with a
present-tense answer would be worse than an error, because it would be answered
confidently and wrongly.

## What the proof does not commit to

**It does not reach past this node's horizon.** A node restored from a snapshot
has no record of anything before its restore point, and the protocol has no
incarnation concept that would say whether an earlier block belonged to the same
network at all. A height below the horizon is refused, never resolved forward.

**It cannot cross a network restart.** The proof reaches only to the genesis of
the current incarnation. **Every network restart is a trust discontinuity**, and
no receipt on either side of one says anything about the other.

**It reaches back only to Baikonur.** The ledger's `bpt` chain, which records the
per-block roots this proof resolves against, is written only when the executor
version is at least `V2Baikonur`. Blocks before that activation have no recorded
root and cannot be proven against.

**It proves what the records held, not who was entitled to write them.** A
membership proof says an account's entry had a particular hash at a particular
block. It says nothing about whether the transaction that produced that state was
authorised.

## Anchor signatures, and a snapshot that breaks this

Completing the binding needs the quorum signatures on the anchors that carry a
partition's roots to the directory. **No pruning of those signatures exists
today, so their retention is incidental rather than guaranteed** — nothing in the
protocol commits to keeping them, and nothing currently deletes them.

One thing does delete them. `tools/cmd/debug snap collect --skip-signatures`
(`tools/cmd/debug/snap_collect.go:163`) produces a snapshot that omits message
references, and **a node restored from such a snapshot cannot serve these
proofs** — it has the state but not the signatures that make the state
checkable. An operator intending to serve historical proofs must not restore
from a `--skip-signatures` snapshot.
