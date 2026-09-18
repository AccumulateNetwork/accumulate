# Soak run 20260918T015356Z

**Purpose:** 30m 100tps chaos proof of #4290 on DI 48036314c: rejoin pull + RejoinGrace + checkpointed committed-digest set; every restarted validator must keep agreeing on the anchor body

| field | value |
|---|---|
| started (UTC) | 2026-09-18T01:53:56Z |
| commit | `48036314c6fde7b51e51fd7502f7fdd434307910` |
| describe | `10k-tps-773-g48036314c-dirty` |
| branch | `dagbft-integration` |
| uncommitted files | 349 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:17c3258f110c577031ad726b8450cb7d91d61504e95272e3183db922be9330e3` |
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

- stopped (UTC): 2026-09-18T02:13:00Z
- reason: stalled 247s: BVN1,BVN2,BVN3,Directory (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-18T02:13:01Z |
| elapsed | 0.28h |
| driver exit | 143 (FAILED) |
| dn height | 37 -> 978 |
| heals | 0 -> 3473 |
| chaos events | 11 |
| monitor samples | 32 |
| seizure | SEIZED at 2026-09-18T01:59:30 :: stuck=n/a stuckStream= worst=BVN1->BVN3 gap=92 deliv=553 undeliv=synthetic BVN2->BVN1 undeliv=120 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 9 |
| read-back probe | Whole run: 2768 timed reads, p50 1.6 ms, p95 4.9 ms, p99 3243.3 ms, max 8040.4 ms (txn read, BVN1, entry 396 blocks old); 25 failed, 25 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 1 wedge-20260918T021047Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
