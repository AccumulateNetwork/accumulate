# Run 20261002T090546Z — summary

Written 2026-10-02 when the run was stopped (Paul: "reset the network, and
redo the tests"). The bulk log it summarises was deleted afterwards (see
"What was kept").

## What it was for

A fresh 12-validator network on the lead with the week's join work (sectioned
BPT sync #4485, redirection, backfill #4493, the handoff overlap #4495),
steady 1 tx/s load, repeated late-follower cycles, then one deliberate
whole-network restart — the same shape as run 20260929T152146Z, which passed.

## Provenance

| | |
|---|---|
| validators | lead `issue-4205-lead` @ **`dfd1efcf8`** (image `disoak-bvn1-val1` `sha256:78049260cf27…`) |
| late followers | `a6893d6ed` (bvn1-fol1, bvn2-fol1, bvn3-fol2), rebuilt per cycle |
| comparison follower | `acc-bvn3-fol1`, started with the network |
| network | 3 BVNs × 4 validators + Directory (12), bootstrap; BlockchainDB; 1 s blocks; mem_limit 2048 MiB, GOMEMLIMIT 1700 MiB |
| load | 1 tx/s, full mix, chaos off (`72h-watch.conf`) |
| started / stopped | 2026-10-02T09:05:46Z / 2026-10-02T14:53:35Z (5 h 48 min) |

## Timeline

| when (Z) | what |
|---|---|
| 09:05 | network up, load starts |
| 10:3x–12:37 | follower cycles 1–3 (`follower-cycles.sh`): all six partition joins synced under the 20-perfect-blocks + 100-block-hold rule, 0 regressions, 0 repairs |
| 12:47:06 | **whole-network restart**: all 12 validators + 4 followers `rm -f`'d and recreated at once |
| 12:47:2x | every validator took the whole-restart exit and executed in agreement from its own last block (BVN1 12949, BVN2 12870, BVN3 12920, Directory 13039) |
| 12:47 → | **BVN3 never produced another block** (round 25839, block 12920) |
| 12:47 → | **BVN1's BVN2→BVN1 synthetic stream held at entry 1896**; 8,846 entries queued behind it by 14:10 |
| 13:05:53 | BVN3's four validators recreated on diag build `3f1705364`: stall reproduced with round state |
| 13:15–13:17 | bvn3-val3 run at Debug for two minutes: both unvotable headers defer on `Missing batch` |
| 13:54 / 14:05 | bvn1-fol1 recreated on diag builds (`3f1705364`, `8ff2d9947`), data kept: the disproved proof names Directory block 13035, its root is in the chain at index 12289, its element index is not |
| 14:3x | bvn1-fol1 stopped; its BVN1 store and a live copy of bvn1-val1's read offline |
| 14:53:35 | stopped |

## Results

**The whole restart itself worked**: 16 of 16 resumed, every validator
"executing in agreement" within 20 s. What followed did not.

**BVN3 (#4502, #4503).** bvn3-val1 and val2 had authored round 25839's
header, each naming one batch, when the others stopped. The block-12920
checkpoint's `Retain` removed every batch no certificate named — those two
among them. Restored, the headers came back with the author guard and
without their payload: bvn3-val3 at Debug deferred both on every rebroadcast
(`Missing batch for header`, 20× each), the fetch found no holder, the round
held at 2 certificates of 3, and neither author could author again. On disk
at bvn3-val1/2/3: 142 certificates, 72 batches, the `authored` header,
neither batch. Fixed on `issue-restart-authored-batches`: the checkpoint
keeps the authored header's batches (#4502); the vote gate asks for every
missing batch, not one per rebroadcast (#4503, found reproducing this in the
simulator: post-restart headers carried 553–756 re-proposed batches).

**BVN1 (#4505).** The pulled proof for entry 1896 terminates at the root of
Directory block 13035. Every BVN1 node holds it (API by hash: index 12289 on
all four validators; diag follower: found by scan in the same batch) — in
packed history, which the executor's shallow batch cannot read
(BlockchainDB's window). `provingAnchorIndex` read NotFound and the proof was
dropped as disproved, for good; 1,011 drops on bvn1-val1 by 14:00. Offline
reads of two stores: `Element`/`ElementIndex` records exist only for the
newest ~100 chain entries. Fixed: a proof whose anchor has executed is
looked for through a deep view before it is called disproved.

**Directory joiners.** BVN3's four Directory nodes, recreated at 13:05 into
a running Directory, joined and never finished: their next block needs
BVN3's anchor 6292, which every Directory peer holds unexecuted because BVN3
is stalled (379 × `The next block has a gap … bvn-BVN3.acme->dn.acme/anchors
missing=6292`). Downstream of #4502, not a join defect. Their `only K of 12
signed` lines are the root watch reading seconds-old anchors.

**Observability found wanting**, not fixed here: after any restart the root
watch asks for the newest anchor the ledgers name — the last *delivered* one
— while the reseeded cache holds from delivered+1 (`no validator answers
anchor 6292` on every BVN3 node for 2 h); the dashboard called BVN1
"stalled" (by delivery) without saying which stream or that its blocks were
fine; a disproved proof said nothing until the diag build.

**Also found:** `TestRestartOneNode` (consim) fails on the untouched lead
(#4504).

**Why 20260929T152146Z passed the same restart:** both failures need a
coincidence at the kill — two of four validators dying with an authored,
uncertified, non-empty header; a BVN→BVN entry in flight whose pull comes
after its anchor left the store's window. Neither mechanism is in code that
changed between `03945b6eb` and `dfd1efcf8`.

## Issues

Filed from this run: #4502, #4503, #4504, #4505. Fixed on
`issue-restart-authored-batches` (ae94c647b): #4502, #4503, #4505 — each with
a test that fails without it; targeted suites green. Not yet on Docker: that
is the next run.

## What was kept

Committed: this summary, `manifest.md`, `chaos.log` (copied into the
manifest), `follower-cycles.md`, `follower-cycles.sh`, `config/`.
On disk, not committed: `monitor.csv`, `stats.csv`, `mem.csv`,
`submissions.csv`, `follower.csv`, `storage-stats.csv`, `soak.log`, the
hourly and wedge snapshots.
Deleted: `node-logs-live.txt` (1.4 GB), `node-logs.txt` (402 MB, the teardown dump), `node-logs-acc-bvn1-fol1-before-diag.txt` (18 MB).
