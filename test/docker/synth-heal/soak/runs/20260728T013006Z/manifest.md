# Soak run 20260728T013006Z

**Purpose:** #4073 R: ISOLATION — gap healer OFF, reconcile is the only recovery path

| field | value |
|---|---|
| started (UTC) | 2026-07-28T01:30:06Z |
| commit | `eeafbfb6018dc4de3bea2d1c6342af1fe4960ec7` |
| describe | `v1.4.5-5-geeafbfb60-dirty` |
| branch | `4073-idle-stream-reconcile` |
| uncommitted files | 20 (see config/uncommitted.patch) |
| image | `acc-4073:iso` |
| image id | `sha256:96bd58ee9db20399793882eb741233b4e304487d2755df658450b8a597b27944` |
| executor version | **v2-jiuquan** |
| healing | unconditional (no config, v1.4.5+) |
| synthetic drops | `*:%1000000+999999` |
| anchor drops | `*:%997+3` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 4m |
| target TPS | 2 |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-07-28T01:40:26Z |
| elapsed | 0.08h |
| driver exit | 0 (clean) |
| dn height | 8 -> 209 |
| heals | 0 -> 524 |
| chaos events | 5 |
| monitor samples | 13 |
| seizure | SEIZED at 2026-07-28T01:33:43 :: stuck=0 stuckStream= worst=BVN2->Directory gap=74 deliv=0 undeliv=synthetic BVN2->BVN1 undeliv=57 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 0 |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
