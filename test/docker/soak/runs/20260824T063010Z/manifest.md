# Soak run 20260824T063010Z

**Purpose:** throughput probe 2 on 90de353ba+b926b82fd: parallel submitters (16), quadratic bound, start 150 tps and walk up via control API. Chaos OFF until defect 7 lands

| field | value |
|---|---|
| started (UTC) | 2026-08-24T06:30:10Z |
| commit | `b926b82fd50bec411dfbd17c7e403d101eb0669e` |
| describe | `10k-tps-501-gb926b82fd-dirty` |
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
| target TPS | 150 |

Config as run is frozen in `config/`. Results appended below on exit.
