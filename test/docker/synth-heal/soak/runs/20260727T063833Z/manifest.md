# Soak run 20260727T063833Z

**Purpose:** #4073 proof G: NO fix, quiet channels (no bootstrap)

| field | value |
|---|---|
| started (UTC) | 2026-07-27T06:38:33Z |
| commit | `e39a450ff934053ed9496d24b36614c2f23b0079` |
| describe | `v1.4.5-3-ge39a450ff-dirty` |
| branch | `4073-idle-stream-reconcile` |
| uncommitted files | 6 (see config/uncommitted.patch) |
| image | `acc-release:v1.4.5` |
| image id | `sha256:7e0c0553b4cdc0796efc91445c2566c9679342bf72c3da2158459e4f11602f61` |
| executor version | **v2-jiuquan** |
| healing | unconditional (no config, v1.4.5+) |
| synthetic drops | `bvn-BVN1:%2+1` |
| anchor drops | `*:%997+3` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 5m |
| target TPS | 0.02 |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-07-27T06:44:16Z |
| elapsed | 0.08h |
| driver exit | 0 (clean) |
| dn height | 9 -> 209 |
| heals | 0 -> 4 |
| chaos events | 2 |
| monitor samples | 14 |
| seizure | none detected |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
