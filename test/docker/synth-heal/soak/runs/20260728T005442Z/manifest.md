# Soak run 20260728T005442Z

**Purpose:** #4073 O: grace 60 blocks, per-sequence claim

| field | value |
|---|---|
| started (UTC) | 2026-07-28T00:54:42Z |
| commit | `eeafbfb6018dc4de3bea2d1c6342af1fe4960ec7` |
| describe | `v1.4.5-5-geeafbfb60-dirty` |
| branch | `4073-idle-stream-reconcile` |
| uncommitted files | 16 (see config/uncommitted.patch) |
| image | `acc-4073:fix3` |
| image id | `sha256:09f35eba842da89bfa9af3e64eec4a89b2fb32b9c5982b26f59664588df2f740` |
| executor version | **v2-jiuquan** |
| healing | unconditional (no config, v1.4.5+) |
| synthetic drops | `*:%1000000+999999` |
| anchor drops | `*:%997+3` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 8m |
| target TPS | 2 |

Config as run is frozen in `config/`. Results appended below on exit.
