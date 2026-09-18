# Soak run 20260905T225751Z

**Purpose:** ACCEPTANCE RUN #7 (ninth start), 12 H, CHAOS OFF, on issue-4193-producer-cache @70e33b0ba = eighth start (C6 lag bound, 256 KiB header cap) plus E12: a synthetic chain per destination (a proof covers one chain, its index is the sequence number), the stage as two index-aligned lists (entries and validated hashes from Delivered+1), anchors through the stage (held below quorum, tossed at or below Delivered, pulled by the requester; the source-side anchor re-send is deleted), the requester walks anchor streams beside synthetic ones. Eighth start held heals at 0 for 37 min with 22 ten-second blocks and was stopped. THIS RUN ANSWERS: heals == 0 for 12 h with nothing dropped, and heal_requests answered == 0 (anchors dispatched once arrive; nothing is asked for that is in flight); every synthetic stream ends at received == delivered (streams-final.txt); no 10 s blocks under the header cap, load accepted near 500 tps; memory plateau after the cache horizon (1h); Directory store history reads ~0. 12 hours: long enough for a claim.

| field | value |
|---|---|
| started (UTC) | 2026-09-05T22:57:51Z |
| commit | `70e33b0ba590408baf8b95d7e3b0b9b85ee987b7` |
| describe | `10k-tps-768-g70e33b0ba-dirty` |
| branch | `issue-4193-producer-cache` |
| uncommitted files | 276 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:2c82d721ed8b266554516e1c897c9318d1262b9864c0f6e63e294d284ff4572a` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | off |
| target duration | 12h |
| target TPS | 500 |
| storage | leveldb |
| block interval | 1s |
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1700MiB (effective, from docker inspect) |

Config as run is frozen in `config/`. Results appended below on exit.

## Stopped early by stallkill

- stopped (UTC): 2026-09-05T23:06:28Z
- reason: stalled 244s: Directory (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-05T23:06:29Z |
| elapsed | ?h |
| driver exit | 143 (FAILED) |
| dn height | 38 -> 54 |
| heals | 42 -> 671 |
| chaos events | 1 |
| monitor samples | 9 |
| seizure | none detected |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 1 |
| read-back probe | Whole run: 210 timed reads, p50 1.0 ms, p95 2.1 ms, p99 2.4 ms, max 2.7 ms (txn read, BVN2, entry 274 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 1 wedge-20260905T230355Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
