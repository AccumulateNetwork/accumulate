# Soak run 20260824T064308Z

**Purpose:** throughput probe 3 on 290589215: leveldb bloom+caches, batched pacer, 400 tps target. Chaos OFF until defect 7

| field | value |
|---|---|
| started (UTC) | 2026-08-24T06:43:09Z |
| commit | `2905892156ec9b350d35b922c496670301f495b9` |
| describe | `10k-tps-502-g290589215-dirty` |
| branch | `4138-conductor-owns-recovery` |
| uncommitted files | 48 (see config/uncommitted.patch) |
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
