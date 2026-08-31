# Soak run 20260831T060018Z

**Purpose:** SOAK-12H-500-WIN: 12h CHAOS soak at 500 tps on BlockchainDB 5c7b0e5 (fix/streaming-merge: promote-only seal #60, streaming merge #59, WINDOWED permanent reads — Get stops at 2N, adapter falls back to GetDeep on a miss, bf1cfc50d) + background maintenance, N=64. Prior 17-min run 20260831T045856Z: auto-seal pauses gone, but history bloom probing was 23% of CPU and climbing — these commits are the answer. Chaos ON (first live test of #4173 restart fixes). Acceptance: CPU flat over hours (the windowed read is the fix under test); zero stall lines; blocks 3.0 s; restarted nodes rejoin without seal-height errors; probe reads of OLD entries still answer (GetDeep path); heals return to 0 after chaos; maintenanceErrors 0; no conflicts/misroutes.

| field | value |
|---|---|
| started (UTC) | 2026-08-31T06:00:19Z |
| commit | `bf1cfc50d60b42f376add23acb1730f69e313674` |
| describe | `10k-tps-610-gbf1cfc50d` |
| branch | `bcdb-storage-backend` |
| uncommitted files | 2 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:433f9439da81f123282892a5bf024f26d2af33d0a4ec17dcd8913928aaf4e91d` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | on |
| target duration | 12h |
| target TPS | 500 |
| storage | blockchainDB |
| memory budget | mem_limit 2560m, GOMEMLIMIT 2GiB |

Config as run is frozen in `config/`. Results appended below on exit.

## Stopped early by stallkill

- stopped (UTC): 2026-08-31T06:26:06Z
- reason: stalled 241s: BVN2,Directory (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-08-31T06:26:07Z |
| elapsed | 0.38h |
| driver exit | 143 (FAILED) |
| dn height | 8 -> 416 |
| heals | 0 -> 11253 |
| chaos events | 6 |
| monitor samples | 5 |
| seizure | SEIZED at 2026-08-31T06:17:08 :: stalled stream, undelivered for 20 polls :: stuck=0 stuckStream= worst=BVN1->Directory gap=0 deliv=5295 undeliv=synthetic BVN2->BVN1 undeliv=4233 |
| reconcile pulls (#4073) | 372 |
| stalled channels at end | 4 |
| read-back probe | Whole run: 4650 timed reads, p50 1.5 ms, p95 5.3 ms, p99 37.0 ms, max 8040.5 ms (chain read, BVN1, entry 434 blocks old); 2 failed, 2 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 1 wedge-20260831T062401Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.

## Verdict: not a storage failure — every transaction routed to one worker (#4179)

Storage is exonerated. CPU 18–27%, RSS 1.5–1.7 GiB against a 2.5 GiB limit, and
the eight goroutine dumps in `probe-20260831T062603Z/` hold ~11 BlockchainDB
frames between them, none blocking. Every block-production loop was parked in
`select` waiting for a committed group, so consensus was not committing —
execution was not behind.

The wedge signature is 35,999 copies of one warning, **every one `workerID=1`**:

```
Batch store over limit with un-evictable own uncommitted batches (commit is lagging)
  limit=1000 limitBytes=8388608 ownUncommitted=19 stored=19 storedBytes=8711225 workerID=1
```

`limitBytes=8388608` is exactly 32 MB / 4 workers. Evictions spread across all
four workers, because received batches route by digest — but worker 1 evicted
11x as often as any other:

| workerID | evictions | over-limit warnings |
|---|---|---|
| 0 | 966 | 0 |
| 1 | **10916** | **35999** |
| 2 | 973 | 0 |
| 3 | 965 | 0 |

Cause: `Service.SubmitTransaction` called `SubmitTransactionFor("")`, which
routes on the hash of an empty key — a constant, worker 1 for every worker
count we ship. All own-authored batches landed in one worker's 1/N byte share;
own batches cannot be evicted, so it evicted peers' batches, which are what
certificates need; those were refetched, commit lagged further, and it ran away.

Timeline: 06:16 first evictions and backpressure; 06:17 seizewatch; 06:21
thousands of over-limit warnings per minute; 06:23 Directory stalls at block
418 (9.7 s/block DN, 25.0 s/block BVN2, 3.0 s/block BVN1). The chaos pause of
`acc-bvn2-val2` was 06:22 — six minutes after the collapse started, not the
trigger. Reconcile amplified it: `produced=23671 received=20751 requested=200`
repeating with `received` frozen.

The windowed-read fix under test could not be judged in 23 minutes, but showed
no counter-evidence: bvn2 sat in the twenties where the previous run climbed
through 140–160%, and the probe answered old entries via GetDeep (p99 37 ms;
the 8.0 s max is a chain read during the wedge).

Loadgen skips are still not zero: 27,987, essentially all `add-page-key`
(13,693) and `add-key-book` (542) — the ready predicates do not cover those two.
