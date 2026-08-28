# Soak run 20260824T104703Z

**Purpose:** OOM leak hunt: reproduce the 146MiB->4GiB growth from run 065208Z at 700 tps, capture heap profiles as RSS climbs. RSS alarm + heap-in-wedge-dumps armed (63dd04d68)

| field | value |
|---|---|
| started (UTC) | 2026-08-24T10:47:03Z |
| commit | `78afd56af3d622d6d34f4c859515c4626e22ec51` |
| describe | `10k-tps-505-g78afd56af-dirty` |
| branch | `4138-conductor-owns-recovery` |
| uncommitted files | 49 (see config/uncommitted.patch) |
| image | `docker-bvn1-val1` |
| image id | `sha256:65b858c9077fda33b4a36f055f914a456779e8a589508f12ad25459065abb3f2` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 12h |
| target TPS | 700 |

Config as run is frozen in `config/`. Results appended below on exit.
