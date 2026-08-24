# Soak run 20260823T194028Z

**Purpose:** first soak of the #4137 stack on real nodes (packaging #4141, local delivery #4146, synthetic replica #4140, conductor recovery #4138, deferred sequencing #4144); serial execution, fresh v2-kourou genesis; branch 4138-conductor-owns-recovery @ 79e71e990; watching replica growth vs OOM history

| field | value |
|---|---|
| started (UTC) | 2026-08-23T19:40:28Z |
| commit | `79e71e9902e23f9a20a80dac579c132356e50bc9` |
| describe | `10k-tps-477-g79e71e990-dirty` |
| branch | `4138-conductor-owns-recovery` |
| uncommitted files | 4 (see config/uncommitted.patch) |
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
