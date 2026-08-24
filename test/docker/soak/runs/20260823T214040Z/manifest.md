# Soak run 20260823T214040Z

**Purpose:** validate #4159 fix (own-batch retention + bounded CollectBatches) — must advance PAST DN 553, the prior deterministic stall; branch 4138-conductor-owns-recovery @ 7ef5b28de, serial exec, fresh v2-kourou genesis

| field | value |
|---|---|
| started (UTC) | 2026-08-23T21:40:40Z |
| commit | `7ef5b28de672160481aa8d9d28e5e9d121a212f2` |
| describe | `10k-tps-479-g7ef5b28de-dirty` |
| branch | `4138-conductor-owns-recovery` |
| uncommitted files | 5 (see config/uncommitted.patch) |
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
