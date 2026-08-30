# Soak run 20260830T172800Z

**Purpose:** RUN-4H-500-BG: 4h at 500 tps on BlockchainDB eea0b86 (two tiers, two locks, #57/#58) with the adapter running history maintenance in the BACKGROUND (a864f45cf) — the 05:44Z acceptance run 20260830T054402Z still paused every node 88-100 s at block 400 because Compress/MergeBelow ran on the block producer goroutine. N=64 set at creation (0f29e49d2). Loadgen: pacer + race fixes; probe reports query-gate refusals separately (9798296a3). 8 nodes, chaos OFF, 8 shards, mem_limit 2560m + GOMEMLIMIT 2GiB. Watch: no Partition-stalled lines at the 128-block cadence; blocks 3.0 s past block 1000; probe max flat; dyna layer size and file count; maintenanceErrors 0 in stats.json.

| field | value |
|---|---|
| started (UTC) | 2026-08-30T17:28:00Z |
| commit | `d650b2394e0da20166a60c4282040a535738cf84` |
| describe | `10k-tps-606-gd650b2394` |
| branch | `bcdb-storage-backend` |
| uncommitted files | 2 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:218dd3d75ee71e083b135e854aa2835b7956c951d9eaf6a2a44c0791a79ddd20` |
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

## Result (stopped by hand at 17:58Z, 30 min)

No harness verdict. Loadgen 688,114 sent, 135 rejected, 440 tx/s average
(500.0 for the first ten minutes), 17 ADIs. Network torn down.

## Reading

**What the background-maintenance change fixed:** the 88–100 s pauses of
20260830T054402Z are gone — the block producer no longer waits for
Compress/MergeBelow (stats.json maintenanceErrors 0). Read probe over the
run: p50 1.7 ms, p95 34 ms, p99 133 ms, 0 failed, 0 timeouts, 26 query-gate
refusals (now reported separately).

**What remains — and the goroutine dump that names it.** Nodes still paused
11–29 s, clustered just after each 128-block maintenance cadence (blocks
274; 386–405; 522–533). `stall-dumps/174849-bvn2-val1-h.txt`, taken while
BVN2's height was frozen at block 386: the block producer was inside

    KV2.PutDyna → sealDynaIfFull → SegmentStore.SealNext → seal
      → rewriteLiveFile (segstore.go:2220) → BFile.ReadAt → pread

with 71 goroutines queued on the store lock behind it and NO maintenance
goroutine running. That is the dynamic layer's auto-seal: at SealLimit
(100,000 records, the adapter's constant) the store rewrites the ~98 MB live
tail record by record — one pread each — under the exclusive store lock, on
the put path. At ~22,000 dynamic puts per block it fires every 4–5 blocks;
it is only catastrophic when a background history compaction is saturating
the disk at the same time, which is why the pauses cluster after the
cadence. Filed as BlockchainDB#60.

**Adapter mitigation for the next run:** SealLimit 100,000 → ~10,000 bounds
each rewrite to ~1 s. The fix is the store's: cut the live file into a
segment as it stands (dedup belongs to compaction, now off the lock), or
read it in bulk.

Store correctness held: 0 conflicts, 0 misroutes. Paul's local BlockchainDB
`replace` was parked in a stash for this run and restored after.
