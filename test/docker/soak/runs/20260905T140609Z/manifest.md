# Soak run 20260905T140609Z

**Purpose:** ACCEPTANCE RUN #7 (fifth start), 12 H, CHAOS OFF, on issue-4193-producer-cache @05ad3b704 = fourth start (mark-point fix, confirmed: streams delivered, zero receipt rejections) plus: an entry covered by a staged proof waiting for its anchor is not a gap (fourth start pulled 37k duplicates with nothing dropped while BVN2 lagged). THIS RUN ANSWERS: heals == 0 and no answered heal requests for 12 h with nothing dropped; C6 lag bound on BVN2; memory plateau after the cache horizon (1h); E9 shallow misses; Directory store history reads ~0.

| field | value |
|---|---|
| started (UTC) | 2026-09-05T14:06:09Z |
| commit | `05ad3b704e416b0490778d291dcaf8cf6b77d390` |
| describe | `10k-tps-750-g05ad3b704-dirty` |
| branch | `issue-4193-producer-cache` |
| uncommitted files | 249 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:390f80fb89dae057efc4437a9270645d2855de22087ccbfa7816cc72fcf2fac6` |
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

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-05T14:26:34Z |
| elapsed | 0.27h |
| driver exit | 1 (FAILED) |
| dn height | 10 -> 1006 |
| heals | 0 -> 5160 |
| chaos events | 1 |
| monitor samples | 31 |
| seizure | SEIZED at 2026-09-05T14:19:31 :: stuck=0 stuckStream= worst=BVN2->BVN1 gap=202 deliv=52030 undeliv=synthetic BVN2->BVN1 undeliv=732 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 4 |
| read-back probe | Whole run: 2496 timed reads, p50 2.1 ms, p95 24.7 ms, p99 73.7 ms, max 493.7 ms (chain read, Directory, entry 994 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 0  |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.

## Stopped at 20 minutes — duplicates again, from the lag bound

Streams delivered throughout (received tracks produced, zero receipt
rejections). Stopped by hand at 14:26Z: heals reached 5,160 with nothing
dropped. Once BVN2's executor lag reached the C6 bound its primary proposed
no batches, so BVN1's packages could not land for as long as the lag lasted
(up to ~17 blocks), and the requester's two-activation notice (8 blocks)
called them gaps; the Directory likewise asked BVN2 for entries BVN2's
lagging executor dispatched late. Fixed before the next run: a partition
whose own execution is lagging asks for nothing, and the notice age is six
activations, longer than the lag bound on either side plus a flight.
