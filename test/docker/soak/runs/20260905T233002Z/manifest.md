# Soak run 20260905T233002Z

**Purpose:** DIAGNOSTIC, 8 MIN, CHAOS OFF: run 20260905T225751Z died at minute 5 of load — BVN2 produced 55 synthetics for BVN1 in its blocks 39-41, the Directory anchors carrying their receipts executed at BVN2 at 23:01:49, and the entries were never dispatched: BVN1's requester got 'not yet' from BVN2 for five minutes, every partition went quiet, the Directory starved at height 54. Not reproduced in the simulator (memory or BlockchainDB store, 2x4). This build adds an Info line per dispatched block (module=synthetic: block, entries, streams, send, receipt) and logs the requester's not-yet reason at Info. THIS RUN ANSWERS: is sendSyntheticTransactionsForBlock reached for the producing blocks, on which nodes, with send=true, and what exactly does the source answer the requester. A diagnostic, not a soak: no claim from it.

| field | value |
|---|---|
| started (UTC) | 2026-09-05T23:30:02Z |
| commit | `ce54639f10dcad90d8cc91337cf8a13a7aa33c21` |
| describe | `10k-tps-769-gce54639f1-dirty` |
| branch | `issue-4193-producer-cache` |
| uncommitted files | 281 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:d4b4489ec88bafc5199e0c86d1ca2cabc0ae06d223103b86d588208b7de84ee0` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | off |
| target duration | 8m |
| target TPS | 500 |
| storage | leveldb |
| block interval | 1s |
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1700MiB (effective, from docker inspect) |

Config as run is frozen in `config/`. Results appended below on exit.

## Stopped early by stallkill

- stopped (UTC): 2026-09-05T23:38:30Z
- reason: stalled 245s: Directory (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-05T23:38:31Z |
| elapsed | ?h |
| driver exit | 143 (FAILED) |
| dn height | 38 -> 54 |
| heals | 42 -> 789 |
| chaos events | 1 |
| monitor samples | 13 |
| seizure | none detected |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 1 |
| read-back probe | Whole run: 210 timed reads, p50 1.1 ms, p95 2.7 ms, p99 4.3 ms, max 4.9 ms (txn read, BVN2, entry 154 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 1 wedge-20260905T233557Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
