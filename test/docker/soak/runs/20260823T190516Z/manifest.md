# Soak run 20260823T190516Z

**Purpose:** first soak of the #4137 stack on real nodes (packaging #4141, local delivery #4146, synthetic replica #4140, conductor recovery #4138, deferred sequencing #4144); serial execution, fresh v2-kourou genesis; branch 4138-conductor-owns-recovery at HEAD; watching replica growth vs the OOM history

| field | value |
|---|---|
| started (UTC) | 2026-08-23T19:05:16Z |
| commit | `6996cffba60b396cb751eaea3ca60d269826f530` |
| describe | `10k-tps-476-g6996cffba-dirty` |
| branch | `4138-conductor-owns-recovery` |
| uncommitted files | 2 (see config/uncommitted.patch) |
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
