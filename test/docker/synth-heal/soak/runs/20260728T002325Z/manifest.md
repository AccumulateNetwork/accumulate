# Soak run 20260728T002325Z

**Purpose:** diag2

| field | value |
|---|---|
| started (UTC) | 2026-07-28T00:23:25Z |
| commit | `eeafbfb6018dc4de3bea2d1c6342af1fe4960ec7` |
| describe | `v1.4.5-5-geeafbfb60-dirty` |
| branch | `4073-idle-stream-reconcile` |
| uncommitted files | 13 (see config/uncommitted.patch) |
| image | `acc-4073:diag2` |
| image id | `sha256:3700bf16c2aaf8686c143ad8b92fd2b1f45878d65d66c232a4e1cc18d3d6b00c` |
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
| ended (UTC) | 2026-07-28T00:29:33Z |
| elapsed | 0.09h |
| driver exit | 1 (FAILED) |
| dn height | 8 -> 7 |
| heals | 0 -> 0 |
| chaos events | 2 |
| monitor samples | 15 |
| seizure | none detected |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 2 |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
