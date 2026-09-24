# Soak run 20260919T115321Z

**Purpose:** 1 h at 100 tps, chaos off, on DI bff83e840: re-establish the no-chaos baseline after ~25 unsoaked merges since the last clean 1 h/100 tps run (ea34d9284, 20260917T161555Z) -- E11 #4290-#4296, #4304, block-ledger-last (three incompatible state roots, no version gate), #4295 served-body, #4344/#4345 ExecutedBlock, #4348 x2. Paul: 'I believe we merged code we should not have merged.' Every run since 09-17 16:15 was a chaos proof that died in the join; the 100 tps path itself has not been re-established. Pass = 1 h to completion, 0 stalls, heals 0, s/block flat.

| field | value |
|---|---|
| started (UTC) | 2026-09-19T11:53:21Z |
| commit | `bff83e84044f5731e0a475ba5978539257e3e6f4` |
| describe | `10k-tps-881-gbff83e840` |
| branch | `di-merge-staging` |
| uncommitted files | 1 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:7dacada3d2f74fcf6de73ff255eeaeeabc91301e5b8f231c16f4e695702ec762` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| fault model | none (CHAOS=off) |
| topology | 3 BVNs, 12 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 BVN3 |
| chaos | off |
| target duration | 1h |
| target TPS | 100 |
| storage |  (docker-network.yml) |
| block interval | 1s |
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1700MiB (effective, from docker inspect) |

Config as run is frozen in `config/` (soak.conf + override.conf, the compose and network files). Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-19T12:55:48Z |
| elapsed | 1.01h |
| driver exit | 0 (clean) |
| dn height | 38 -> 3665 |
| heals | 0 -> 0 |
| chaos events | 1 |
| monitor samples | 114 |
| seizure | SEIZED at 2026-09-19T12:09:42 :: stalled stream, undelivered for 20 polls :: stuck=n/a stuckStream= worst=BVN1->Directory gap=0 deliv=1359 undeliv=synthetic BVN2->BVN3 undeliv=124 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 3 |
| read-back probe | Whole run: 16376 timed reads, p50 1.2 ms, p95 2.3 ms, p99 6.1 ms, max 52.5 ms (chain read, BVN3, entry 1461 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 0  |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
