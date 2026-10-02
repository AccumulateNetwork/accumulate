# Run 20261002T145702Z — summary

Written 2026-10-02 when the run was stopped (Paul: "stop the run and write it
up"). The bulk log it summarises was deleted afterwards (see "What was kept").

## What it was for

The redo of run 20261002T090546Z on the branch that fixes what that run
broke: a fresh 12-validator network on `issue-restart-authored-batches`
(lead a6893d6ed + #4502 + #4503 + #4505), steady 1 tx/s, three late-follower
cycles, then the same whole-network restart (`docker rm -f` of all 16
containers, recreated at once).

## Provenance

| | |
|---|---|
| validators and followers | `d04f77ab7` (= `ae94c647b` + the previous run's evidence commit); image `disoak-bvn1-val1` `ba7cdbba0c06` built 14:58Z |
| comparison follower | `acc-bvn3-fol1`, started with the network |
| late followers | bvn1-fol1, bvn2-fol1, bvn3-fol2, re-added per cycle with databases and consensus/ cleared |
| network | 3 BVNs × 4 validators + Directory (12), bootstrap; BlockchainDB `v0.1.2-0.20260916201027-8751863d31ac`; 1 s blocks; mem_limit 2048 MiB, GOMEMLIMIT 1700 MiB |
| load | 1 tx/s, full mix, chaos off (`72h-watch.conf`) |
| started / stopped | 2026-10-02T14:57:02Z / 2026-10-02T20:51:57Z (5 h 55 min) |

## Timeline

| when (Z) | what |
|---|---|
| 14:57 | network up on the fix build; load from 15:00:51 |
| 15:31 / 15:42 / 15:53 | follower cycles 1–3 (`follower-cycles.sh`) |
| 16:09:05 | **whole-network restart**: all 16 `rm -f`'d, recreated at once, started 16:09:08 |
| 16:09:1x | every validator took the whole-restart exit and was "executing in agreement" within 10 s; bvn3-fol1 and bvn1-fol1 resumed; bvn2-fol1 and bvn3-fol2 joined one partition each |
| 16:09:16 → | **bvn2-val3 and bvn2-val4 fail every block** (see Results); BVN2 produces on two executing validators |
| 16:10 → | the Directory holds BVN2's anchors (2 signatures of 3): 123 at 16:12, 1,385 at 16:40, 1,987 at 16:50; inbound flows from BVN2 red everywhere |
| 16:48 | bvn2-val3's and val4's BVN2 stores copied (live) into `torn-stores/` |
| 16:50 → | the Directory hiccups: 10-second production gaps on node after node, blocks still ~1/s |
| 17:18 | **recovery attempt**: val3/val4 recreated with cleared BVN2 stores and consensus checkpoints, to rejoin |
| 17:18:38 → | they cannot rejoin (`Fewer of this partition's validators serve than sign an anchor` — BVN2's anchors carry 2 signatures of 3) and, with no checkpoint, no longer vote: **BVN2 stops producing at block 8215** |
| 20:51:57 | stopped |

## Results

**Follower cycles: 18 of 18 partition joins ACTIVE and synced.** Time to
ACTIVE 46–66 s (Directory 57–66, BVNs 46–62); synced (20 perfect blocks,
then 100 with no regression) 3–5 min after the add. The BVN joins each saw
0–3 root ≠ anchor repairs in the seconds after their first ACTIVE, every one
returning to agreement within seconds — Paul's loop working as specified.

**The whole restart: the three fixes held.** No partition held at a
consensus round (BVN3's class, #4502/#4503): every round certified with four
certificates. No stream held behind a dropped proof (BVN1's class, #4505): 0
disproved on every node read. All 16 nodes resumed.

**New: two stores torn by the kill (#4507, BlockchainDB#99).** bvn2-val3 and
bvn2-val4 restored at block 4153 — where BVN2 was — and failed their first
block:

```
val3 16:09:16  add entry to main chain: tail chunk 12 of acc://bvn-BVN2.acme/anchors.MainChain holds 1 hashes
               before entry 3938 and element 3937 is missing
val3 16:09:26  produce block: close block: add anchor to root chain: entry 32256 of acc://bvn-BVN2.acme/ledger.RootChain
               is not in the tail and its element is missing
val4 16:12:47  store block ledger: entry 4096 of acc://bvn-BVN2.acme/ledger.BlockLedgerChain is not in the tail
               and its element is missing
```

Read offline from the copied stores: val3's `RootChain Tail(21)` holds 4 of 8
hashes and val4's `BlockLedgerChain Tail(5)` 7 of 8 — a mutable chunk record
on each lost its last rewrite at the kill while the head and later chunks
persisted; val3 also lost one permanent record outright
(`anchors.MainChain.Element.3937`, present on bvn2-val1). The elements named
in the fatal errors are **present** (deep read) and **missing** (shallow
read): `merkle.Chain.tailFallback` refilled the short chunk from elements
through the block's shallow batch, the elements were past the store's window,
and every block failed for ever (348 / 151 failed groups by 16:15; execution
lag 290 / 93 blocks). Fixed on the branch at `082935148`: the adapter reads
history for positional permanent records (`Element`, `Intermediate`), never
for `ElementIndex`; two tests fail without it. The tear itself is the store's
(BlockchainDB#99); the node side — detect a torn store at resume and join
rather than fail for ever — is #4507.

**Network effect of a partition at 2 of 4 executing validators.** Its anchors
never reach the Directory's quorum: held there (2,000+), BVN2's proofs cannot
anchor, inbound flows from BVN2 red on every partition for 2.7 h, the
Directory's heap on bvn1-val1 1.0 GB and climbing, the Directory hiccuping.
The load generator: 3,107 of 17,971 submissions rejected — the two lagging
validators refuse submissions (`Refusing user submissions: execution is
lagging consensus`).

**The recovery attempt was wrong, twice.** Rejoining the two validators with
cleared stores cannot work: no anchor of a partition with two signers reaches
the threshold of three, so no state can be proven to a joiner (`Fewer of this
partition's validators serve than sign an anchor; resuming from this node's
own block` → `cannot resume from its own block: no consensus checkpoint
matched block 1`, looping). And clearing their consensus checkpoints took
their votes away: BVN2 fell to two voters and stopped producing at 8215. A
partition that loses two of four stores cannot be repaired by rejoining them
under the threshold rule — #4501's "resume from whichever node has the
highest signed, validated state" is what that needs, and it is not built.

**The genesis follower.** bvn3-fol1 was never checked before the restart
(executed from genesis, never joined — "never joined" on the panel, Paul).
#4506 (`dafbf946a`, not in this run's build) makes such a node run the root
watch from its first block. After the restart it resumed and was checked
like the others.

## Issues

Filed from this run: #4506, #4507, BlockchainDB#99. Fixed on
`issue-restart-authored-batches`: #4506 (`dafbf946a`), #4507 node-side read
(`082935148`); open: #4507's detect-and-join, BlockchainDB#99, #4501,
#4504 (pre-existing). Proved on Docker: #4502, #4503, #4505.

## What was kept

Committed: this summary, `manifest.md` (with the disturbance record copied
in), `follower-cycles.md`, `follower-cycles.sh`, `config/`.
On disk, not committed: `torn-stores/` (the two copied BVN2 stores, 505 MB —
BlockchainDB#99's evidence), `monitor.csv`, `stats.csv`, `mem.csv`,
`submissions.csv`, `follower.csv`, `storage-stats.csv`, `soak.log`, hourly
and wedge snapshots.
Deleted: `node-logs-live.txt` (775 MB) and the teardown's `node-logs.txt`.
