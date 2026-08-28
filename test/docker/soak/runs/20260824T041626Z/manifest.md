# Soak run 20260824T041626Z

**Purpose:** 12h soak on 3c1f119d2: 045fe6c0f fixes + loadgen control API; start 2 tps, live-bump to 10 via POST /control as the API validation; chaos ON

| field | value |
|---|---|
| started (UTC) | 2026-08-24T04:16:26Z |
| commit | `3c1f119d2170e42110259eec9b08cf8b27e2e8ed` |
| describe | `10k-tps-492-g3c1f119d2-dirty` |
| branch | `4138-conductor-owns-recovery` |
| uncommitted files | 41 (see config/uncommitted.patch) |
| image | `docker-bvn1-val1` |
| image id | `sha256:65b858c9077fda33b4a36f055f914a456779e8a589508f12ad25459065abb3f2` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 12h |
| target TPS | 2 |

Config as run is frozen in `config/`. Results appended below on exit.
