# Soak run 20260824T033536Z

**Purpose:** 12h soak on c091579ec: #4132 windowed replay + #4163 bounded healer contexts; chaos ON — watching whether a restart can still wedge a synthetic stream (defects 2/3 of #4163 unfixed, now loud instead of silent)

| field | value |
|---|---|
| started (UTC) | 2026-08-24T03:35:36Z |
| commit | `c091579ec9678a39f103d70418d54f35c7505976` |
| describe | `10k-tps-490-gc091579ec-dirty` |
| branch | `4138-conductor-owns-recovery` |
| uncommitted files | 39 (see config/uncommitted.patch) |
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
