# Soak run 20260824T025912Z

**Purpose:** validate 37cf512b3 windowed replay protection (#4132): expect 100/100 sub-treasuries funded, zero Signature-rejected lines, no flat window at DN 553; on success this is the 12h soak

| field | value |
|---|---|
| started (UTC) | 2026-08-24T02:59:12Z |
| commit | `37cf512b37c8f28ea2174a23f3d058570e0bb8a9` |
| describe | `10k-tps-489-g37cf512b3-dirty` |
| branch | `4138-conductor-owns-recovery` |
| uncommitted files | 38 (see config/uncommitted.patch) |
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
