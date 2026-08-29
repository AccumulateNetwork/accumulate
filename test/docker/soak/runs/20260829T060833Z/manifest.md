# Soak run 20260829T060833Z

**Purpose:** 4h at 500 tps on BlockchainDB f5ab54d (#4165) with the READ-BACK PROBE (readprobe.py, 45367177c): committed entries sampled every 20s, 150 re-read every 60s, slowest per round with entry age, whole-run latency-by-age report at teardown. Memory budget raised as one factor (mem_limit 2560m + GOMEMLIMIT 2GiB) because the 400 tps run reached 1024 MiB RSS against the 1200 MiB ceiling. Loadgen with the pacer fix 26c24bafe (achieved should equal target) and the page-key race fix. 8 nodes, chaos OFF, 8 shards. Watch: achieved tps, block interval, BVN2 CPU (saturated at 400), RSS vs 2 GiB, read-probe max and its trend with age, perm segment count (MergeBelow fires past block 512), conflicts/misroutes 0.

| field | value |
|---|---|
| started (UTC) | 2026-08-29T06:08:33Z |
| commit | `45367177c834a9e20881908621a41747bbc03d8e` |
| describe | `10k-tps-588-g45367177c` |
| branch | `bcdb-storage-backend` |
| uncommitted files | 1 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:78f59f349775b0df0059684f47001894dafe375e864bf95f4486ba2d9d20f692` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | off |
| target duration | 4h |
| target TPS | 500 |
| storage | blockchainDB |
| memory budget | mem_limit 2560m, GOMEMLIMIT 2GiB |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-08-29T06:41:07Z |
| elapsed | 0.5h |
| driver exit | 1 (FAILED) |
| dn height | 8 -> 617 |
| heals | 0 -> 2441 |
| chaos events | 1 |
| monitor samples | 7 |
| seizure | SEIZED at 2026-08-29T06:32:33 :: stuck=0 stuckStream= worst=BVN2->Directory gap=255 deliv=30521 undeliv=synthetic BVN2->BVN1 undeliv=2050 |
| reconcile pulls (#4073) | 97 |
| stalled channels at end | 4 |
| read-back probe | Whole run: 6720 timed reads, p50 1.4 ms, p95 4.2 ms, p99 40.2 ms, max 3750.5 ms (txn read, BVN2, entry 550 blocks old); 12 failed. |
| wedge captures (#4125) | 0  |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.

## Stopped early by stallkill

- stopped (UTC): 2026-08-29T06:43:01Z
- reason: monitor unreachable for 120s

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Reading (06:45Z)

**Ended by a loadgen panic, not the network:** `panic: invalid argument to
Intn` in set-threshold on a page another action had just emptied
(actions.go:323) — the second page-mutation race reachable since ADIs
exist (#4176). The harness tore the network down on the exit.

**What 33 minutes at 500 tps showed on BlockchainDB f5ab54d:** clean for the
first ~20 min (498 tx/s achieved — the pacer fix works — 3.0 s blocks, 0
heals), then BVN2 fell behind: 4.3 s blocks, 37 blocks behind the DN by
06:38, heals 0 → 2,441, achieved rate 498 → ~295 tx/s. Same shape as the
leveldb collapses (treasury partition lags → healing), reached at 500 tps
instead of 100–250.

**Where it went, from the 5-minute pprof captures (bvn2-val1, `profiles/`):**
`SegmentStore.get` 15.7 % of CPU, 62 % of that `segment.lookup` — the walk
over sealed permanent segments on a hit. 1,052 perm segment files on
bvn2-val1's BVN database at commit 500; MergeBelow's first merge (commit
640) never arrived. Bloom 19 %.

**Read-back probe:** 6,720 reads, p50 1.4 ms, p95 4.2 ms, p99 40 ms, max
3,750 ms; 12 failed (API timeouts). Median flat with age (0–100 blocks
1.3 ms, 100–1000 1.4 ms) — the tail is not age, it is commits: every slow
read coincided with a block commit, because the adapter held its lock
across the write-through (puts + the dual-layer fsync + dyna sealing).
Fixed for the next run in 8b3f91951 (write-through outside the lock).

**Disk:** database avg 5.5 GB/node, max 7.4 GB, growing 12.5 GB/h per node;
bvn2-val1 bvnn = perm 2.1 GB / dyna 5.1 GB (284 dyna segments). The dynamic
layer dominates: every mutable rewrite appends a record and Compress every
128 commits is not keeping up at this rate. Host root was at 83 % before the
run; a disk guard (stop under 150 GB free) was armed. Store correctness held
throughout: 0 conflicts, 0 misroutes, 1.19 % permanent duplicates.

## Why BVN2 stalled (07:05Z, from the profiles and node logs)

The loadgen panic ended the run; it did not cause the stall. BVN2's executor
got slower every block in proportion to the number of sealed BlockchainDB
segments: block time 3.0 s through block 448, then 3.7 / 4.1 / 4.4 / 5.0 /
5.2 s per 2-minute window, while `SegmentStore.get` rose from 9 % to 20 % of
CPU and `segment.lookup` (the walk over sealed segments on a hit) from 4 % to
15 % across the 06:16–06:37 profiles. Consensus symptoms followed, not led:
batch queue 500 → 1,000 at 06:38–06:40, then backpressure (160), then
missing-batch deferrals (1 → 95/min) and re-shares.

Two store-side causes: (1) hits walk every sealed segment and the count grows
one per block — 1,052 perm + 284 dyna segments on bvn2's BVN database by
commit 500 (MergeBelow's first merge, at commit 640, never arrived; Compress
waits for 25 % garbage); 3.3 M walks over the run. (2) `KV2.Get` and
`SegmentStore.Get` hold an exclusive mutex across the walk, so the 8
execution shards' reads serialize on one lock — bvn2-val1 at ~110 % CPU, one
core. BVN2 first because its database is 2.3× BVN1's. Filed as
BlockchainDB#32. The 897 "Local delivery failed" errors are the loadgen's
fail:send-to-void cases.
