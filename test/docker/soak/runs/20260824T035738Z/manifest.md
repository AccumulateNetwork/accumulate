# Soak run 20260824T035738Z

**Purpose:** 12h soak on 045fe6c0f: windowed replay (#4132) + bounded healer contexts + all-runs-one-anchor healing (#4163); chaos ON

| field | value |
|---|---|
| started (UTC) | 2026-08-24T03:57:38Z |
| commit | `045fe6c0f813106db4ec5c7a1c62d5d627af8bd2` |
| describe | `10k-tps-491-g045fe6c0f-dirty` |
| branch | `4138-conductor-owns-recovery` |
| uncommitted files | 40 (see config/uncommitted.patch) |
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
