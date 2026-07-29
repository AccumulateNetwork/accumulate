# Soak run 20260728T011520Z

**Purpose:** #4073 Q: 5m load then 6m idle so tail losses age past the 60-block grace

| field | value |
|---|---|
| started (UTC) | 2026-07-28T01:15:20Z |
| commit | `eeafbfb6018dc4de3bea2d1c6342af1fe4960ec7` |
| describe | `v1.4.5-5-geeafbfb60-dirty` |
| branch | `4073-idle-stream-reconcile` |
| uncommitted files | 19 (see config/uncommitted.patch) |
| image | `acc-4073:fix3` |
| image id | `sha256:09f35eba842da89bfa9af3e64eec4a89b2fb32b9c5982b26f59664588df2f740` |
| executor version | **v2-jiuquan** |
| healing | unconditional (no config, v1.4.5+) |
| synthetic drops | `*:%1000000+999999` |
| anchor drops | `*:%997+3` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 5m |
| target TPS | 2 |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-07-28T01:27:46Z |
| elapsed | 0.1h |
| driver exit | 0 (clean) |
| dn height | 13 -> 257 |
| heals | 0 -> 2917 |
| chaos events | 5 |
| monitor samples | 16 |
| seizure | none detected |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 0 |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
