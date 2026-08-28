# Soak run 20260824T051249Z

**Purpose:** 12h soak on 971e9d274 at 10 tps: block-per-leader-group (#4164 P0) + num-workers 4 + log demotions, on top of replay window + healer fixes + cascade window; chaos ON. Expect ~0.2-0.3 blocks/s, CPU a fraction of prior runs

| field | value |
|---|---|
| started (UTC) | 2026-08-24T05:12:49Z |
| commit | `971e9d27402534de64160af2539a77e9a6dd9e70` |
| describe | `10k-tps-495-g971e9d274-dirty` |
| branch | `4138-conductor-owns-recovery` |
| uncommitted files | 47 (see config/uncommitted.patch) |
| image | `docker-bvn1-val1` |
| image id | `sha256:65b858c9077fda33b4a36f055f914a456779e8a589508f12ad25459065abb3f2` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 12h |
| target TPS | 10 |

Config as run is frozen in `config/`. Results appended below on exit.
