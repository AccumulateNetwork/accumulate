# Soak run 20260727T233228Z

**Purpose:** #4073 grace: does the reconcile stop racing delivery in docker?

| field | value |
|---|---|
| started (UTC) | 2026-07-27T23:32:28Z |
| commit | `eeafbfb6018dc4de3bea2d1c6342af1fe4960ec7` |
| describe | `v1.4.5-5-geeafbfb60-dirty` |
| branch | `4073-idle-stream-reconcile` |
| uncommitted files | 9 (see config/uncommitted.patch) |
| image | `acc-4073:grace` |
| image id | `sha256:4611e8c3db7ce65e1d1b37348d89f10d8844182621cdbdd125287713690f1f53` |
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
| ended (UTC) | 2026-07-27T23:38:36Z |
| elapsed | 0.09h |
| driver exit | 0 (clean) |
| dn height | 8 -> 218 |
| heals | 0 -> 168 |
| chaos events | 3 |
| monitor samples | 15 |
| seizure | none detected |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 0 |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
