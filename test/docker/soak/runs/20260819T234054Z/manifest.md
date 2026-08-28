# Soak run 20260819T234054Z

**Purpose:** 12h DagBFT integration soak at f7dac217f (3s blocks, tip of issue-4105-collection-proof-delivery): does cross-partition delivery/healing ever engage at 3s block cadence (#4111)? Zeros-audit issues #4110-#4114 filed against run 20260819T215957Z before this run.

| field | value |
|---|---|
| started (UTC) | 2026-08-19T23:40:54Z |
| commit | `3edf25bf86fd6bbbfde62c9c75607884126dbe7b` |
| describe | `10k-tps-362-g3edf25bf8-dirty` |
| branch | `issue-4105-collection-proof-delivery` |
| uncommitted files | 1 (see config/uncommitted.patch) |
| image | `docker-bvn1-val1` |
| image id | `sha256:a82e6212bdbbc05097d15f02a492b5780df21a10e5cd54dfb5ba769f999c8be7` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 12h |
| target TPS | 10 |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-08-20T04:25:53Z |
| elapsed | 4.64h |
| driver exit | 1 (FAILED) |
| dn height | 131 -> 33564 |
| heals | 0 -> 6955 |
| chaos events | 26 |
| monitor samples | 56 |
| seizure | SEIZED at 2026-08-19T23:56:51 :: stalled stream, undelivered for 20 polls :: stuck=0 stuckStream= worst=BVN2->Directory gap=0 deliv=8 undeliv=anchor BVN2->Directory undeliv=385 |
| reconcile pulls (#4073) | 9826 |
| stalled channels at end | 6 |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
