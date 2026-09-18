# Soak run 20260917T215646Z

**Purpose:** 30m proof, rerun with a full-length loadgen timeout and chaos every ~2m: #4284 healing guard, #4283 accounting, #4282 byte budget, #4277 heartbeat+restart seed

| field | value |
|---|---|
| started (UTC) | 2026-09-17T21:56:46Z |
| commit | `539937b10e74fb64090d9f054775d17f1c2b30b0` |
| describe | `10k-tps-751-g539937b10` |
| branch | `issue-4283-heal-accounting` |
| uncommitted files | 346 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:146ae93ffd36b9c82aff2e67414dbc753ea8a7f56f623f8dcceb7f8ea4e28d11` |
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

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-17T22:30:20Z |
| elapsed | 0.53h |
| driver exit | 0 (clean) |
| dn height | 38 -> 1674 |
| heals | 0 -> 9685 |
| chaos events | 19 |
| monitor samples | 53 |
| seizure | SEIZED at 2026-09-17T22:04:17 :: stuck=n/a stuckStream= worst=BVN2->BVN1 gap=357 deliv=3754 undeliv=synthetic BVN2->BVN1 undeliv=181 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 5 |
| read-back probe | Whole run: 7012 timed reads, p50 1.5 ms, p95 3.4 ms, p99 14.1 ms, max 8040.5 ms (txn read, BVN1, entry 1659 blocks old); 49 failed, 42 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 0  |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
