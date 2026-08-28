# Soak run 20260824T114552Z

**Purpose:** byte-capped batch stores (1036aa85b) + 1s blocks + resized leveldb: 700 tps 12h. RSS should plateau ~1GB/container; channels caught up ~7s

| field | value |
|---|---|
| started (UTC) | 2026-08-24T11:45:52Z |
| commit | `1036aa85b977a4b8adb7ad30598387a556d2ee17` |
| describe | `10k-tps-508-g1036aa85b-dirty` |
| branch | `4138-conductor-owns-recovery` |
| uncommitted files | 52 (see config/uncommitted.patch) |
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
