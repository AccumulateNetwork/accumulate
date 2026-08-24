# Soak run 20260824T120353Z

**Purpose:** 12h endurance attempt on 29ae50bb2: 2s blocks, 400 tps, byte-capped batch stores, resized leveldb, honest channel states. Inside every measured envelope - this one should survive

| field | value |
|---|---|
| started (UTC) | 2026-08-24T12:03:53Z |
| commit | `29ae50bb289fe8f34bb5209b4d19d7ea20c430a9` |
| describe | `10k-tps-509-g29ae50bb2-dirty` |
| branch | `4138-conductor-owns-recovery` |
| uncommitted files | 53 (see config/uncommitted.patch) |
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
