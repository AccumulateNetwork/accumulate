# Soak run 20260829T054019Z

**Purpose:** BlockchainDB f5ab54d on real nodes (#4165): durable dynamic layer at seal (BlockchainDB#29), ErrImmutable sentinel (#28), merged block sets via MergeBelow lag 512 (#30/#47), borrowed segment fds. Adapter 1daa40fb1. Same recipe as arm B 20260829T003712Z (8 nodes, chaos OFF, 50 tps, 8 shards, full-mix loadgen a0d734cc6) so it reads against B directly: block interval, CPU, RSS, perm segment file count over time, permWalkPct, 0 conflicts / 0 misroutes.

| field | value |
|---|---|
| started (UTC) | 2026-08-29T05:40:19Z |
| commit | `1daa40fb1b3dd6d44036e7ec48f2d8fa990b7d5d` |
| describe | `10k-tps-583-g1daa40fb1` |
| branch | `bcdb-storage-backend` |
| uncommitted files | 1 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:d3b1e34db8bbb6bd074495bcad2d9a462d9b729583656ac9b8fa9016f0a3a4b3` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | off |
| target duration | 30m |
| target TPS | 50 |
| storage | blockchainDB |

Config as run is frozen in `config/`. Results appended below on exit.
aborted before generation started; superseded by the 100 tps run
