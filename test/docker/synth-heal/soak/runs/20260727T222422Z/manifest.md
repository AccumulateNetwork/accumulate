# Soak run 20260727T222422Z

**Purpose:** #4073 A/B rep 2: WITH fix + 1s reconcile, run H config

| field | value |
|---|---|
| started (UTC) | 2026-07-27T22:24:22Z |
| commit | `eeafbfb6018dc4de3bea2d1c6342af1fe4960ec7` |
| describe | `v1.4.5-5-geeafbfb60-dirty` |
| branch | `4073-idle-stream-reconcile` |
| uncommitted files | 4 (see config/uncommitted.patch) |
| image | `acc-4073:test` |
| image id | `sha256:a91d61704cc251017ded9a50318b57153a92a084c95373a77f0e92d582f2c06a` |
| executor version | **v2-jiuquan** |
| healing | unconditional (no config, v1.4.5+) |
| synthetic drops | `*:%2+1` |
| anchor drops | `*:%997+3` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 5m |
| target TPS | 0.02 |

Config as run is frozen in `config/`. Results appended below on exit.
