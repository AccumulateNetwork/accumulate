# Soak run 20260829T054138Z

**Purpose:** BlockchainDB f5ab54d at 100 tps (#4165): durable dynamic layer at seal (BlockchainDB#29), ErrImmutable (#28), MergeBelow lag 512 (#30/#47), borrowed fds; adapter 1daa40fb1. 8 nodes, chaos OFF, 8 shards, full-mix loadgen a0d734cc6. Twice arm B rate — the 100 tps leveldb run 20260828T153810Z collapsed on memory (GOMEMLIMIT 1200MiB) at 1.46h; watch RSS, block interval, perm segment file count, permWalkPct, conflicts/misroutes.

| field | value |
|---|---|
| started (UTC) | 2026-08-29T05:41:38Z |
| commit | `1daa40fb1b3dd6d44036e7ec48f2d8fa990b7d5d` |
| describe | `10k-tps-583-g1daa40fb1` |
| branch | `bcdb-storage-backend` |
| uncommitted files | 2 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:55291878c1063af1dd0168cba6c0b28711ecf79baba68e981d86b3b6569991a4` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | off |
| target duration | 30m |
| target TPS | 100 |
| storage | blockchainDB |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-08-29T05:50:39Z |
| elapsed | 0.11h |
| driver exit | 1 (FAILED) |
| dn height | 8 -> 156 |
| heals | 0 -> 8 |
| chaos events | 1 |
| monitor samples | 20 |
| seizure | none detected |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 4 |
| wedge captures (#4125) | 0  |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
