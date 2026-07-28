# Soak run 20260728T170348Z

**Purpose:** #4073 TREATMENT4: identical to CONTROL4 but reconcile ON

| field | value |
|---|---|
| started (UTC) | 2026-07-28T17:03:48Z |
| commit | `988486db9737d5b7a6108ac63f3da75fadbaa030` |
| describe | `v1.4.5-6-g988486db9-dirty` |
| branch | `4073-idle-stream-reconcile` |
| uncommitted files | 9 (see config/uncommitted.patch) |
| image | `acc-4073:final` |
| image id | `sha256:5dd876b024038adbdf2c2a3100d83ce6fcbdd6a9094fa1daaf0e5cf38e4031cc` |
| executor version | **v2-jiuquan** |
| healing | unconditional (no config, v1.4.5+) |
| synthetic drops | `BVN3:%1000+999!` |
| anchor drops | `*:%997+3` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 3m |
| target TPS | 2 |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-07-28T17:13:08Z |
| elapsed | 0.1h |
| driver exit | 0 (clean) |
| dn height | 9 -> 246 |
| heals | 0 -> 97 |
| chaos events | 4 |
| monitor samples | 16 |
| seizure | none detected |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 0 |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
