# Soak run 20260727T055822Z

**Purpose:** #4073 proof C: WITH fix, same drops as B2

| field | value |
|---|---|
| started (UTC) | 2026-07-27T05:58:22Z |
| commit | `d5e5be6c2cfae5def605af8ba53a35129d9cf055` |
| describe | `v1.4.5-2-gd5e5be6c2-dirty` |
| branch | `4073-idle-stream-reconcile` |
| uncommitted files | 5 (see config/uncommitted.patch) |
| image | `acc-4073:test` |
| image id | `sha256:28d1704607694da9b77a75179a00e2cb857d88006422420209d231fcf6a92ffd` |
| executor version | **v2-jiuquan** |
| healing | unconditional (no config, v1.4.5+) |
| synthetic drops | `bvn-BVN1:%7+1` |
| anchor drops | `*:%997+3` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 5m |
| target TPS | 2 |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-07-27T06:05:25Z |
| elapsed | 0.1h |
| driver exit | 0 (clean) |
| dn height | 28 -> 275 |
| heals | 0 -> 6669 |
| chaos events | 5 |
| monitor samples | 17 |
| seizure | none detected |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
