# Soak run 20260906T134054Z

**Purpose:** Acceptance after the memory/execution review fixes (#4222, #4224, #4226–#4246) on issue-4193-producer-cache @ 9390f3c42: 12h, 500 tps, chaos off, BlockchainDB

| field | value |
|---|---|
| started (UTC) | 2026-09-06T13:40:54Z |
| commit | `9390f3c426500d85806eb303300fbdb1eadd23e8` |
| describe | `10k-tps-817-g9390f3c42-dirty` |
| branch | `issue-4193-producer-cache` |
| uncommitted files | 281 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:000cd9a7060655dd8b33e02c951b4d7788fd94fad10ebad52d23d542e5161d77` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | off |
| target duration | 12h |
| target TPS | 500 |
| storage |  (docker-network.yml) |
| block interval | 1s |
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1700MiB (effective, from docker inspect) |

Config as run is frozen in `config/` (soak.conf, the compose and network files). Results appended below on exit.

## Stopped early by stallkill

- stopped (UTC): 2026-09-06T18:07:24Z
- reason: stalled 241s: BVN2 (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-06T18:07:24Z |
| elapsed | 4.37h |
| driver exit | 143 (FAILED) |
| dn height | 10 -> 15634 |
| heals | 0 -> 1090982 |
| chaos events | 1 |
| monitor samples | 483 |
| seizure | SEIZED at 2026-09-06T13:50:37 :: stuck=0 stuckStream= worst=BVN2->BVN1 gap=217 deliv=31330 undeliv=synthetic BVN2->BVN1 undeliv=705 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 3 |
| read-back probe | Whole run: 75750 timed reads, p50 2.6 ms, p95 14.2 ms, p99 40.5 ms, max 1973.2 ms (txn read, Directory, entry 9688 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 1 wedge-20260906T180450Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
