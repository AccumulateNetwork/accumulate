# Soak run 20260820T050616Z

**Purpose:** Shakedown of the #4115 fix stack (23fdd5388..c04de643d): rcmgr limits+metrics, tracker-poisoning fix, heal circuit breaker, backpressure requeue, metrics listener on every node. Chaos disabled — verifying transport health and #4111 delivery behaviour at 3s blocks before the 12h chaos-free run.

| field | value |
|---|---|
| started (UTC) | 2026-08-20T05:06:16Z |
| commit | `c04de643d954ffbd29e1ffc5f60eb83b9e529f3c` |
| describe | `10k-tps-370-gc04de643d-dirty` |
| branch | `issue-4105-collection-proof-delivery` |
| uncommitted files | 3 (see config/uncommitted.patch) |
| image | `docker-bvn1-val1` |
| image id | `sha256:cd5cd3de2dc40c5a6f3254ba365ca93d0a5d5cdf9a07f08790f94d25ab947e48` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 30m |
| target TPS | 10 |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-08-20T05:28:13Z |
| elapsed | 0.25h |
| driver exit | 0 (clean) |
| dn height | 127 -> 4830 |
| heals | 0 -> 16005 |
| chaos events | 0 |
| monitor samples | 52 |
| seizure | SEIZED at 2026-08-20T05:22:28 :: stalled stream, undelivered for 20 polls :: stuck=0 stuckStream= worst=BVN2->Directory gap=0 deliv=46 undeliv=anchor BVN1->Directory undeliv=475 |
| reconcile pulls (#4073) | 30702 |
| stalled channels at end | 9 |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
