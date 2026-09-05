# Soak run 20260905T144928Z

**Purpose:** ACCEPTANCE RUN #7 (seventh start), 12 H, CHAOS OFF, on issue-4193-producer-cache @a176d43ba = sixth start plus: the requester decides only when its executor has no committed-but-unexecuted block (sixth start healed 1,863 duplicates out of its own backlog). THIS RUN ANSWERS: heals == 0 for 12 h with nothing dropped; C6 lag bound on BVN2; memory plateau after the cache horizon (1h); E9 shallow misses; Directory store history reads ~0.

| field | value |
|---|---|
| started (UTC) | 2026-09-05T14:49:28Z |
| commit | `180427eb1a1d025a60cd47d77ebbe35ecd2b0b68` |
| describe | `10k-tps-753-g180427eb1-dirty` |
| branch | `issue-4193-producer-cache` |
| uncommitted files | 266 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:fdad2a319ba7c6710418b44c78ac9641969d6ed65f3a48c72c232ec3a4fb2c1c` |
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
| ended (UTC) | 2026-09-05T15:38:08Z |
| elapsed | 0.73h |
| driver exit | 1 (FAILED) |
| dn height | 15 -> 2667 |
| heals | 0 -> 0 |
| chaos events | 1 |
| monitor samples | 82 |
| seizure | SEIZED at 2026-09-05T15:08:05 :: stuck=0 stuckStream= worst=BVN1->BVN2 gap=124 deliv=27564 undeliv=synthetic BVN2->BVN1 undeliv=762 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 4 |
| read-back probe | Whole run: 10644 timed reads, p50 2.3 ms, p95 20.8 ms, p99 66.7 ms, max 897.4 ms (txn read, BVN2, entry 866 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 0  |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.

## Stopped at 49 minutes — heals held at 0; the network oscillated on the lag bound

The first criterion held for the whole run: heals 0, no heal request
answered, zero receipt rejections, every stream delivering. Stopped by hand
at 15:38Z because both BVNs oscillated on the C6 bound: refusal, batches
piling up, lag clearing, the header builder draining every batch into one
header, one block of ten to seventeen seconds, lag past the bound again.
Load accepted fell to 392 tps with 944k submissions refused. Fixed before
the next run: a header carries at most MaxHeaderBytes (1 MiB) of batches and
the rest wait for the next header (455a82ee1).
