# Soak run 20260728T000532Z

**Purpose:** #4073 J: NO fix, 100% drop into BVN3 — nothing arrives so no hole is visible

| field | value |
|---|---|
| started (UTC) | 2026-07-28T00:05:32Z |
| commit | `eeafbfb6018dc4de3bea2d1c6342af1fe4960ec7` |
| describe | `v1.4.5-5-geeafbfb60-dirty` |
| branch | `4073-idle-stream-reconcile` |
| uncommitted files | 10 (see config/uncommitted.patch) |
| image | `acc-release:v1.4.5` |
| image id | `sha256:7e0c0553b4cdc0796efc91445c2566c9679342bf72c3da2158459e4f11602f61` |
| executor version | **v2-jiuquan** |
| healing | unconditional (no config, v1.4.5+) |
| synthetic drops | `bvn-BVN3:%1000000+999999` |
| anchor drops | `*:%997+3` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 5m |
| target TPS | 0.2 |

Config as run is frozen in `config/`. Results appended below on exit.
