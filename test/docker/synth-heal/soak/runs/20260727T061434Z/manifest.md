# Soak run 20260727T061434Z

**Purpose:** #4073 proof D: NO fix, sparse load 0.2tps, 50% synthetic drop into BVN1

| field | value |
|---|---|
| started (UTC) | 2026-07-27T06:14:34Z |
| commit | `e39a450ff934053ed9496d24b36614c2f23b0079` |
| describe | `v1.4.5-3-ge39a450ff` |
| branch | `4073-idle-stream-reconcile` |
| uncommitted files | 1 (see config/uncommitted.patch) |
| image | `acc-release:v1.4.5` |
| image id | `sha256:7e0c0553b4cdc0796efc91445c2566c9679342bf72c3da2158459e4f11602f61` |
| executor version | **v2-jiuquan** |
| healing | unconditional (no config, v1.4.5+) |
| synthetic drops | `bvn-BVN1:%2+1` |
| anchor drops | `*:%997+3` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 5m |
| target TPS | 0.2 |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-07-27T06:21:21Z |
| elapsed | 0.1h |
| driver exit | 0 (clean) |
| dn height | 8 -> 234 |
| heals | 0 -> 0 |
| chaos events | 3 |
| monitor samples | 16 |
| seizure | none detected |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
