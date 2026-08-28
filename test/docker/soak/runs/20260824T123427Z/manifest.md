# Soak run 20260824T123427Z

**Purpose:** 12h endurance on b48944899: full memory package (byte-capped stores, DAG GC 2k rounds, GOMEMLIMIT 1250MiB, leveldb pool restored) + 2s blocks at 400 tps. Every gigabyte accounted for - this is the pass attempt

| field | value |
|---|---|
| started (UTC) | 2026-08-24T12:34:27Z |
| commit | `b48944899ba48e53fa9be54e6014b35c967d1767` |
| describe | `10k-tps-511-gb48944899-dirty` |
| branch | `4138-conductor-owns-recovery` |
| uncommitted files | 54 (see config/uncommitted.patch) |
| image | `docker-bvn1-val1` |
| image id | `sha256:65b858c9077fda33b4a36f055f914a456779e8a589508f12ad25459065abb3f2` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 12h |
| target TPS | 400 |

Config as run is frozen in `config/`. Results appended below on exit.
