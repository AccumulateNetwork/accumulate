# Soak run 20260824T024122Z

**Purpose:** hold the 553 wall for live analysis: which leg of the synthetic pipeline dies (produced/dispatched/received/delivered) and why healing never fires; stallkill disabled on purpose

| field | value |
|---|---|
| started (UTC) | 2026-08-24T02:41:22Z |
| commit | `98bfcfb5f568c6391806ab41d2c68a5c19789062` |
| describe | `10k-tps-488-g98bfcfb5f-dirty` |
| branch | `4138-conductor-owns-recovery` |
| uncommitted files | 37 (see config/uncommitted.patch) |
| image | `docker-bvn1-val1` |
| image id | `sha256:65b858c9077fda33b4a36f055f914a456779e8a589508f12ad25459065abb3f2` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 2h |
| target TPS | 2 |

Config as run is frozen in `config/`. Results appended below on exit.
