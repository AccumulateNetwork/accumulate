# Soak run 20260918T014155Z

**Purpose:** 30m 100tps chaos proof of #4290 rejoin + RejoinGrace on DI 6cb83db5d; restarts must not shrink the Directory anchor quorum

| field | value |
|---|---|
| started (UTC) | 2026-09-18T01:41:55Z |
| commit | `a66b39afa7378af0dcb83679bf85221f2caefdea` |
| describe | `10k-tps-769-ga66b39afa` |
| branch | `dagbft-integration` |
| uncommitted files | 347 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:670fb8c512fa5752e497862ddbf9904ca0c133c9f0614d8bca428a8438319739` |
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

- stopped (UTC): 2026-09-18T01:53:10Z
- reason: the rejoin held the right entries but every restarted node still executed a different first block (109 msgs/16 batches vs 13/3 at the same leader round): Bullshark's committed-digest set was not in the checkpoint. Fixed in 394f6c192; relaunching on that image.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-18T01:53:11Z |
| elapsed | 0.15h |
| driver exit | 143 (FAILED) |
| dn height | 38 -> 569 |
| heals | 0 -> 741 |
| chaos events | 9 |
| monitor samples | 18 |
| seizure | SEIZED at 2026-09-18T01:46:50 :: stuck=n/a stuckStream= worst=BVN2->BVN1 gap=359 deliv=1607 undeliv=synthetic BVN2->BVN1 undeliv=133 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 9 |
| read-back probe | Whole run: 872 timed reads, p50 1.5 ms, p95 7.5 ms, p99 8039.9 ms, max 8040.2 ms (txn read, BVN3, entry 339 blocks old); 13 failed, 13 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 0  |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
