# Soak run 20260824T132258Z

**Purpose:** 12h pass attempt #3 on d0619c7d4: 2s blocks, 250 tps. The BVN2 treasury skew carries ~2.5x share and its knee is ~250tps-share; 250 global keeps 1.6x margin. Full memory package + zero-copy decode

| field | value |
|---|---|
| started (UTC) | 2026-08-24T13:22:58Z |
| commit | `d0619c7d4ec7a6f0ac9b9b39f57dededf0beee5b` |
| describe | `10k-tps-513-gd0619c7d4-dirty` |
| branch | `4138-conductor-owns-recovery` |
| uncommitted files | 56 (see config/uncommitted.patch) |
| image | `docker-bvn1-val1` |
| image id | `sha256:65b858c9077fda33b4a36f055f914a456779e8a589508f12ad25459065abb3f2` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 12h |
| target TPS | 250 |

Config as run is frozen in `config/`. Results appended below on exit.
