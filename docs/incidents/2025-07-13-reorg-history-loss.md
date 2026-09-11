# The 13 July 2025 reorg — what it dropped, and what can be recovered

**Status**: investigation complete; recovery partly implemented, partly a decision
**Date of incident**: 2025-07-13
**Date of this document**: 2026-09-11
**Affected**: mainnet, all partitions
**Issues**: #4268, #4269, #4270; related #4020 (a separate, later corruption)

---

## Summary

The reorg consolidated four partitions into two by restoring a filtered
snapshot into an empty database. The filter — `internal/node/genesis/extract.go`,
run by `accumulated prepare-genesis` — dropped more than intended, and the loss
was invisible until an integrator reported that `query-data` and
`query-tx-history` had stopped working (#4268, #4269).

Three things were dropped:

1. **Mark points** (`States(i)`) for every account that is not a
   `DataAccount` or `LiteDataAccount`.
2. **The message bodies those mark points referenced** — a second pass restores
   only bodies whose hashes appear in a *kept* HashList, so dropping the mark
   points dropped the bodies with them.
3. **Every system and partition account**, wholesale — including the
   Directory's anchor pool, and with it the anchor chains for the three
   retired BVNs.

These are one loss, not three. A merkle chain keeps its entry hashes in the
mark points and in the head; there are no separate `Element(i)` records in the
restored database. Discarding the mark points therefore discarded the hashes of
every closed mark set, and nothing then referenced the bodies.

## Measured damage

**Chain history.** For `acc://bridge.acme/1-ACME` (441 entries):

| record | present after the reorg |
|---|---|
| `MainChain.Head` | yes — `Count=441`, `HashList=185`, `Pending=9` |
| bodies for those 185 hashes | 185 of 185 |
| `MainChain.States(*)` | none |
| `MainChain.Element(i)` | none |

What survives is exactly the **open mark set** — entries 256–440. Entries
0–255 have no hashes, no bodies and no mark point. Every read of the account's
history fails, including of the 185 that survive, because `Chain.StateAt`
replays from the mark point at 255 and it is gone. On `main` that is a hard
error (`pkg/database/merkle/chain.go`); `dagbft-integration` carries
truncated-chain handling that `main` does not.

Accounts with fewer entries than one mark frequency lost nothing: their whole
history is still in the head. On the pre-reorg bvn2, of **2,163,468** accounts,
58,464 have a main chain and only **197** have ≥256 entries — so the damaged
set is small, though it includes the busiest accounts.

**Anchors.** The Directory's anchor pool lost the retired partitions entirely:

| partition | pre-reorg DN | current DN |
|---|---|---|
| Apollo | 229,051 | 0 |
| Yutu | 282,504 | 0 |
| Chandrayaan | 314,387 | 0 |
| Directory | 761,404 | 13,064 |
| Cyclops | — | 11,892 |

The DN's own anchor history was truncated in the same operation.

## What can be recovered

The pre-reorg databases survive, extracted, on thelio at
`/mnt/secondary/databases7-13-25-restored/{bvn0,bvn1,bvn2,dn}` (775 GB, Badger
v1), with copies on the Expansion drive. They are complete: for
`bridge.acme/1-ACME`, bvn2 holds the mark point at 255, all 256 bodies, every
`Element`, and for data accounts the `Data.Entry` index that #4268 is about.

**The chain continued through the reorg rather than restarting.** Pre-reorg
count 439, current 441 — the head was carried with its count intact, so the
missing records carry identical keys. Recovery is a key-for-key copy between
two different storage backends, not a migration.

Demonstrated end to end: copying 1,094 records for two accounts into a copy of
a post-reorg database restored the mark point at 255, and the restored mark
point replays through the head's own 185 hashes to the stored head anchor.

### Two mechanisms, for two kinds of record

- **History** — mark points, bodies, the data-entry index — does not feed
  `hashChains`, which folds each chain's head anchor. It is read-side, so it
  needs no coordination: a node-local sidecar of the recovered records, read
  through when a lookup misses, repairs the live database as it is used.
  Nodes may differ without consequence.
- **Anchors** are consensus state. Restoring them is not a node-local matter
  and belongs with the repair-by-transaction mechanism discussed on #4020.

### Verification, not trust

A restored mark point is checkable rather than trusted: replaying the head's
own HashList onto it must reproduce the stored head anchor. Any account whose
replay does not match is flagged and left alone.

Where the archive is unavailable, the boundary mark point is often
reconstructible from the head's peaks alone — a peak at level *k* is the root
of a complete subtree of 2^k leaves, and a state's peaks are exactly the set
bits of its count (`pkg/database/merkle/reconstruct.go`). That covers 99.8% of
chains with a closed mark set; the exceptions are counts that are multiples of
512, where the boundary's subtree has been absorbed into a larger peak and
cannot be extracted. The reconstruction and the archive agree where both are
available: for `bridge.acme/1-ACME` both give anchor `6530e661395b`.

## Open decisions

- **Whether to restore the anchor chains.** The data exists. Writing it is
  consensus state, so it needs an authority and a mechanism (#4020). Until then
  pre-reorg BVN state can be made readable but not provable.
- **`acc://dn.acme/network`** (#4020) has no body in any archive; zeroing its
  leaf or clearing the entry outright are the only options.
- **The filter itself.** Dropping mark points does not trim history, it makes
  the chain unreadable below the first surviving mark point and takes the
  bodies with it. Whatever is decided about mainnet, `extract.go` should not do
  this to the next network that is rebuilt.

## Method note

Do not probe these databases with hand-built `record.Key` paths. During this
investigation, hand-built keys reported the DN's anchor chains as absent when
they were present, and the error was only caught because a control against a
chain known to exist failed the same way. Use the model's own accessors —
`Account.AnchorChain(part).Root()`, `Account.Chains()` — so the key derivation
is the code's. `tools/cmd/modelprobe` does this.
