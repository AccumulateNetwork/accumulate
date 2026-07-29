# Soak run 20260728T001321Z

**Purpose:** #4073 K: WITH fix, ~100% drop everywhere

| field | value |
|---|---|
| started (UTC) | 2026-07-28T00:13:21Z |
| commit | `eeafbfb6018dc4de3bea2d1c6342af1fe4960ec7` |
| describe | `v1.4.5-5-geeafbfb60-dirty` |
| branch | `4073-idle-stream-reconcile` |
| uncommitted files | 12 (see config/uncommitted.patch) |
| image | `acc-4073:grace` |
| image id | `sha256:4611e8c3db7ce65e1d1b37348d89f10d8844182621cdbdd125287713690f1f53` |
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
| ended (UTC) | 2026-07-28T00:20:59Z |
| elapsed | 0.12h |
| driver exit | 0 (clean) |
| dn height | 25 -> 297 |
| heals | 0 -> 1131 |
| chaos events | 7 |
| monitor samples | 19 |
| seizure | none detected |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 3 |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
