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

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-08-24T01:52:47Z |
| elapsed | ?h |
| driver exit | 143 (FAILED) |
| dn height | 121 -> 529 |
| heals | 0 -> 0 |
| chaos events | 24 |
| monitor samples | 50 |
| seizure | none detected |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 0 |
| wedge captures (#4125) | 5 wedge-20260823T215118Z,wedge-20260823T220623Z wedge-20260823T222127Z,wedge-manual-4152 wedge-manual-477 |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
