# Soak run 20260728T163801Z

**Purpose:** #4073 TREATMENT: permanent drops into BVN3, reconcile ON — identical to CONTROL2

| field | value |
|---|---|
| started (UTC) | 2026-07-28T16:38:01Z |
| commit | `988486db9737d5b7a6108ac63f3da75fadbaa030` |
| describe | `v1.4.5-6-g988486db9-dirty` |
| branch | `4073-idle-stream-reconcile` |
| uncommitted files | 6 (see config/uncommitted.patch) |
| image | `acc-4073:final` |
| image id | `sha256:5dd876b024038adbdf2c2a3100d83ce6fcbdd6a9094fa1daaf0e5cf38e4031cc` |
| executor version | **v2-jiuquan** |
| healing | unconditional (no config, v1.4.5+) |
| synthetic drops | `BVN3:%20+1!` |
| anchor drops | `*:%997+3` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 3m |
| target TPS | 2 |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-07-28T16:46:46Z |
| elapsed | 0.07h |
| driver exit | 0 (clean) |
| dn height | 7 -> 169 |
| heals | 0 -> 3 |
| chaos events | 4 |
| monitor samples | 11 |
| seizure | none detected |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 0 |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
