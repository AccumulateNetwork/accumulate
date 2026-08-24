# Soak run 20260824T061956Z

**Purpose:** 100 tps throughput probe on ba8c21bb7 (block-per-commit + cascade window 1024 + channel-lag invariant): find the next ceiling. Chaos OFF on purpose - defect 7 (halted node serves stale reads) must be fixed before chaos returns

| field | value |
|---|---|
| started (UTC) | 2026-08-24T06:19:56Z |
| commit | `ba8c21bb7c6ae52e81ec2794e142d758ca98f99e` |
| describe | `10k-tps-499-gba8c21bb7-dirty` |
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
| target TPS | 100 |

Config as run is frozen in `config/`. Results appended below on exit.
