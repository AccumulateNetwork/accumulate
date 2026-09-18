# Soak run 20260918T021748Z

**Purpose:** 30m 100tps chaos proof of #4290 on DI 1f30c573e: rejoin pull (synthetics + anchors) + RejoinGrace + checkpointed committed-digest set; every restarted validator must keep agreeing on the anchor body

| field | value |
|---|---|
| started (UTC) | 2026-09-18T02:17:48Z |
| commit | `1f30c573eb549f1d62e461dd4aa20ae5b3d4ff19` |
| describe | `10k-tps-775-g1f30c573e-dirty` |
| branch | `dagbft-integration` |
| uncommitted files | 350 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:1d6f6ac3c6393a73e3fbf826d402619d4b00e4d5f9266f29f573eb16bc55fb1e` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| fault model | restart or pause one BVN container every 120s + 0-60s |
| topology | 3 BVNs, 12 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 BVN3 |
| chaos | on |
| target duration | 30m |
| target TPS | 100 |
| storage |  (docker-network.yml) |
| block interval | 1s |
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1700MiB (effective, from docker inspect) |

Config as run is frozen in `config/` (soak.conf + override.conf, the compose and network files). Results appended below on exit.

## Stopped early by hand

- stopped (UTC): 2026-09-18T02:25:26Z
- reason: the restarted Directory validator rejoined and agreed 12/12 (first time); its BVN1 validator's anchor pull found the Directory's anchor already released ('anchor 111 is not in the cache') and it diverged. Anchor acks now wait RejoinGrace; relaunching on that image.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-18T02:26:18Z |
| elapsed | 0.09h |
| driver exit | 143 (FAILED) |
| dn height | 38 -> 354 |
| heals | 0 -> 384 |
| chaos events | 6 |
| monitor samples | 11 |
| seizure | SEIZED at 2026-09-18T02:23:23 :: stuck=n/a stuckStream= worst=BVN1->BVN2 gap=82 deliv=529 undeliv=synthetic BVN2->BVN3 undeliv=101 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 3 |
| read-back probe | Whole run: 406 timed reads, p50 1.5 ms, p95 4.3 ms, p99 8040.0 ms, max 8040.5 ms (txn read, BVN3, entry 340 blocks old); 16 failed, 11 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 0  |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
