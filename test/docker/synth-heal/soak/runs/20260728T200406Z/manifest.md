# Soak run 20260728T200406Z

**Purpose:** #4073 TREATMENT5: same as TREATMENT4, success log now visible

| field | value |
|---|---|
| started (UTC) | 2026-07-28T20:04:06Z |
| commit | `988486db9737d5b7a6108ac63f3da75fadbaa030` |
| describe | `v1.4.5-6-g988486db9-dirty` |
| branch | `4073-idle-stream-reconcile` |
| uncommitted files | 11 (see config/uncommitted.patch) |
| image | `acc-4073:final2` |
| image id | `sha256:8201c9507f5abb580ee3f4a95dcffd99425fc0749a6dfe17e09134e95148879b` |
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
| ended (UTC) | 2026-07-28T20:12:20Z |
| elapsed | 0.08h |
| driver exit | 0 (clean) |
| dn height | 9 -> 202 |
| heals | 0 -> 105 |
| chaos events | 4 |
| monitor samples | 13 |
| seizure | none detected |
| reconcile pulls (#4073) | 129 |
| stalled channels at end | 0 |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
