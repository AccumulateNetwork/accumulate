# Soak run 20260727T223014Z

**Purpose:** #4073 diagnostic: is the reconcile running at all?

| field | value |
|---|---|
| started (UTC) | 2026-07-27T22:30:14Z |
| commit | `eeafbfb6018dc4de3bea2d1c6342af1fe4960ec7` |
| describe | `v1.4.5-5-geeafbfb60-dirty` |
| branch | `4073-idle-stream-reconcile` |
| uncommitted files | 6 (see config/uncommitted.patch) |
| image | `acc-4073:diag` |
| image id | `sha256:4cc22ddaf9fc9d99924816b4a0f2813253601e89361918c06e0e8b8879102100` |
| executor version | **v2-jiuquan** |
| healing | unconditional (no config, v1.4.5+) |
| synthetic drops | `*:%2+1` |
| anchor drops | `*:%997+3` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 5m |
| target TPS | 0.02 |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-07-27T22:35:52Z |
| elapsed | 0.08h |
| driver exit | 0 (clean) |
| dn height | 8 -> 190 |
| heals | 0 -> 286 |
| chaos events | 4 |
| monitor samples | 14 |
| seizure | none detected |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 0 |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
