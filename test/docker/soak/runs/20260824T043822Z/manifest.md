# Soak run 20260824T043822Z

**Purpose:** 12h soak on 8fe68c2f9 at 10 tps: windowed replay + bounded healers + all-runs healing + cascade window (32/block drain); chaos ON

| field | value |
|---|---|
| started (UTC) | 2026-08-24T04:38:22Z |
| commit | `8fe68c2f99b2feb030b77d8fef5eb5a1a63bb8bc` |
| describe | `10k-tps-494-g8fe68c2f9-dirty` |
| branch | `4138-conductor-owns-recovery` |
| uncommitted files | 42 (see config/uncommitted.patch) |
| image | `docker-bvn1-val1` |
| image id | `sha256:65b858c9077fda33b4a36f055f914a456779e8a589508f12ad25459065abb3f2` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 12h |
| target TPS | 10 |

Config as run is frozen in `config/`. Results appended below on exit.
