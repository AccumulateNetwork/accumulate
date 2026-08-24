# Soak run 20260824T002315Z

**Purpose:** validate #4159 REAL fix (data-availability enforced at vote time, 5e335dfb7) + fixed watchdogs; must run clean well past DN 553 and 4152; branch 4138-conductor-owns-recovery @ 5e335dfb7

| field | value |
|---|---|
| started (UTC) | 2026-08-24T00:23:15Z |
| commit | `5e335dfb7e5a1e623fb5945d7d62429f73151cb3` |
| describe | `10k-tps-481-g5e335dfb7-dirty` |
| branch | `4138-conductor-owns-recovery` |
| uncommitted files | 6 (see config/uncommitted.patch) |
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
