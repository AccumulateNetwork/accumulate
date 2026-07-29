# Soak run 20260728T004607Z

**Purpose:** #4073 N: WITH per-sequence claim — same config as K which stalled 552

| field | value |
|---|---|
| started (UTC) | 2026-07-28T00:46:07Z |
| commit | `eeafbfb6018dc4de3bea2d1c6342af1fe4960ec7` |
| describe | `v1.4.5-5-geeafbfb60-dirty` |
| branch | `4073-idle-stream-reconcile` |
| uncommitted files | 15 (see config/uncommitted.patch) |
| image | `acc-4073:fix2` |
| image id | `sha256:46ca8fd5034e7ab8d723a56e68223ccde9bdd00f3b27d7ec4e97ecdb0f7838f8` |
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
| ended (UTC) | 2026-07-28T00:52:13Z |
| elapsed | 0.09h |
| driver exit | 0 (clean) |
| dn height | 8 -> 224 |
| heals | 0 -> 1410 |
| chaos events | 4 |
| monitor samples | 15 |
| seizure | none detected |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 1 |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
