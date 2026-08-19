# Soak run 20260819T205323Z

**Purpose:** local 10 TPS with the end+1 recovery fix (67b287682) — cadence + heal-under-chaos

| field | value |
|---|---|
| started (UTC) | 2026-08-19T20:53:23Z |
| commit | `67b2876826a41a016f7b91bde4715c496abf02a8` |
| describe | `10k-tps-360-g67b287682-dirty` |
| branch | `issue-4105-collection-proof-delivery` |
| uncommitted files | 4 (see config/uncommitted.patch) |
| image | `docker-bvn1-val1` |
| image id | `sha256:9a22cc2d861b27e9926800bcd656049d4c552d9ded002b49b48f2c9a25e5df20` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 20m |
| target TPS | 10 |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-08-19T21:15:06Z |
| elapsed | 0.25h |
| driver exit | 0 (clean) |
| dn height | 2023 -> 27414 |
| heals | 224 -> 13117 |
| chaos events | 13 |
| monitor samples | 50 |
| seizure | SEIZED at 2026-08-19T21:09:21 :: stalled stream, undelivered for 20 polls :: stuck=0 stuckStream= worst=BVN2->Directory gap=0 deliv=33 undeliv=anchor BVN2->Directory undeliv=1047 |
| reconcile pulls (#4073) | 21338 |
| stalled channels at end | 5 |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
