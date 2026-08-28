# Soak run 20260819T204303Z

**Purpose:** local 3BVNx4val at 10 TPS — does load make ~1s blocks (#4098)

| field | value |
|---|---|
| started (UTC) | 2026-08-19T20:43:03Z |
| commit | `2ebce8422a3aba83b0897b32ae4f38d135146925` |
| describe | `10k-tps-359-g2ebce8422-dirty` |
| branch | `issue-4105-collection-proof-delivery` |
| uncommitted files | 6 (see config/uncommitted.patch) |
| image | `docker-bvn1-val1` |
| image id | `sha256:4114745fdea26f05d9e94b50bb15d7783d504a65d3ec6f7b03ac0fbbf31ab213` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 20m |
| target TPS | 10 |

Config as run is frozen in `config/`. Results appended below on exit.
