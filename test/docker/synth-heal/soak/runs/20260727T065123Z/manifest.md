# Soak run 20260727T065123Z

**Purpose:** #4073 proof I: WITH fix, identical config to H

| field | value |
|---|---|
| started (UTC) | 2026-07-27T06:51:23Z |
| commit | `e39a450ff934053ed9496d24b36614c2f23b0079` |
| describe | `v1.4.5-3-ge39a450ff-dirty` |
| branch | `4073-idle-stream-reconcile` |
| uncommitted files | 8 (see config/uncommitted.patch) |
| image | `acc-4073:test` |
| image id | `sha256:28d1704607694da9b77a75179a00e2cb857d88006422420209d231fcf6a92ffd` |
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
| ended (UTC) | 2026-07-27T06:57:00Z |
| elapsed | 0.08h |
| driver exit | 0 (clean) |
| dn height | 8 -> 199 |
| heals | 0 -> 21 |
| chaos events | 3 |
| monitor samples | 13 |
| seizure | none detected |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
