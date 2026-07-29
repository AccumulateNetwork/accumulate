# Soak run 20260727T053231Z

**Purpose:** #4073 proof A: 5m with the interval reconcile

| field | value |
|---|---|
| started (UTC) | 2026-07-27T05:32:31Z |
| commit | `d5e5be6c2cfae5def605af8ba53a35129d9cf055` |
| describe | `v1.4.5-2-gd5e5be6c2` |
| branch | `4073-idle-stream-reconcile` |
| uncommitted files | 0  |
| image | `acc-4073:test` |
| image id | `sha256:28d1704607694da9b77a75179a00e2cb857d88006422420209d231fcf6a92ffd` |
| executor version | **v2-jiuquan** |
| healing | unconditional (no config, v1.4.5+) |
| synthetic drops | `*:%499+3` |
| anchor drops | `*:%997+3` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 5m |
| target TPS | 2 |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-07-27T05:40:49Z |
| elapsed | 0.09h |
| driver exit | 0 (clean) |
| dn height | 9 -> 297 |
| heals | 0 -> 3142 |
| chaos events | 3 |
| monitor samples | 20 |
| seizure | none detected |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
