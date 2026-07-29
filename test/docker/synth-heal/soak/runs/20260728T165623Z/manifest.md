# Soak run 20260728T165623Z

**Purpose:** #4073 CONTROL4: ALL synthetics into BVN3 permanently dropped, reconcile OFF

| field | value |
|---|---|
| started (UTC) | 2026-07-28T16:56:23Z |
| commit | `988486db9737d5b7a6108ac63f3da75fadbaa030` |
| describe | `v1.4.5-6-g988486db9-dirty` |
| branch | `4073-idle-stream-reconcile` |
| uncommitted files | 8 (see config/uncommitted.patch) |
| image | `acc-4073:noreconcile` |
| image id | `sha256:3b033b6ad04b310108a0c7a690a622db4e82574613d16c8fb116a564abfdc54b` |
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
| ended (UTC) | 2026-07-28T17:03:23Z |
| elapsed | 0.06h |
| driver exit | 0 (clean) |
| dn height | 8 -> 142 |
| heals | 0 -> 720 |
| chaos events | 2 |
| monitor samples | 9 |
| seizure | none detected |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 2 |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
