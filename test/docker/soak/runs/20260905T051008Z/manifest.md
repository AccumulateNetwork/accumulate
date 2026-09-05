# Soak run 20260905T051008Z

**Purpose:** ACCEPTANCE RUN #7 (third start, detached), 12 H, CHAOS OFF, on issue-4193-producer-cache @f8e4bbc50 = run 20260905T032333Z (H1, D7, D8, R3, E10) plus E9 (single-site append), C6 (execution lag bound 8 blocks), H8 (healing requester pulls spans from staging; the source serves only blocks dispatched 8+ blocks ago and answers the dispatched prefix; not-yet is not a failure), DN height from the ledger index. Starts one (20260905T042031Z, requester asked for undispatched tails) and two (20260905T044428Z, launching shell killed at 25 min) are superseded. THIS RUN ANSWERS: heals == 0 and no answered heal requests with nothing dropped; does C6 stop the BVN2 GC spiral (#4220): BVN2 block rate vs BVN1, execution_lag_blocks, batch_store_refusing{execution-lagging}; does memory plateau after the cache horizon (1h); shallowMisses without MainChain.ElementIndex (E9); Directory store history reads ~0. 12 hours: long enough for a claim.

| field | value |
|---|---|
| started (UTC) | 2026-09-05T05:10:08Z |
| commit | `f8e4bbc50a7cc78fdf801052296146eaaeb42f45` |
| describe | `10k-tps-747-gf8e4bbc50-dirty` |
| branch | `issue-4193-producer-cache` |
| uncommitted files | 180 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:749ed5c909d4e0467900725bf6e85c0719548af844bb65fbd1de76100f8bfb4a` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | off |
| target duration | 12h |
| target TPS | 500 |
| storage | BlockchainDB |
| block interval | 1s |
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1700MiB (effective, from docker inspect) |

Config as run is frozen in `config/`. Results appended below on exit.

## Stopped early by stallkill

- stopped (UTC): 2026-09-05T07:30:42Z
- reason: stalled 240s: BVN2 (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-05T07:30:43Z |
| elapsed | 2.28h |
| driver exit | 143 (FAILED) |
| dn height | 11 -> 8137 |
| heals | 0 -> 0 |
| chaos events | 1 |
| monitor samples | 249 |
| seizure | SEIZED at 2026-09-05T05:28:17 :: stalled stream, undelivered for 20 polls :: stuck=0 stuckStream= worst=BVN1->Directory gap=0 deliv=841 undeliv=synthetic BVN2->BVN1 undeliv=94651 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 4 |
| read-back probe | Whole run: 38514 timed reads, p50 3.3 ms, p95 40.5 ms, p99 104.6 ms, max 768.2 ms (txn read, Directory, entry 1830 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 1 wedge-20260905T072817Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
