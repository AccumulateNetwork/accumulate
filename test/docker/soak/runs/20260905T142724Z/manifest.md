# Soak run 20260905T142724Z

**Purpose:** ACCEPTANCE RUN #7 (sixth start), 12 H, CHAOS OFF, on issue-4193-producer-cache @426aac799 = mark-point fix (confirmed) + requester: staged proof is not a gap, a lagging partition asks for nothing, notice 6 activations / expected 10. Fifth start healed 5,160 duplicates while BVN2 sat at the C6 lag bound. THIS RUN ANSWERS: heals == 0 for 12 h with nothing dropped (the first criterion); C6 lag bound on BVN2; memory plateau after the cache horizon (1h); E9 shallow misses; Directory store history reads ~0.

| field | value |
|---|---|
| started (UTC) | 2026-09-05T14:27:24Z |
| commit | `426aac799f85d78bca25b25f2d8b5b6b61be8dcf` |
| describe | `10k-tps-751-g426aac799-dirty` |
| branch | `issue-4193-producer-cache` |
| uncommitted files | 257 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:3dfc8e81f552e5cde64cc0f3602b05aa54c4104fd95e0ae1f534d742827dc5a9` |
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
| ended (UTC) | 2026-09-05T14:48:49Z |
| elapsed | 0.29h |
| driver exit | 1 (FAILED) |
| dn height | 15 -> 1072 |
| heals | 0 -> 1863 |
| chaos events | 1 |
| monitor samples | 33 |
| seizure | SEIZED at 2026-09-05T14:43:51 :: stuck=0 stuckStream= worst=BVN2->BVN1 gap=332 deliv=64410 undeliv=synthetic BVN1->BVN2 undeliv=656 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 4 |
| read-back probe | Whole run: 2802 timed reads, p50 1.9 ms, p95 14.8 ms, p99 45.3 ms, max 1744.9 ms (txn read, BVN2, entry 925 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 0  |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.

## Stopped at 21 minutes — duplicates from the executor's own backlog

Streams delivered throughout, zero receipt rejections. Stopped by hand at
14:48Z at heals 1,863: both BVNs touched the lag bound in episodes, and the
requester, checking only for the bound at activation time, pulled entries
that sat in its own partition's backlog of committed, unexecuted blocks.
Fixed before the next run: the requester asks for nothing while a single
committed block is unexecuted.
