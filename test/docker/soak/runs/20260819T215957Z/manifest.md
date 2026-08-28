# Soak run 20260819T215957Z

**Purpose:** 10 TPS with 3s blocks (#4098 f7dac217f) — before/after vs the 21-blocks-per-sec run

| field | value |
|---|---|
| started (UTC) | 2026-08-19T21:59:57Z |
| commit | `f7dac217f83f3f2c6f5c0d3c1daa170cdf265441` |
| describe | `10k-tps-361-gf7dac217f-dirty` |
| branch | `issue-4105-collection-proof-delivery` |
| uncommitted files | 6 (see config/uncommitted.patch) |
| image | `docker-bvn1-val1` |
| image id | `sha256:1c59dd224e16b45579b03826a341fb6f51c74a3d1393dd99511aee7038468806` |
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
| ended (UTC) | 2026-08-19T22:21:46Z |
| elapsed | 0.25h |
| driver exit | 0 (clean) |
| dn height | 144 -> 2885 |
| heals | 0 -> 0 |
| chaos events | 18 |
| monitor samples | 53 |
| seizure | SEIZED at 2026-08-19T22:16:01 :: stalled stream, undelivered for 20 polls :: stuck=0 stuckStream= worst=BVN1->BVN2 gap=0 deliv=0 undeliv=anchor BVN2->Directory undeliv=248 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 0 |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
