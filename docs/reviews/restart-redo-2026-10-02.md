# Review: the whole-restart redo, run 20261002T145702Z

Written 2026-10-02 after the run was stopped (Paul: "stop the run and write
it up, then do a deep review of the data and the code"). Data: the run
directory `test/docker/soak/runs/20261002T145702Z` (manifest, `SUMMARY.md`,
`monitor.csv`, `mem.csv`, `stats.csv`, `submissions.csv`, the hourly profile
snapshots, the live log before it was deleted, and the two torn stores in
`torn-stores/`). Code: every commit on `issue-restart-authored-batches`
above the lead (a6893d6ed).

## 1. What the run did, in one table

| when (Z) | event | state of the network |
|---|---|---|
| 14:57 → 16:09 | load at 1 tx/s; three late-follower cycles | all four partitions at 1 block/s; 18/18 joins ACTIVE in 46–66 s and synced |
| 16:09:05 | `rm -f` of all 16 containers, recreated at once | all 16 resumed within 10 s; no consensus stall, no dropped proof (#4502/#4503/#4505 held) |
| 16:09:16 → | bvn2-val3 and bvn2-val4 fail every block on torn stores | BVN2 produces on two executing validators; its anchors carry 2 signatures of 3 |
| 16:10 → 17:18 | the Directory holds BVN2's anchors (123 → 1,385 → 1,987); inbound flows from BVN2 red everywhere; heaps climb on every node | Directory still ~1 block/s; load generator rejections climb (two validators refuse submissions as lagging) |
| 17:18 | recovery attempt: val3/val4 recreated with cleared BVN2 stores and checkpoints | they cannot rejoin (2 signers < threshold 3) and no longer vote: **BVN2 halts at 8215** |
| 17:33 / 17:35 / 17:35 | bvn3-val2, bvn2-val1, bvn2-val2 exit 137 at the 2 GiB container limit | nine Directory validators left |
| 17:36:55 | **the Directory freezes at 9079** and never moves again | BVN1 and BVN3 run on alone for 3 h (to 20664 / 17430), anchoring nothing |
| 20:51:57 | stopped | |

## 2. Data review

### 2.1 Follower cycles: the joins work; every BVN join pays one repair

18 of 18 partition joins reached ACTIVE in 46–66 s and synced 3–5 min after
the add (20 perfect blocks, then 100 with no regression). Every Directory
join closed its overlap with `differences=0`. **Every BVN join** logged
exactly one `Execution differs from the block ledger … differences=8` on the
block right after its first ACTIVE, went back to the ledger, repaired
(`Repairing the state … own=2..5`), and was ACTIVE again within seconds. The
cause is in the lines around it:

```
A section a block reads was taken as a peer holds it after this node's block; the root check decides  block=1986 peer-block=1987 section=400
A block changed a chain other than its record; the record stands  block=1986 chain=acc://lg-….acme/tokens#main  why="the record names a change this execution did not make"
```

The sectioned BPT sync (#4485) loads a section lazily, from a peer, **as of
the peer's current block**, one ahead of the block being executed; the
accounts in it are a block ahead, the execution of that block then differs
from the record, and the root check repairs it. This is the designed
fallback working — Paul's loop — but it fires on every BVN join at the same
place, which means the first block after a handoff is never clean. It
costs seconds, not correctness; it is a lazy-load design point, not a bug.
Filed as a question (see §5).

### 2.2 The restart: the three fixes held

No partition held at a round (every BVN3-class round certified with four
certificates); no stream held behind a dropped proof (`disproved` 0 on every
node read; the only validated-via-history proof seen was on bvn1-val1). All
16 nodes took the whole-restart exit and were "executing in agreement"
within 10 s.

### 2.3 Two torn stores, read offline

| store | mutable record stale | permanent record lost | fatal read |
|---|---|---|---|
| bvn2-val3 | `ledger.RootChain Tail(21)` (index 32424) holds 4 of 8 | `anchors.MainChain.Element.3937` (present on bvn2-val1) | `RootChain Element(32256)` shallow: missing; deep: present |
| bvn2-val4 | `ledger.BlockLedgerChain Tail(5)` (index 4136) holds 7 of 8 | none found | `BlockLedgerChain Element(4096)` shallow: missing; deep: present |

The kill lost the last rewrite of one mutable Tail chunk on each store (and
one permanent element on val3) while the head and later chunks persisted
(BlockchainDB#99). `merkle.Chain.tailFallback` then refilled the chunk from
elements through the block's shallow batch; the elements were older than
the window and read as missing; the block failed, every time (348 / 151
failed groups by 16:15). Fixed at `082935148` (#4507): the adapter reads
history for positional permanent records.

### 2.4 How a partition at 2 of 4 killed the network

1. BVN2's anchors carried val1's and val2's signatures only: below the
   Directory's threshold of 3, so the Directory **held** them — 123 at
   16:12, 1,385 at 16:40, 1,987 at 16:50 — and every proof BVN2 built could
   not anchor. Inbound flows from BVN2 went red on every partition and
   stayed red (the dashboard's `deliveryStall` 3,200 s by 17:0x).
2. Heaps climbed on **every** node from 16:30: 0.5 GiB → 1.0–1.5 GiB by
   17:00, 1.4–1.75 GiB by 17:30 (`mem.csv`). The 17:00 heap profile of
   bvn1-val1 shows only **319 MB live**, dominated by
   `types.UnmarshalHeader` (66 MB) and `UnmarshalCertificate` (85 MB
   cumulative) and libp2p message buffers — consensus traffic, not state;
   `heapInuse` 956 MB at the same moment is GC headroom under `GOMEMLIMIT`
   1.7 GiB. RSS followed: bvn2-val1 1.33 → 1.87 GiB between 17:31 and
   17:34 with `nextGC` chasing it.
3. bvn3-val2 (17:33:34), bvn2-val1 and bvn2-val2 (17:35:50) exited 137 at
   the 2 GiB container limit. Peaks on the survivors: 1.77–1.90 GiB.
4. With nine Directory validators alive the Directory needs 8 votes; its
   headers got **7** (`ownVotes=7` on every surviving author). The two
   recreated BVN2 validators' Directory nodes were alive and restored
   (`Restored consensus position … certificates=721`) but did not vote on
   current rounds — not explained by the logs read so far (§5).
5. The Directory froze at 9079 at 17:36:55; BVN1 and BVN3 kept producing
   blocks that anchored nowhere for three hours. The load generator ended
   with 3,107 of 17,971 submissions rejected; the validators' Directory
   partitions show ~4,900 "accepted, neither certified nor refused" each.

The chain is: a partition below its signing quorum → the Directory holds
its anchors without bound → cross-partition delivery stops → consensus
traffic swells (re-sharing, re-proposing) → GC headroom plus RSS overhead
crosses the container limit → validators die → the Directory loses its
quorum. Each link is a finding (§5).

### 2.5 The recovery attempt (mine) made it worse

Recreating val3/val4 with cleared stores assumed they could rejoin. They
cannot: with two signers, no BVN2 anchor reaches the threshold, so no state
can be proven to a joiner — and the join then took the **whole-restart exit**
(`Fewer of this partition's validators serve than sign an anchor; resuming
from this node's own block block=1`) on a node with no own block, and
looped on `cannot resume from its own block: no consensus checkpoint matched
block 1`. Clearing their consensus checkpoints also removed their votes:
BVN2 fell to two voters and halted. Two lessons: the exit branch must not be
taken by a node with nothing to resume from (§5), and a partition that loses
a quorum of stores needs #4501, not a rejoin.

## 3. Code review of the branch

| commit | change | finding |
|---|---|---|
| f7350a956 | stall line carries round state; disproved proof said once | `RoundStatus` takes `roundMu` then `pendingMu` sequentially, never nested — no lock-order risk. The disproved map is bounded at 4096. OK. |
| 3dadf560a #4502 | `writeTail` names the last authored header's batches | `dagStore.Authored()` is one small file read per checkpoint; `writeFile` is atomic, so a concurrent `persistAuthored` cannot be half-read. A certified header's batches are named twice (by its cert and here) — harmless. **Test fails without it.** OK. |
| c452a9c44 #4503 | the vote gate asks for every missing batch | `requestMissingBatch` deduplicates per digest with a 5 s retry window and a bounded map, so 600 missing batches are 600 fetches per 5 s, each a goroutine walking peers with 2 s timeouts. Bounded, but a header re-proposing hundreds of batches makes a burst of hundreds of concurrent fetches. **Improvement:** fetch several digests per request, or cap concurrency. Not a correctness issue. |
| ae94c647b #4505 | admissibility retries through a deep view for an executed anchor | Still needed after #4507: `ElementIndex` is deliberately not positional. The deep read happens only when `SourceBlock <= executed`, so a fresh proof keeps its cheap miss. OK. |
| dafbf946a #4506 | `join.Watch` for a non-validator from genesis | **Defect found and fixed (3cc65e9b2):** with the globals not yet loaded, `CanPropose()` is false and a validator would have been watched — and demoted, and stopped producing, on its first mismatch. Gated on `Known()`, predicate under test. |
| 082935148 #4507 | the adapter reads history for `Element`/`Intermediate` | `isPositional` matches every chain's `Element(i)` (main, root, BPT-index, block-ledger chains) and `Intermediate(i, h)`; `ElementIndex` and `States` excluded. The miss is still counted. Two tests fail without it. The whole adapter suite is green (536 s). OK. |

Spec: `consensus.md` (restart tail, deferred-vote rule), `database.md`
(windowed stores, two additions), `executor.md` (a node that starts with its
network) were changed in the same commits. `DIFFERENCES.md` gains the items
in §4.

Tests not run on this branch: the full gate. Run before merging to the lead.

## 4. What the spec says and the code does not yet do

- A node that resumes onto an inconsistent store must detect it and join
  (#4507, node side). Today it resumes and fails every block.
- A partition that restarts as a whole resumes from the node with the
  highest signed, validated state (#4501). Not built; and it is also the only
  repair for a partition that lost a quorum of stores.
- A node with nothing to resume from must not take the whole-restart exit
  (new, below).

## 5. Issues filed from this review

- **#4508 Join**: a joiner with no own block takes the whole-restart exit when
  fewer validators serve than sign, and loops on "no checkpoint matched
  block 1"; it should keep collecting and say so.
- **#4509 Sections**: a section loaded lazily is taken as of the peer's block, one
  ahead of the block being executed, so the first block after every BVN
  handoff differs and is repaired. Question: take it as of the node's block,
  or accept the repair as the design.
- **#4510 Memory**: with `GOMEMLIMIT` 1.7 GiB in a 2 GiB container, three
  validators were killed at the limit under a consensus-traffic storm while
  live data was ~300 MB; the margin is too thin for a stall's allocation
  rate, and nothing bounds what the Directory holds for a partition below
  its quorum.
- **#4511 Directory quorum**: with nine validators alive, headers collected seven
  votes; the two recreated validators' Directory nodes restored their
  position but did not vote. Needs a debugger pass.

Already filed earlier today from the same run: #4506, #4507, BlockchainDB#99
(plus #4502–#4505 from the previous run).
