# Soak run 20260918T023054Z

**Purpose:** 30m 100tps chaos proof of #4290 on DI c37ef6d60: rejoin pull (synthetics + anchors) + RejoinGrace on both caches + checkpointed committed-digest set; every restarted validator must keep agreeing on the anchor body

| field | value |
|---|---|
| started (UTC) | 2026-09-18T02:30:54Z |
| commit | `c37ef6d609d4aecab0f248273466cb834d68e20f` |
| describe | `10k-tps-777-gc37ef6d60-dirty` |
| branch | `dagbft-integration` |
| uncommitted files | 351 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:0245476d6d427f8b0846c3801a06e7f60a578640d460a4f3dac3c39c27cb3473` |
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

## Stopped early by stallkill

- stopped (UTC): 2026-09-18T02:53:09Z
- reason: stalled 245s: BVN1 (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-18T02:53:09Z |
| elapsed | 0.33h |
| driver exit | 143 (FAILED) |
| dn height | 37 -> 1165 |
| heals | 0 -> 5085 |
| chaos events | 13 |
| monitor samples | 38 |
| seizure | SEIZED at 2026-09-18T02:36:29 :: stuck=n/a stuckStream= worst=BVN1->BVN2 gap=98 deliv=600 undeliv=synthetic BVN2->BVN3 undeliv=88 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 9 |
| read-back probe | Whole run: 3676 timed reads, p50 1.5 ms, p95 3.6 ms, p99 17.5 ms, max 8040.4 ms (chain read, Directory, entry 829 blocks old); 24 failed, 24 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 1 wedge-20260918T024826Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
