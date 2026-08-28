# Soak run 20260824T110727Z

**Purpose:** OOM fix A/B on ed2ba278b: leveldb sized for two engines per cgroup + DisableBufferPool. 700 tps, 40 min, watching RSS slope — flat plateau or the fix is wrong

| field | value |
|---|---|
| started (UTC) | 2026-08-24T11:07:27Z |
| commit | `ed2ba278bf0676f95f37b0285a115fa178e4f4ac` |
| describe | `10k-tps-506-ged2ba278b-dirty` |
| branch | `4138-conductor-owns-recovery` |
| uncommitted files | 50 (see config/uncommitted.patch) |
| image | `docker-bvn1-val1` |
| image id | `sha256:65b858c9077fda33b4a36f055f914a456779e8a589508f12ad25459065abb3f2` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 40m |
| target TPS | 700 |

Config as run is frozen in `config/`. Results appended below on exit.
