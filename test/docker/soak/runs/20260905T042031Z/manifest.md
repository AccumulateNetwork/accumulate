# Soak run 20260905T042031Z

**Purpose:** ACCEPTANCE RUN #7, 12 H, CHAOS OFF, on issue-4193-producer-cache @5d8785c7e = run 20260905T032333Z (H1, D7, D8, R3, E10) plus E9 (single-site append), C6 (execution lag bound 8 blocks), H8 (healing requester pulls spans from staging; heals must stay 0 with nothing dropped), DN height from the ledger index. THIS RUN ANSWERS: does C6 stop BVN2 GC spiral (#4220) — BVN2 block rate vs BVN1, execution_lag_blocks, batch_store_refusing{execution-lagging}; does memory plateau after the cache horizon (1h); heals == 0 and heal_requests_total == 0; shallowMisses without MainChain.ElementIndex (E9); Directory store history reads ~0 (getDnHeight fixed). 12 hours: long enough for a claim.

| field | value |
|---|---|
| started (UTC) | 2026-09-05T04:20:31Z |
| commit | `5d8785c7e93d4e263e7e5a26736a13945c1252ce` |
| describe | `10k-tps-744-g5d8785c7e-dirty` |
| branch | `issue-4193-producer-cache` |
| uncommitted files | 169 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:81fe9f57638a6961309c0f8502189c9152d309cbc35da0e2c5bbb96038277501` |
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
| ended (UTC) | 2026-09-05T04:36:03Z |
| elapsed | 0.2h |
| driver exit | 143 (FAILED) |
| dn height | 15 -> ? |
| heals | 0 -> 0 |
| chaos events | 1 |
| monitor samples | 23 |
| seizure | none detected |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 0 |
| read-back probe | Whole run: 1326 timed reads, p50 0.6 ms, p95 6.7 ms, p99 22.4 ms, max 59.0 ms (txn read, Directory, entry 158 blocks old); 624 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 0  |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.

## Stopped early by stallkill

- stopped (UTC): 2026-09-05T04:38:02Z
- reason: monitor unreachable for 120s

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Stopped at 15 minutes

Stopped by hand at 04:36Z. In its first activations the healing requester
asked BVN1 for the whole undelivered tail (`[4286, 7885]`, 3,600 entries the
source had produced but not yet dispatched, because the Directory had not
receipted those blocks) and the source refused the whole span as `notReady`;
one span was answered and 38 in-flight entries were pulled twice (heals 76 with
nothing dropped). Execution lag stayed 0 on both BVNs under C6 for the 15
minutes. Fixed before the next run: the source serves only blocks dispatched
at least `InFlightBlocks` (8) blocks ago and answers the dispatched prefix of a
span; the requester treats "not yet" as neither a gap nor a failure; the
simulator commits empty blocks like the node does. The run is superseded by
the next one.
