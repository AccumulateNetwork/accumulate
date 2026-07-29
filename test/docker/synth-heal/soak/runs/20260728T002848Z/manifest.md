# Soak run 20260728T002848Z

**Purpose:** diag3

| field | value |
|---|---|
| started (UTC) | 2026-07-28T00:28:48Z |
| commit | `eeafbfb6018dc4de3bea2d1c6342af1fe4960ec7` |
| describe | `v1.4.5-5-geeafbfb60-dirty` |
| branch | `4073-idle-stream-reconcile` |
| uncommitted files | 14 (see config/uncommitted.patch) |
| image | `acc-4073:diag3` |
| image id | `sha256:da5dab4ba160f61451d34e3c715fe9d9bb33333faf134ba7ad5b218da60b8734` |
| executor version | **v2-jiuquan** |
| healing | unconditional (no config, v1.4.5+) |
| synthetic drops | `*:%1000000+999999` |
| anchor drops | `*:%997+3` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 3m |
| target TPS | 2 |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-07-28T00:34:52Z |
| elapsed | 0.09h |
| driver exit | 0 (clean) |
| dn height | 7 -> 237 |
| heals | 0 -> 874 |
| chaos events | 5 |
| monitor samples | 15 |
| seizure | none detected |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 4 |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
