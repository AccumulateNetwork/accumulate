# Soak run 20260727T054232Z

**Purpose:** #4073 proof B: no fix, DN->BVN1 seq 1-2 dropped, major blocks every minute

| field | value |
|---|---|
| started (UTC) | 2026-07-27T05:42:32Z |
| commit | `d5e5be6c2cfae5def605af8ba53a35129d9cf055` |
| describe | `v1.4.5-2-gd5e5be6c2-dirty` |
| branch | `4073-idle-stream-reconcile` |
| uncommitted files | 3 (see config/uncommitted.patch) |
| image | `acc-release:v1.4.5` |
| image id | `sha256:7e0c0553b4cdc0796efc91445c2566c9679342bf72c3da2158459e4f11602f61` |
| executor version | **v2-jiuquan** |
| healing | unconditional (no config, v1.4.5+) |
| synthetic drops | `bvn-BVN1:%997+3` |
| anchor drops | `*:%997+3` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 5m |
| target TPS | 2 |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-07-27T05:49:11Z |
| elapsed | 0.09h |
| driver exit | 0 (clean) |
| dn height | 28 -> 244 |
| heals | 0 -> 12 |
| chaos events | 4 |
| monitor samples | 16 |
| seizure | none detected |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
